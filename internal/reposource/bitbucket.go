package reposource

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

const bitbucketFirstPage = "https://api.bitbucket.org/2.0/repositories?role=member&pagelen=100"

// Bitbucket lists the Bitbucket Cloud repos the credentials are a member of.
type Bitbucket struct {
	token  string
	getter jsonGetter
}

// NewBitbucket builds the source. With an email, token is an Atlassian API
// token sent with basic auth; without one, it is an access token sent as a
// bearer token. An empty token leaves it unconfigured.
func NewBitbucket(client *http.Client, email, token string) *Bitbucket {
	authorization := "Bearer " + token
	if strings.TrimSpace(email) != "" {
		authorization = "Basic " + base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(email)+":"+token))
	}
	return &Bitbucket{token: token, getter: jsonGetter{http: client, headers: map[string]string{"Authorization": authorization}}}
}

// Provider implements app.RepoSource.
func (b *Bitbucket) Provider() domain.Provider { return domain.ProviderBitbucket }

// Label implements app.RepoSource.
func (b *Bitbucket) Label() string { return "Bitbucket" }

// IsConfigured implements app.RepoSource.
func (b *Bitbucket) IsConfigured() bool { return b.token != "" }

// List implements app.RepoSource.
func (b *Bitbucket) List(ctx context.Context) ([]domain.RepoListing, error) {
	var listings []domain.RepoListing
	next := bitbucketFirstPage
	for page := 0; next != "" && page < maxPages; page++ {
		// The next link comes from the response; refuse to send credentials to
		// any other host.
		if hostOf(next) != hostOf(bitbucketFirstPage) {
			return nil, fmt.Errorf("refusing to follow pagination to %s", hostOf(next))
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
