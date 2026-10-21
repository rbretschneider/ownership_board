package reposource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
	"github.com/rbretschneider/ownership_board/internal/secretbox"
)

// GitHub OAuth endpoints. The device verification page is fixed: the board
// refuses to send a person anywhere else, whatever GitHub's response says.
const (
	GitHubLoginURL        = "https://github.com"
	GitHubDeviceVerifyURL = "https://github.com/login/device"
	deviceGrantType       = "urn:ietf:params:oauth:grant-type:device_code"
	refreshGrantType      = "refresh_token"
	// offlineAccessScope asks GitHub for an expiring token plus refresh token
	// even if the app has expiration turned off.
	offlineAccessScope = "offline_access"
	slowDownStep       = 5 * time.Second
	maxTokenBodyBytes  = 64 << 10
)

// Ranges a device-code answer must fall in to be believed.
const (
	minDeviceInterval = 5 * time.Second
	maxDeviceInterval = 60 * time.Second
	minDeviceLifetime = 60 * time.Second
	maxDeviceLifetime = 30 * time.Minute
)

var userCodePattern = regexp.MustCompile(`^[A-Z0-9-]{4,16}$`)

// Device-flow poll outcomes.
const (
	PollPending   = "pending"
	PollConnected = "connected"
	PollExpired   = "expired"
	PollDenied    = "denied"
)

// ErrRefreshRefused means GitHub definitively refused a refresh token (it was
// revoked, expired or already used). Only this error should make the board
// ask for a reconnect; network trouble is just retried.
var ErrRefreshRefused error = &ProviderError{Message: "GitHub refused the stored sign-in; connect GitHub again"}

// DeviceCode is GitHub's answer to starting the device flow. DeviceCode is a
// secret and must stay on the server.
type DeviceCode struct {
	DeviceCode secretbox.Secret
	UserCode   string
	ExpiresAt  time.Time
	Interval   time.Duration
}

// TokenSet is a user access token and, for expiring tokens, its refresh token.
type TokenSet struct {
	AccessToken      secretbox.Secret
	AccessExpiresAt  *time.Time
	RefreshToken     secretbox.Secret
	RefreshExpiresAt *time.Time
}

// GitHubUser identifies the account a token belongs to.
type GitHubUser struct {
	ID    int64
	Login string
}

// PollResult is one device-flow poll.
type PollResult struct {
	Outcome string
	Tokens  TokenSet
	// Interval is the polling interval to use from now on.
	Interval time.Duration
}

// GitHubApp speaks the GitHub App user-token protocol: the device flow, token
// refresh, and listing the repos a user token can see. It holds no secrets of
// its own; only the public client ID. Tokens only ever go to github.com and
// api.github.com; the URL parameters exist so tests can stand in for GitHub.
type GitHubApp struct {
	clientID string
	loginURL string
	apiURL   string
	http     *http.Client
	now      func() time.Time
}

// NewGitHubApp builds the client. Empty URLs mean github.com, which is what
// production always passes.
func NewGitHubApp(client *http.Client, clientID, loginURL, apiURL string, now func() time.Time) *GitHubApp {
	return &GitHubApp{
		clientID: strings.TrimSpace(clientID),
		loginURL: strings.TrimRight(firstSet(loginURL, GitHubLoginURL), "/"),
		apiURL:   strings.TrimRight(firstSet(apiURL, DefaultGitHubAPIURL), "/"),
		http:     noRedirects(client), now: now,
	}
}

// IsAvailable reports whether a client ID is configured.
func (g *GitHubApp) IsAvailable() bool {
	return g.clientID != ""
}

// StartDevice begins the device flow. GitHub's answer is range-checked, and
// any verification page other than github.com/login/device is refused.
func (g *GitHubApp) StartDevice(ctx context.Context) (DeviceCode, error) {
	var body struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	}
	form := url.Values{"client_id": {g.clientID}, "scope": {offlineAccessScope}}
	if err := g.postForm(ctx, "/login/device/code", form, &body); err != nil {
		if isNotFound(err) {
			return DeviceCode{}, providerError("GitHub does not recognise GITHUB_APP_CLIENT_ID; check it is the app's Client ID (it starts with Iv)")
		}
		return DeviceCode{}, err
	}
	if body.DeviceCode == "" || len(body.DeviceCode) > maxTokenLength || !userCodePattern.MatchString(body.UserCode) {
		return DeviceCode{}, providerError("GitHub did not start a device sign-in; check the app has Device Flow enabled and the client ID is right")
	}
	if body.VerificationURI != GitHubDeviceVerifyURL {
		return DeviceCode{}, providerError("GitHub answered with an unexpected verification page; refusing it")
	}
	lifetime := clampDuration(time.Duration(body.ExpiresIn)*time.Second, minDeviceLifetime, maxDeviceLifetime)
	return DeviceCode{
		DeviceCode: secretbox.NewSecret(body.DeviceCode), UserCode: body.UserCode,
		ExpiresAt: g.now().Add(lifetime),
		Interval:  clampDuration(time.Duration(body.Interval)*time.Second, minDeviceInterval, maxDeviceInterval),
	}, nil
}

// Poll asks once whether the person has entered the code.
func (g *GitHubApp) Poll(ctx context.Context, device DeviceCode) (PollResult, error) {
	form := url.Values{"client_id": {g.clientID}, "device_code": {device.DeviceCode.Reveal()}, "grant_type": {deviceGrantType}}
	var body tokenResponse
	if err := g.postForm(ctx, "/login/oauth/access_token", form, &body); err != nil {
		return PollResult{}, err
	}
	return body.pollResult(device.Interval, g.now())
}

// Refresh exchanges a refresh token for a new pair. Device-flow tokens refresh
// with the client ID alone. Both old tokens stop working. A definitive refusal
// is ErrRefreshRefused; anything else is a transient failure.
func (g *GitHubApp) Refresh(ctx context.Context, refreshToken secretbox.Secret) (TokenSet, error) {
	form := url.Values{"client_id": {g.clientID}, "grant_type": {refreshGrantType}, "refresh_token": {refreshToken.Reveal()}}
	var body tokenResponse
	if err := g.postForm(ctx, "/login/oauth/access_token", form, &body); err != nil {
		return TokenSet{}, err
	}
	if isDeadRefreshToken(body.Error) {
		return TokenSet{}, ErrRefreshRefused
	}
	if body.Error != "" {
		// Configuration errors (a wrong client ID, say) are fixable without
		// reconnecting, so they stay transient.
		return TokenSet{}, providerError(fmt.Sprintf("GitHub refused the refresh (%s)", sanitizeCode(body.Error)))
	}
	if body.AccessToken == "" {
		return TokenSet{}, providerError("GitHub's refresh answer had no token")
	}
	return body.tokens(g.now()), nil
}

// User returns the account a token belongs to.
func (g *GitHubApp) User(ctx context.Context, accessToken secretbox.Secret) (GitHubUser, error) {
	var user struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	}
	if err := g.getter(accessToken).withLimit(maxIdentityBytes).get(ctx, g.apiURL+"/user", &user); err != nil {
		return GitHubUser{}, err
	}
	if user.ID <= 0 || cleanAccountName(user.Login) != user.Login || user.Login == "" {
		return GitHubUser{}, providerError("GitHub did not say who this token belongs to")
	}
	return GitHubUser{ID: user.ID, Login: user.Login}, nil
}

// ListRepos returns every repo the token can see across the app's
// installations, skipping suspended ones.
func (g *GitHubApp) ListRepos(ctx context.Context, accessToken secretbox.Secret) ([]domain.RepoListing, error) {
	getter := g.getter(accessToken)
	installations, err := g.installations(ctx, getter)
	if err != nil {
		return nil, err
	}
	var listings []domain.RepoListing
	for _, id := range installations {
		repos, err := g.installationRepos(ctx, getter, id)
		if err != nil {
			return nil, err
		}
		listings = append(listings, repos...)
	}
	return listings, nil
}

func (g *GitHubApp) installations(ctx context.Context, getter jsonGetter) ([]int64, error) {
	var ids []int64
	for page := 1; page <= maxPages; page++ {
		var body struct {
			Installations []struct {
				ID          int64   `json:"id"`
				SuspendedAt *string `json:"suspended_at"`
			} `json:"installations"`
		}
		if err := getter.get(ctx, fmt.Sprintf("%s/user/installations?per_page=%d&page=%d", g.apiURL, githubPageSize, page), &body); err != nil {
			return nil, err
		}
		for _, installation := range body.Installations {
			if installation.SuspendedAt == nil {
				ids = append(ids, installation.ID)
			}
		}
		if len(body.Installations) < githubPageSize {
			return ids, nil
		}
	}
	return ids, nil
}

func (g *GitHubApp) installationRepos(ctx context.Context, getter jsonGetter, installationID int64) ([]domain.RepoListing, error) {
	var listings []domain.RepoListing
	for page := 1; page <= maxPages; page++ {
		var body struct {
			Repositories []giteaStyleRepo `json:"repositories"`
		}
		target := fmt.Sprintf("%s/user/installations/%d/repositories?per_page=%d&page=%d", g.apiURL, installationID, githubPageSize, page)
		if err := getter.get(ctx, target, &body); err != nil {
			return nil, err
		}
		listings = append(listings, toListings(domain.ProviderGitHub, body.Repositories)...)
		if len(body.Repositories) < githubPageSize {
			return listings, nil
		}
	}
	return listings, nil
}

func (g *GitHubApp) getter(accessToken secretbox.Secret) jsonGetter {
	return jsonGetter{http: g.http, headers: map[string]string{
		"Authorization": "Bearer " + accessToken.Reveal(), "Accept": "application/vnd.github+json", "X-GitHub-Api-Version": "2022-11-28",
	}}
}

// postForm sends a form to GitHub's login host. Secrets travel only in the
// body. Errors never include the response body or the URL.
func (g *GitHubApp) postForm(ctx context.Context, path string, form url.Values, out any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, g.loginURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return providerError("could not build the GitHub request")
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response, err := g.http.Do(request)
	if err != nil {
		return unreachable(err)
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return statusError(response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxTokenBodyBytes)).Decode(out); err != nil {
		return providerError("GitHub's answer was not the JSON expected")
	}
	return nil
}

type tokenResponse struct {
	AccessToken           string `json:"access_token"`
	ExpiresIn             int    `json:"expires_in"`
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresIn int    `json:"refresh_token_expires_in"`
	Error                 string `json:"error"`
	Interval              int    `json:"interval"`
}

func (t tokenResponse) pollResult(interval time.Duration, now time.Time) (PollResult, error) {
	switch t.Error {
	case "":
		if t.AccessToken == "" {
			return PollResult{}, providerError("GitHub's answer had no token")
		}
		return PollResult{Outcome: PollConnected, Tokens: t.tokens(now), Interval: interval}, nil
	case "authorization_pending":
		return PollResult{Outcome: PollPending, Interval: interval}, nil
	case "slow_down":
		slower := max(time.Duration(t.Interval)*time.Second, interval+slowDownStep)
		return PollResult{Outcome: PollPending, Interval: min(slower, maxDeviceInterval)}, nil
	case "expired_token", "token_expired":
		return PollResult{Outcome: PollExpired, Interval: interval}, nil
	case "access_denied":
		return PollResult{Outcome: PollDenied, Interval: interval}, nil
	default:
		return PollResult{}, providerError(fmt.Sprintf("GitHub sign-in failed (%s)", sanitizeCode(t.Error)))
	}
}

func (t tokenResponse) tokens(now time.Time) TokenSet {
	tokens := TokenSet{AccessToken: secretbox.NewSecret(t.AccessToken), RefreshToken: secretbox.NewSecret(t.RefreshToken)}
	if t.ExpiresIn > 0 {
		at := now.Add(time.Duration(t.ExpiresIn) * time.Second)
		tokens.AccessExpiresAt = &at
	}
	if t.RefreshTokenExpiresIn > 0 {
		at := now.Add(time.Duration(t.RefreshTokenExpiresIn) * time.Second)
		tokens.RefreshExpiresAt = &at
	}
	return tokens
}

func clampDuration(value, low, high time.Duration) time.Duration {
	return max(low, min(value, high))
}

var errorCodePattern = regexp.MustCompile(`[^a-z_]`)

// sanitizeCode keeps an OAuth error code printable and short.
func sanitizeCode(code string) string {
	clean := errorCodePattern.ReplaceAllString(strings.ToLower(code), "")
	if len(clean) > 40 {
		return clean[:40]
	}
	return clean
}

// isDeadRefreshToken reports the OAuth errors meaning the stored refresh
// token can never work again, so only a new sign-in helps.
func isDeadRefreshToken(code string) bool {
	switch code {
	case "bad_refresh_token", "invalid_grant", "expired_token", "token_expired":
		return true
	default:
		return false
	}
}
