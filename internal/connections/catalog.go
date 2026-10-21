package connections

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/clock"
	"github.com/rbretschneider/ownership_board/internal/domain"
	"github.com/rbretschneider/ownership_board/internal/reposource"
	"github.com/rbretschneider/ownership_board/internal/secretbox"
)

// refreshAhead renews an access token this long before it expires.
const refreshAhead = 10 * time.Minute

// EnvCredentials are the server-env tokens, used when nothing is connected in
// Settings for that provider.
type EnvCredentials struct {
	GitHubToken    secretbox.Secret
	GitHubAPIURL   string
	ForgejoURL     string
	ForgejoToken   secretbox.Secret
	BitbucketEmail string
	BitbucketToken secretbox.Secret
}

// Catalog resolves the repo sources a sync should use: a connection stored in
// Settings wins over env tokens. It implements app.SourceCatalog.
type Catalog struct {
	store  Store
	vault  vault
	github *reposource.GitHubApp
	http   *http.Client
	env    EnvCredentials
	clock  clock.Clock
	logger *slog.Logger
	alerts app.Notifier
	// refreshMu serialises GitHub token refreshes in this process, so two
	// syncs never spend the same single-use refresh token.
	refreshMu sync.Mutex
}

// CatalogDeps groups the catalog's collaborators.
type CatalogDeps struct {
	Store        Store
	MasterSecret string
	GitHub       *reposource.GitHubApp
	HTTP         *http.Client
	Env          EnvCredentials
	Clock        clock.Clock
	Logger       *slog.Logger
	// Alerts receives a push when a connection stops working.
	Alerts app.Notifier
}

// NewCatalog builds the catalog.
func NewCatalog(deps CatalogDeps) (*Catalog, error) {
	v, err := newVault(deps.MasterSecret)
	if err != nil {
		return nil, err
	}
	return &Catalog{
		store: deps.Store, vault: v, github: deps.GitHub, http: deps.HTTP,
		env: deps.Env, clock: deps.Clock, logger: deps.Logger, alerts: deps.Alerts,
	}, nil
}

// Sources implements app.SourceCatalog.
func (c *Catalog) Sources(ctx context.Context) ([]app.RepoSource, error) {
	sources := make([]app.RepoSource, 0, len(Providers))
	for _, provider := range Providers {
		source, isUsable, err := c.source(ctx, provider)
		if err != nil {
			return nil, err
		}
		if isUsable {
			sources = append(sources, source)
		}
	}
	return sources, nil
}

// load returns the stored record for provider, if any. A record that cannot be
// decrypted is reported as needing a reconnect rather than as an error.
func (c *Catalog) load(ctx context.Context, provider domain.Provider) (Record, bool, error) {
	sealed, isFound, err := c.store.Get(ctx, provider)
	if err != nil || !isFound {
		return Record{}, false, err
	}
	record, err := c.vault.open(sealed)
	if err != nil {
		c.logger.Warn("stored connection cannot be decrypted; it needs reconnecting", "provider", provider, "error", err)
		return Record{Provider: provider, Account: sealed.Account, BaseURL: sealed.BaseURL, Email: sealed.Email,
			Status: StatusNeedsReconnect, Generation: sealed.Generation, ConnectedAt: sealed.ConnectedAt}, true, nil
	}
	return record, true, nil
}

// save writes record as the next generation after expected, and reports
// whether it won the compare-and-swap.
func (c *Catalog) save(ctx context.Context, record Record, expected int64) (bool, error) {
	record.Generation = expected + 1
	record.UpdatedAt = c.clock.Now()
	sealed, err := c.vault.seal(record)
	if err != nil {
		return false, err
	}
	return c.store.Put(ctx, sealed, expected)
}

func (c *Catalog) source(ctx context.Context, provider domain.Provider) (app.RepoSource, bool, error) {
	record, isStored, err := c.load(ctx, provider)
	if err != nil {
		return nil, false, err
	}
	if isStored {
		if record.Status != StatusConnected {
			return nil, false, nil
		}
		return c.storedSource(record), true, nil
	}
	source := c.envSource(provider)
	return source, source != nil && source.IsConfigured(), nil
}

func (c *Catalog) storedSource(record Record) app.RepoSource {
	switch record.Provider {
	case domain.ProviderGitHub:
		return githubAppSource{catalog: c}
	case domain.ProviderForgejo:
		return reposource.NewForgejo(c.http, record.BaseURL, record.AccessToken)
	default:
		return reposource.NewBitbucket(c.http, record.Email, record.AccessToken)
	}
}

func (c *Catalog) envSource(provider domain.Provider) app.RepoSource {
	switch provider {
	case domain.ProviderGitHub:
		return reposource.NewGitHub(c.http, c.env.GitHubAPIURL, c.env.GitHubToken)
	case domain.ProviderForgejo:
		return reposource.NewForgejo(c.http, c.env.ForgejoURL, c.env.ForgejoToken)
	default:
		return reposource.NewBitbucket(c.http, c.env.BitbucketEmail, c.env.BitbucketToken)
	}
}

// isEnvConfigured reports whether the server env holds a token for provider.
func (c *Catalog) isEnvConfigured(provider domain.Provider) bool {
	source := c.envSource(provider)
	return source != nil && source.IsConfigured()
}

// githubToken returns a usable GitHub access token, refreshing it first when
// it is about to expire. No network call happens inside a transaction: the
// refresh runs between a read and a compare-and-swap write, under refreshMu.
func (c *Catalog) githubToken(ctx context.Context) (secretbox.Secret, error) {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()
	record, isStored, err := c.load(ctx, domain.ProviderGitHub)
	if err != nil {
		return secretbox.Secret{}, err
	}
	if !isStored || record.Status != StatusConnected {
		return secretbox.Secret{}, errors.New("GitHub is not connected")
	}
	if !c.isDueForRefresh(record) {
		return record.AccessToken, nil
	}
	return c.refresh(ctx, record)
}

func (c *Catalog) isDueForRefresh(record Record) bool {
	return record.AccessExpiresAt != nil && !record.RefreshToken.IsEmpty() &&
		c.clock.Now().Add(refreshAhead).After(*record.AccessExpiresAt)
}

func (c *Catalog) refresh(ctx context.Context, record Record) (secretbox.Secret, error) {
	tokens, err := c.github.Refresh(ctx, record.RefreshToken)
	if errors.Is(err, reposource.ErrRefreshRefused) {
		c.markNeedsReconnect(ctx, record)
		return secretbox.Secret{}, err
	}
	if err != nil {
		// Transient: keep the connection and try again next sync.
		return secretbox.Secret{}, err
	}
	record.AccessToken, record.AccessExpiresAt = tokens.AccessToken, tokens.AccessExpiresAt
	record.RefreshToken, record.RefreshExpiresAt = tokens.RefreshToken, tokens.RefreshExpiresAt
	isSaved, err := c.save(ctx, record, record.Generation)
	if err != nil {
		return secretbox.Secret{}, err
	}
	if !isSaved {
		// Someone reconnected meanwhile; their stored tokens are the truth.
		return c.storedAccessToken(ctx)
	}
	return tokens.AccessToken, nil
}

func (c *Catalog) storedAccessToken(ctx context.Context) (secretbox.Secret, error) {
	record, isStored, err := c.load(ctx, domain.ProviderGitHub)
	if err != nil || !isStored || record.Status != StatusConnected {
		return secretbox.Secret{}, errors.Join(err, errors.New("GitHub is not connected"))
	}
	return record.AccessToken, nil
}

func (c *Catalog) markNeedsReconnect(ctx context.Context, record Record) {
	record.Status = StatusNeedsReconnect
	if _, err := c.save(ctx, record, record.Generation); err != nil {
		c.logger.Error("could not mark GitHub as needing reconnect", "error", err)
		return
	}
	c.logger.Warn("GitHub sign-in stopped working; it needs reconnecting", "account", record.Account)
	if c.alerts == nil {
		return
	}
	go func() {
		pushCtx, cancel := context.WithTimeout(context.Background(), validateTimeout)
		defer cancel()
		notification := app.Notification{Title: "GitHub needs reconnecting", Priority: app.PriorityDefault, Tags: []string{"key"},
			Message: "The board can no longer sync repos from GitHub. Open Settings > Repositories and reconnect."}
		if err := c.alerts.Notify(pushCtx, notification); err != nil {
			c.logger.Warn("reconnect push failed", "error", err)
		}
	}()
}

// githubAppSource lists repos with the stored GitHub App user token.
type githubAppSource struct {
	catalog *Catalog
}

func (githubAppSource) Provider() domain.Provider { return domain.ProviderGitHub }
func (githubAppSource) Label() string             { return "GitHub" }
func (githubAppSource) IsConfigured() bool        { return true }

func (s githubAppSource) List(ctx context.Context) ([]domain.RepoListing, error) {
	token, err := s.catalog.githubToken(ctx)
	if err != nil {
		return nil, err
	}
	return s.catalog.github.ListRepos(ctx, token)
}
