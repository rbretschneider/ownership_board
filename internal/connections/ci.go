package connections

import (
	"context"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
	"github.com/rbretschneider/ownership_board/internal/reposource"
)

// CI implements app.CICatalog: the provider's CI, reached with the same
// credentials repo sync uses (a connection in Settings wins over env tokens).
func (c *Catalog) CI(ctx context.Context, provider domain.Provider) (app.CIProvider, error) {
	switch provider {
	case domain.ProviderGitHub, domain.ProviderForgejo:
	default:
		return nil, domain.Invalidf("the board reads CI only for GitHub and Forgejo repos, not %s", provider)
	}
	record, isStored, err := c.load(ctx, provider)
	if err != nil {
		return nil, err
	}
	if isStored {
		if record.Status != StatusConnected {
			return nil, domain.Conflictf("the %s connection needs reconnecting in Settings > Repositories", provider)
		}
		return c.storedCI(record), nil
	}
	if !c.isEnvConfigured(provider) {
		return nil, domain.Conflictf("the board has no %s connection or token to read CI with", provider)
	}
	return c.envCI(provider), nil
}

func (c *Catalog) storedCI(record Record) app.CIProvider {
	if record.Provider == domain.ProviderGitHub {
		return reposource.NewGitHubCI(c.http, c.github.APIURL(), c.githubToken)
	}
	return reposource.NewForgejoCI(c.http, record.BaseURL, record.AccessToken)
}

func (c *Catalog) envCI(provider domain.Provider) app.CIProvider {
	if provider == domain.ProviderGitHub {
		return reposource.NewGitHubCI(c.http, c.env.GitHubAPIURL, reposource.StaticToken(c.env.GitHubToken))
	}
	return reposource.NewForgejoCI(c.http, c.env.ForgejoURL, c.env.ForgejoToken)
}

var _ app.CICatalog = (*Catalog)(nil)
