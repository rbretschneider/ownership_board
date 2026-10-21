package reposource

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// forgejoPageSize matches the default MAX_RESPONSE_ITEMS of Forgejo and Gitea.
const forgejoPageSize = 50

// Forgejo lists the repos a Forgejo or Gitea token can see. Both share the
// same API.
type Forgejo struct {
	baseURL string
	token   string
	getter  jsonGetter
}

// NewForgejo builds the source for the instance at baseURL, such as
// https://git.example.com. An empty URL or token leaves it unconfigured.
func NewForgejo(client *http.Client, baseURL, token string) *Forgejo {
	headers := map[string]string{"Authorization": "token " + token}
	return &Forgejo{baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), token: token, getter: jsonGetter{http: client, headers: headers}}
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
func (f *Forgejo) IsConfigured() bool { return f.baseURL != "" && f.token != "" }

// List implements app.RepoSource. It pages until an empty page because an
// instance may cap the page size below what was asked for.
func (f *Forgejo) List(ctx context.Context) ([]domain.RepoListing, error) {
	var listings []domain.RepoListing
	for page := 1; page <= maxPages; page++ {
		url := fmt.Sprintf("%s/api/v1/user/repos?limit=%d&page=%d", f.baseURL, forgejoPageSize, page)
		var batch []giteaStyleRepo
		if err := f.getter.get(ctx, url, &batch); err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			return listings, nil
		}
		listings = append(listings, toListings(domain.ProviderForgejo, batch)...)
	}
	return listings, nil
}
