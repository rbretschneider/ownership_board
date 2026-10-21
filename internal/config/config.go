// Package config loads and validates every setting from the environment in one
// place, failing once with every problem listed.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Defaults for optional settings.
const (
	defaultPort             = 8080
	defaultDataDir          = "./data"
	defaultRepoSyncInterval = time.Hour
	defaultBackupKeep       = 7
	minAuthSecretLength     = 32
)

// Config is the board's bootstrap configuration.
type Config struct {
	Port             int
	DataDir          string
	PublicBaseURL    string
	AuthTokenSecret  string
	TrustProxyHops   int
	LogLevel         slog.Level
	OIDC             OIDC
	Ntfy             Ntfy
	Sources          Sources
	RepoSyncInterval time.Duration
	BackupKeep       int
}

// OIDC holds SSO settings. An empty IssuerURL turns SSO off.
type OIDC struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	ButtonLabel  string
}

// Ntfy holds push settings. An empty URL turns that stream off.
type Ntfy struct {
	// AlertsURL receives error-level logs for the operator.
	AlertsURL string
	// BoardURL receives "your move" notifications for the board's owner.
	BoardURL string
	Token    string
}

// Sources holds repo provider credentials. An empty token turns a provider off.
type Sources struct {
	GitHubToken    string
	GitHubAPIURL   string
	ForgejoURL     string
	ForgejoToken   string
	BitbucketEmail string
	BitbucketToken string
}

// DatabasePath is the SQLite file.
func (c Config) DatabasePath() string { return filepath.Join(c.DataDir, "ownership.db") }

// LogDir holds the rolling log files.
func (c Config) LogDir() string { return filepath.Join(c.DataDir, "logs") }

// BackupDir holds database backups.
func (c Config) BackupDir() string { return filepath.Join(c.DataDir, "backups") }

// OIDCRedirectURL is the exact callback to register at the identity provider.
func (c Config) OIDCRedirectURL() string { return c.PublicBaseURL + "/api/v1/auth/oidc/callback" }

// Load reads the configuration through getenv, such as os.Getenv.
func Load(getenv func(string) string) (Config, error) {
	r := reader{getenv: getenv}
	cfg := Config{
		Port:             r.integer("PORT", defaultPort, 1, 65535),
		DataDir:          r.text("DATA_DIR", defaultDataDir),
		PublicBaseURL:    r.baseURL("PUBLIC_BASE_URL"),
		AuthTokenSecret:  r.secret("AUTH_TOKEN_SECRET", minAuthSecretLength),
		TrustProxyHops:   r.integer("TRUST_PROXY_HOPS", 0, 0, 5),
		LogLevel:         r.level("LOG_LEVEL"),
		OIDC:             loadOIDC(&r),
		Ntfy:             Ntfy{AlertsURL: r.absoluteURL("NTFY_URL"), BoardURL: r.absoluteURL("BOARD_NTFY_URL"), Token: r.text("NTFY_TOKEN", "")},
		Sources:          loadSources(&r),
		RepoSyncInterval: r.duration("REPO_SYNC_INTERVAL", defaultRepoSyncInterval),
		BackupKeep:       r.integer("BACKUP_KEEP", defaultBackupKeep, 0, 365),
	}
	if cfg.OIDC.IssuerURL != "" && cfg.PublicBaseURL == "" {
		r.fail("PUBLIC_BASE_URL is required when OIDC_ISSUER_URL is set, to build the redirect URI")
	}
	return cfg, r.err()
}

func loadOIDC(r *reader) OIDC {
	oidc := OIDC{
		IssuerURL:    r.absoluteURL("OIDC_ISSUER_URL"),
		ClientID:     r.text("OIDC_CLIENT_ID", ""),
		ClientSecret: r.text("OIDC_CLIENT_SECRET", ""),
		ButtonLabel:  r.text("OIDC_BUTTON_LABEL", "Sign in with SSO"),
	}
	if oidc.IssuerURL != "" && (oidc.ClientID == "" || oidc.ClientSecret == "") {
		r.fail("OIDC_CLIENT_ID and OIDC_CLIENT_SECRET are required when OIDC_ISSUER_URL is set")
	}
	return oidc
}

func loadSources(r *reader) Sources {
	sources := Sources{
		GitHubToken:    r.text("GITHUB_TOKEN", ""),
		GitHubAPIURL:   r.absoluteURL("GITHUB_API_URL"),
		ForgejoURL:     r.absoluteURL("FORGEJO_URL"),
		ForgejoToken:   r.text("FORGEJO_TOKEN", ""),
		BitbucketEmail: r.text("BITBUCKET_EMAIL", ""),
		BitbucketToken: r.text("BITBUCKET_TOKEN", ""),
	}
	if (sources.ForgejoURL == "") != (sources.ForgejoToken == "") {
		r.fail("FORGEJO_URL and FORGEJO_TOKEN must be set together")
	}
	return sources
}

// reader collects every problem instead of stopping at the first.
type reader struct {
	getenv   func(string) string
	problems []string
}

func (r *reader) fail(format string, args ...any) {
	r.problems = append(r.problems, fmt.Sprintf(format, args...))
}

func (r *reader) err() error {
	if len(r.problems) == 0 {
		return nil
	}
	return errors.New("invalid configuration:\n  - " + strings.Join(r.problems, "\n  - "))
}

func (r *reader) text(key, fallback string) string {
	if value := strings.TrimSpace(r.getenv(key)); value != "" {
		return value
	}
	return fallback
}

func (r *reader) integer(key string, fallback, min, max int) int {
	raw := r.text(key, "")
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < min || value > max {
		r.fail("%s must be a whole number from %d to %d", key, min, max)
		return fallback
	}
	return value
}

func (r *reader) duration(key string, fallback time.Duration) time.Duration {
	raw := r.text(key, "")
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value < 0 {
		r.fail("%s must be a duration such as 30m or 2h, or 0 to turn it off", key)
		return fallback
	}
	return value
}

func (r *reader) secret(key string, minLength int) string {
	value := r.text(key, "")
	if len(value) < minLength {
		r.fail("%s is required and must be at least %d characters (openssl rand -hex 32)", key, minLength)
	}
	return value
}

func (r *reader) level(key string) slog.Level {
	var level slog.Level
	if err := level.UnmarshalText([]byte(r.text(key, "info"))); err != nil {
		r.fail("%s must be debug, info, warn or error", key)
	}
	return level
}

func (r *reader) absoluteURL(key string) string {
	value := r.text(key, "")
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		r.fail("%s must be an absolute http or https URL", key)
		return ""
	}
	return strings.TrimRight(value, "/")
}

// baseURL is an origin with no path, since cookies and redirects hang off it.
func (r *reader) baseURL(key string) string {
	value := r.absoluteURL(key)
	if value == "" {
		return ""
	}
	if parsed, _ := url.Parse(value); parsed.Path != "" {
		r.fail("%s must be an origin such as https://board.example.com, with no path", key)
	}
	return value
}
