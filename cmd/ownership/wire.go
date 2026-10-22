package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/auth"
	"github.com/rbretschneider/ownership_board/internal/backup"
	"github.com/rbretschneider/ownership_board/internal/broker"
	"github.com/rbretschneider/ownership_board/internal/clock"
	"github.com/rbretschneider/ownership_board/internal/config"
	"github.com/rbretschneider/ownership_board/internal/connections"
	"github.com/rbretschneider/ownership_board/internal/httpapi"
	"github.com/rbretschneider/ownership_board/internal/mcp"
	"github.com/rbretschneider/ownership_board/internal/notify"
	"github.com/rbretschneider/ownership_board/internal/reposource"
	"github.com/rbretschneider/ownership_board/internal/secretbox"
	"github.com/rbretschneider/ownership_board/internal/store/sqlite"
	"github.com/rbretschneider/ownership_board/internal/webui"
)

// outboundTimeout bounds every call the board makes to another service.
const outboundTimeout = 30 * time.Second

var systemClock = clock.System{}

// application is the wired board plus what runs beside the HTTP server.
type application struct {
	handler http.Handler
	db      *sqlite.DB
	repos   *app.RepoService
	backups *backup.Backups
	cfg     config.Config
	logger  *slog.Logger
}

func build(ctx context.Context, cfg config.Config, logger *slog.Logger) (*application, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return nil, fmt.Errorf("create data dir %s: %w", cfg.DataDir, err)
	}
	db, err := sqlite.Open(ctx, cfg.DatabasePath())
	if err != nil {
		return nil, err
	}
	authService, err := auth.NewService(sqlite.NewAuthStore(db), cfg.AuthTokenSecret, systemClock, auth.NewThrottle(systemClock))
	if err != nil {
		db.Close()
		return nil, err
	}
	hub := broker.New()
	sink := app.FanoutSink{hub, app.NewNotifyingSink(notificationPolicy(cfg), boardNotifier(cfg), logger, systemClock)}
	catalog, err := newCatalog(cfg, db, logger)
	if err != nil {
		db.Close()
		return nil, err
	}
	services := mcp.Services{
		Board:  app.NewBoardService(db, systemClock, sink),
		Agents: app.NewAgentService(db, systemClock, sink, hub),
		Repos:  app.NewRepoService(db, systemClock, catalog, logger),
	}
	handler := httpapi.New(httpapi.Deps{
		Board: services.Board, Agents: services.Agents, Repos: services.Repos,
		Connections: connections.NewService(catalog, services.Repos, boardNotifier(cfg), connectionPolicy(cfg)),
		Auth:        authService, OIDC: auth.NewOIDC(oidcConfig(cfg), authService),
		Broker: hub, Health: db, Logger: logger,
		MCP:           mcp.NewServer(version, logger, mcp.BoardTools(services)...),
		PublicBaseURL: cfg.PublicBaseURL, TrustedHops: cfg.TrustProxyHops, Frontend: webui.FS(),
	})
	return &application{
		handler: handler, db: db, repos: services.Repos, cfg: cfg, logger: logger,
		backups: backup.New(db.SQL(), cfg.BackupDir(), cfg.BackupKeep, systemClock, logger),
	}, nil
}

func (a *application) close() {
	if err := a.db.Close(); err != nil {
		a.logger.Error("closing database", "error", err)
	}
}

// startBackground runs the scheduled jobs until ctx ends.
func (a *application) startBackground(ctx context.Context) {
	if a.cfg.BackupKeep > 0 {
		go a.backups.Run(ctx)
	}
	if a.cfg.RepoSyncInterval > 0 {
		go a.syncReposEvery(ctx, a.cfg.RepoSyncInterval)
	}
}

// syncReposEvery syncs once at boot, then on the interval. Each provider's
// failure is recorded on its status for the settings screen.
func (a *application) syncReposEvery(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := a.repos.Sync(ctx); err != nil && ctx.Err() == nil {
			a.logger.Error("repo sync failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// newCatalog builds the repo-source catalog: connections stored in Settings,
// falling back to env tokens. Every provider call goes through the guarded
// client (no redirects, no loopback or link-local destinations).
func newCatalog(cfg config.Config, db *sqlite.DB, logger *slog.Logger) (*connections.Catalog, error) {
	client := reposource.NewHTTPClient(reposource.ClientOptions{Timeout: outboundTimeout})
	sources := cfg.Sources
	return connections.NewCatalog(connections.CatalogDeps{
		Store: sqlite.NewConnectionStore(db), MasterSecret: cfg.AuthTokenSecret,
		GitHub: reposource.NewGitHubApp(client, sources.GitHubAppClientID, "", "", systemClock.Now),
		HTTP:   client, Clock: systemClock, Logger: logger, Alerts: boardNotifier(cfg),
		Env: connections.EnvCredentials{
			GitHubToken: secretbox.NewSecret(sources.GitHubToken), GitHubAPIURL: sources.GitHubAPIURL,
			ForgejoURL: sources.ForgejoURL, ForgejoToken: secretbox.NewSecret(sources.ForgejoToken),
			BitbucketEmail: sources.BitbucketEmail, BitbucketToken: secretbox.NewSecret(sources.BitbucketToken),
		},
	})
}

func connectionPolicy(cfg config.Config) connections.Policy {
	return connections.Policy{AllowedGitHubLogins: cfg.Sources.AllowedGitHubLogins, AllowedForgejoHosts: cfg.Sources.AllowedForgejoHosts}
}

func boardNotifier(cfg config.Config) app.Notifier {
	if cfg.Ntfy.BoardURL == "" {
		return notify.Noop{}
	}
	return newNtfy(cfg.Ntfy.BoardURL, cfg.Ntfy.Token)
}

func newNtfy(topicURL, token string) *notify.Ntfy {
	return notify.NewNtfy(&http.Client{Timeout: outboundTimeout}, topicURL, token)
}

func notificationPolicy(cfg config.Config) app.NotificationPolicy {
	return app.NewNotificationPolicy(ticketLinks{base: cfg.PublicBaseURL}, app.DefaultNotificationRules()...)
}

func oidcConfig(cfg config.Config) auth.OIDCConfig {
	return auth.OIDCConfig{
		IssuerURL: cfg.OIDC.IssuerURL, ClientID: cfg.OIDC.ClientID, ClientSecret: cfg.OIDC.ClientSecret,
		RedirectURL: cfg.OIDCRedirectURL(), ButtonLabel: cfg.OIDC.ButtonLabel,
	}
}

// ticketLinks deep-links notifications to a ticket in the web app.
type ticketLinks struct {
	base string
}

// Ticket implements app.Links. Without a public URL there is nothing to link.
func (l ticketLinks) Ticket(id int64) string {
	if l.base == "" {
		return ""
	}
	return l.base + "/#/t/" + strconv.FormatInt(id, 10)
}
