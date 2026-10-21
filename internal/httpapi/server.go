// Package httpapi is the board's HTTP adapter: the JSON API, the event
// stream, the MCP endpoint and the embedded web app.
package httpapi

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/auth"
	"github.com/rbretschneider/ownership_board/internal/broker"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

// Pinger checks a dependency is reachable, for the health endpoint.
type Pinger interface {
	Ping(ctx context.Context) error
}

// MCPHandler serves the Model Context Protocol endpoint for an authenticated
// agent.
type MCPHandler interface {
	ServeMCP(w http.ResponseWriter, r *http.Request, actor domain.Actor)
}

// Deps is everything the HTTP layer needs.
type Deps struct {
	Board  *app.BoardService
	Agents *app.AgentService
	Repos  *app.RepoService
	Auth   *auth.Service
	OIDC   *auth.OIDC
	Broker *broker.Broker
	Health Pinger
	// MCP serves the Model Context Protocol endpoint for agents.
	MCP    MCPHandler
	Logger *slog.Logger
	// PublicBaseURL is the board's origin, or empty to trust the request host.
	PublicBaseURL string
	TrustedHops   int
	// Frontend is the built web app, or nil when it was not embedded.
	Frontend fs.FS
}

type server struct {
	deps    Deps
	cookies cookieJar
	network network
}

// New builds the board's root handler.
func New(deps Deps) http.Handler {
	net := network{trustedHops: deps.TrustedHops}
	s := &server{deps: deps, cookies: cookieJar{network: net}, network: net}
	mux := http.NewServeMux()
	rt := &router{mux: mux, auth: deps.Auth, origin: deps.PublicBaseURL, logger: deps.Logger, network: net}
	s.routes(rt)
	mux.Handle("/api/", http.HandlerFunc(apiNotFound))
	mux.Handle("/", readOnly(newSPA(deps.Frontend)))
	return securityHeaders(mux)
}

func (s *server) routes(rt *router) {
	rt.public("GET /health", s.health)
	rt.public("GET /api/v1/health", s.health)
	s.authRoutes(rt)
	s.boardRoutes(rt)
	s.repoRoutes(rt)
	s.agentRoutes(rt)
	rt.human("GET /api/v1/tokens", s.listTokens)
	rt.human("POST /api/v1/tokens", s.createToken)
	rt.human("DELETE /api/v1/tokens/{id}", s.revokeToken)
	rt.anyone("GET /api/v1/events/stream", s.eventStream)
}

func (s *server) health(w http.ResponseWriter, r *http.Request, _ caller) error {
	if err := s.deps.Health.Ping(r.Context()); err != nil {
		writeErrorCode(w, http.StatusServiceUnavailable, codeInternal, "database unavailable")
		return nil
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	return nil
}

func apiNotFound(w http.ResponseWriter, r *http.Request) {
	writeErrorCode(w, http.StatusNotFound, codeNotFound, "no such endpoint: "+r.Method+" "+r.URL.Path)
}

// securityHeaders applies headers every response should carry. The CSP allows
// inline styles only, which React's style attributes need.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("Referrer-Policy", "same-origin")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data: https:; style-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}
