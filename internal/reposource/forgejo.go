package reposource

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/rbretschneider/ownership_board/internal/domain"
	"github.com/rbretschneider/ownership_board/internal/secretbox"
)

// Forgejo limits.
const (
	// forgejoPageSize matches the default MAX_RESPONSE_ITEMS of Forgejo and Gitea.
	forgejoPageSize      = 50
	maxForgejoURLLength  = 300
	maxTokenLength       = 512
	forgejoUserEndpoint  = "/api/v1/user"
	forgejoReposEndpoint = "/api/v1/user/repos"
)

// Forgejo lists the repos a Forgejo or Gitea token can see. Both share the
// same API.
type Forgejo struct {
	baseURL string
	token   secretbox.Secret
	getter  jsonGetter
}

// NewForgejo builds the source for the instance at baseURL, which must already
// be normalised with NormalizeForgejoURL. An empty URL or token leaves it
// unconfigured.
func NewForgejo(client *http.Client, baseURL string, token secretbox.Secret) *Forgejo {
	headers := map[string]string{"Authorization": "token " + token.Reveal()}
	return &Forgejo{baseURL: baseURL, token: token, getter: jsonGetter{http: noRedirects(client), headers: headers}}
}

// NormalizeForgejoURL reduces a typed URL to scheme, host, optional port and
// path, and refuses anything else: credentials, a query, a fragment, dot or
// encoded-slash path segments, a bad port, or (when allowedHosts is set) a
// host[:port] not on that list.
func NormalizeForgejoURL(raw string, allowedHosts []string) (string, error) {
	value := strings.TrimSpace(raw)
	parsed, err := url.Parse(value)
	if err != nil || len(value) > maxForgejoURLLength || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return "", domain.Invalidf("the Forgejo address must be an http or https URL such as https://git.example.com")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" || parsed.ForceQuery {
		return "", domain.Invalidf("the Forgejo address must not contain a user name, password, query or fragment")
	}
	if !isValidPort(parsed.Port()) || hasUnsafePath(parsed) {
		return "", domain.Invalidf("the Forgejo address has an invalid port or path")
	}
	host := strings.ToLower(parsed.Host)
	if len(allowedHosts) > 0 && !isAllowedHost(host, parsed.Hostname(), allowedHosts) {
		return "", domain.Forbiddenf("%s is not in FORGEJO_ALLOWED_HOSTS on the server", host)
	}
	return parsed.Scheme + "://" + host + strings.TrimRight(parsed.EscapedPath(), "/"), nil
}

// CheckToken validates a token's length and characters before it is used.
// Real provider tokens are visible ASCII; anything else would be a mistake or
// an attempt to smuggle header characters.
func CheckToken(token string) (secretbox.Secret, error) {
	value := strings.TrimSpace(token)
	if value == "" || len(value) > maxTokenLength || !isPrintableASCII(value) {
		return secretbox.Secret{}, domain.Invalidf("the token is empty, too long, or contains spaces or unusual characters")
	}
	return secretbox.NewSecret(value), nil
}

func isValidPort(port string) bool {
	if port == "" {
		return true
	}
	number, err := strconv.Atoi(port)
	return err == nil && number >= 1 && number <= 65535
}

// hasUnsafePath rejects "." and ".." segments and percent-encoded dots or
// slashes, which could walk the request off the forge's API.
func hasUnsafePath(parsed *url.URL) bool {
	lowered := strings.ToLower(parsed.EscapedPath())
	if strings.Contains(lowered, "%2e") || strings.Contains(lowered, "%2f") || strings.Contains(lowered, "%5c") {
		return true
	}
	for _, segment := range strings.Split(parsed.Path, "/") {
		if segment == "." || segment == ".." {
			return true
		}
	}
	return false
}

// isAllowedHost matches an allowlist entry against host[:port]. An entry with
// a port must match exactly; an entry without one matches that host on any
// port.
func isAllowedHost(hostPort, hostname string, allowed []string) bool {
	for _, candidate := range allowed {
		entry := strings.ToLower(strings.TrimSpace(candidate))
		if entry == hostPort || (!strings.Contains(entry, ":") && entry == strings.ToLower(hostname)) {
			return true
		}
	}
	return false
}

// Provider implements app.RepoSource.
func (f *Forgejo) Provider() domain.Provider { return domain.ProviderForgejo }

// Label implements app.RepoSource.
func (f *Forgejo) Label() string {
	if f.baseURL == "" {
		return "Forgejo / Gitea"
	}
	return fmt.Sprintf("Forgejo (%s)", hostOf(f.baseURL))
}

// IsConfigured implements app.RepoSource.
func (f *Forgejo) IsConfigured() bool { return f.baseURL != "" && !f.token.IsEmpty() }

// Whoami checks the token works and returns the login it belongs to. It only
// accepts an answer shaped like a Forgejo user, so pointing it at some other
// service reveals nothing about that service.
func (f *Forgejo) Whoami(ctx context.Context) (string, error) {
	var user struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	}
	if err := f.getter.withLimit(maxIdentityBytes).get(ctx, f.baseURL+forgejoUserEndpoint, &user); err != nil {
		return "", err
	}
	login := cleanAccountName(user.Login)
	if user.ID <= 0 || login == "" {
		return "", providerError("that address did not answer like Forgejo or Gitea")
	}
	return login, nil
}

// List implements app.RepoSource. It pages until an empty page because an
// instance may cap the page size below what was asked for.
func (f *Forgejo) List(ctx context.Context) ([]domain.RepoListing, error) {
	var listings []domain.RepoListing
	for page := 1; page <= maxPages; page++ {
		target := fmt.Sprintf("%s%s?limit=%d&page=%d", f.baseURL, forgejoReposEndpoint, forgejoPageSize, page)
		var batch []giteaStyleRepo
		if err := f.getter.get(ctx, target, &batch); err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			return listings, nil
		}
		listings = append(listings, toListings(domain.ProviderForgejo, batch)...)
	}
	return listings, nil
}
