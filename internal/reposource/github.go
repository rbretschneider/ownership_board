package reposource

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/rbretschneider/ownership_board/internal/domain"
	"github.com/rbretschneider/ownership_board/internal/secretbox"
)

// DefaultGitHubAPIURL is github.com's API. GitHub Enterprise Server uses
// https://<host>/api/v3.
const DefaultGitHubAPIURL = "https://api.github.com"

const githubPageSize = 100

// GitHub lists the repos a GitHub token can see: owned, collaborator, and
// organisation repos.
type GitHub struct {
	apiURL string
	token  secretbox.Secret
	getter jsonGetter
}

// NewGitHub builds the source. An empty token leaves it unconfigured.
func NewGitHub(client *http.Client, apiURL string, token secretbox.Secret) *GitHub {
	base := strings.TrimRight(firstSet(apiURL, DefaultGitHubAPIURL), "/")
	headers := map[string]string{
		"Authorization":        "Bearer " + token.Reveal(),
		"Accept":               "application/vnd.github+json",
		"X-GitHub-Api-Version": "2022-11-28",
	}
	return &GitHub{apiURL: base, token: token, getter: jsonGetter{http: noRedirects(client), headers: headers}}
}

// Provider implements app.RepoSource.
func (g *GitHub) Provider() domain.Provider { return domain.ProviderGitHub }

// Label implements app.RepoSource.
func (g *GitHub) Label() string {
	if g.apiURL == DefaultGitHubAPIURL {
		return "GitHub"
	}
	return fmt.Sprintf("GitHub (%s)", hostOf(g.apiURL))
}

// IsConfigured implements app.RepoSource.
func (g *GitHub) IsConfigured() bool { return !g.token.IsEmpty() }

// List implements app.RepoSource.
func (g *GitHub) List(ctx context.Context) ([]domain.RepoListing, error) {
	var listings []domain.RepoListing
	for page := 1; page <= maxPages; page++ {
		url := fmt.Sprintf("%s/user/repos?per_page=%d&page=%d&affiliation=owner,collaborator,organization_member&sort=full_name",
			g.apiURL, githubPageSize, page)
		var batch []giteaStyleRepo
		if err := g.getter.get(ctx, url, &batch); err != nil {
			return nil, err
		}
		listings = append(listings, toListings(domain.ProviderGitHub, batch)...)
		if len(batch) < githubPageSize {
			return listings, nil
		}
	}
	return listings, nil
}

// giteaStyleRepo is the shape GitHub, Forgejo and Gitea share for a repo.
type giteaStyleRepo struct {
	FullName      string  `json:"full_name"`
	HTMLURL       string  `json:"html_url"`
	Description   *string `json:"description"`
	DefaultBranch string  `json:"default_branch"`
	IsPrivate     bool    `json:"private"`
	IsArchived    bool    `json:"archived"`
}

func toListings(provider domain.Provider, batch []giteaStyleRepo) []domain.RepoListing {
	listings := make([]domain.RepoListing, 0, len(batch))
	for _, repo := range batch {
		description := ""
		if repo.Description != nil {
			description = *repo.Description
		}
		listings = append(listings, domain.RepoListing{
			Provider: provider, FullName: repo.FullName, URL: repo.HTMLURL, Description: description,
			DefaultBranch: repo.DefaultBranch, IsPrivate: repo.IsPrivate, IsArchived: repo.IsArchived,
		})
	}
	return listings
}

func firstSet(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
