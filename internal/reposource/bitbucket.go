package reposource

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"

	"github.com/rbretschneider/ownership_board/internal/domain"
	"github.com/rbretschneider/ownership_board/internal/secretbox"
)

const (
	bitbucketHost      = "api.bitbucket.org"
	bitbucketFirstPage = "https://" + bitbucketHost + "/2.0/repositories?role=member&pagelen=100"
	bitbucketUserURL   = "https://" + bitbucketHost + "/2.0/user"
	maxEmailLength     = 254
)

// Bitbucket lists the Bitbucket Cloud repos the credentials are a member of.
type Bitbucket struct {
	token  secretbox.Secret
	getter jsonGetter
}

// NewBitbucket builds the source. With an email, token is an Atlassian API
// token sent with basic auth; without one, it is an access token sent as a
// bearer token. An empty token leaves it unconfigured.
func NewBitbucket(client *http.Client, email string, token secretbox.Secret) *Bitbucket {
	authorization := "Bearer " + token.Reveal()
	if strings.TrimSpace(email) != "" {
		authorization = "Basic " + base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(email)+":"+token.Reveal()))
	}
	return &Bitbucket{token: token, getter: jsonGetter{http: noRedirects(client), headers: map[string]string{"Authorization": authorization}}}
}

// CheckEmail validates an optional Bitbucket account email.
func CheckEmail(email string) (string, error) {
	value := strings.TrimSpace(email)
	if len(value) > maxEmailLength || strings.ContainsAny(value, " \t\r\n:") || (value != "" && !strings.Contains(value, "@")) {
		return "", domain.Invalidf("the email must be the Atlassian account email, or empty for an access token")
	}
	return value, nil
}

// Provider implements app.RepoSource.
func (b *Bitbucket) Provider() domain.Provider { return domain.ProviderBitbucket }

// Label implements app.RepoSource.
func (b *Bitbucket) Label() string { return "Bitbucket" }

// IsConfigured implements app.RepoSource.
func (b *Bitbucket) IsConfigured() bool { return !b.token.IsEmpty() }

// Whoami checks the credentials and returns the account's name.
func (b *Bitbucket) Whoami(ctx context.Context) (string, error) {
	var user struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		AccountID   string `json:"account_id"`
	}
	if err := b.getter.withLimit(maxIdentityBytes).get(ctx, bitbucketUserURL, &user); err != nil {
		return "", err
	}
	name := cleanAccountName(firstSet(user.Username, user.DisplayName))
	if name == "" || user.AccountID == "" {
		return "", providerError("Bitbucket did not return an account for these credentials")
	}
	return name, nil
}

// List implements app.RepoSource.
func (b *Bitbucket) List(ctx context.Context) ([]domain.RepoListing, error) {
	var listings []domain.RepoListing
	next := bitbucketFirstPage
	for page := 0; next != "" && page < maxPages; page++ {
		if !isBitbucketAPIURL(next) {
			return nil, providerError("Bitbucket sent a next-page link to another address; refusing to follow it")
		}
		var batch bitbucketPage
		if err := b.getter.get(ctx, next, &batch); err != nil {
			return nil, err
		}
		listings = append(listings, batch.listings()...)
		next = batch.Next
	}
	return listings, nil
}

// isBitbucketAPIURL accepts only https links on the API host, so a crafted
// next link cannot send the credentials anywhere else or in the clear.
func isBitbucketAPIURL(link string) bool {
	parsed, err := url.Parse(link)
	return err == nil && parsed.Scheme == "https" && parsed.Host == bitbucketHost && parsed.User == nil
}

type bitbucketPage struct {
	Next   string          `json:"next"`
	Values []bitbucketRepo `json:"values"`
}

type bitbucketRepo struct {
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	IsPrivate   bool   `json:"is_private"`
	MainBranch  *struct {
		Name string `json:"name"`
	} `json:"mainbranch"`
	Links struct {
		HTML struct {
			Href string `json:"href"`
		} `json:"html"`
	} `json:"links"`
}

func (p bitbucketPage) listings() []domain.RepoListing {
	listings := make([]domain.RepoListing, 0, len(p.Values))
	for _, repo := range p.Values {
		branch := ""
		if repo.MainBranch != nil {
			branch = repo.MainBranch.Name
		}
		listings = append(listings, domain.RepoListing{
			Provider: domain.ProviderBitbucket, FullName: repo.FullName, URL: repo.Links.HTML.Href,
			Description: repo.Description, DefaultBranch: branch, IsPrivate: repo.IsPrivate,
		})
	}
	return listings
}
