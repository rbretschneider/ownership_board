package httpapi

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/rbretschneider/ownership_board/internal/auth"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

// caller is who made a request: a person, an agent, or nobody.
type caller struct {
	actor domain.Actor
	// user is set for a person.
	user auth.User
	// isBearer is true when an agent token authenticated the request; such
	// requests carry no ambient credentials and need no origin check.
	isBearer bool
}

// access is the class of caller a route admits.
type access int

const (
	accessPublic access = iota
	accessAnyone
	accessHuman
	accessAgent
)

// handlerFunc is an endpoint that returns its error instead of writing it.
type handlerFunc func(w http.ResponseWriter, r *http.Request, who caller) error

// router registers routes by access class. It is the only way to add a route,
// so an endpoint cannot ship public by forgetting an annotation: it must be
// registered with public() on purpose.
type router struct {
	mux     *http.ServeMux
	auth    *auth.Service
	origin  string
	logger  *slog.Logger
	network network
	writes  *writeLimiter
}

func (rt *router) public(pattern string, fn handlerFunc)    { rt.handle(pattern, accessPublic, fn) }
func (rt *router) anyone(pattern string, fn handlerFunc)    { rt.handle(pattern, accessAnyone, fn) }
func (rt *router) human(pattern string, fn handlerFunc)     { rt.handle(pattern, accessHuman, fn) }
func (rt *router) agentOnly(pattern string, fn handlerFunc) { rt.handle(pattern, accessAgent, fn) }

func (rt *router) handle(pattern string, level access, fn handlerFunc) {
	rt.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		who, err := rt.identify(r)
		if err != nil {
			writeError(w, r, rt.logger, err)
			return
		}
		if err := rt.authorize(r, who, level); err != nil {
			writeError(w, r, rt.logger, err)
			return
		}
		if err := fn(w, r, who); err != nil {
			writeError(w, r, rt.logger, err)
		}
	})
}

// identify resolves the caller. An invalid credential on a public route is
// treated as no credential, so an expired cookie never blocks signing in.
func (rt *router) identify(r *http.Request) (caller, error) {
	if secret, isBearer := bearerToken(r); isBearer {
		token, err := rt.auth.TokenFromSecret(r.Context(), secret)
		if err != nil {
			return caller{}, err
		}
		return caller{actor: token.Actor(), isBearer: true}, nil
	}
	access := cookieValue(r, accessCookie)
	if access == "" {
		return caller{}, nil
	}
	user, err := rt.auth.UserFromAccessToken(r.Context(), access)
	if err != nil {
		return caller{}, nil
	}
	return caller{actor: user.Actor(), user: user}, nil
}

func (rt *router) authorize(r *http.Request, who caller, level access) error {
	if who.isBearer && isUnsafe(r.Method) {
		if err := rt.writes.allow(who.actor.ID); err != nil {
			return err
		}
	}
	if !who.isBearer && isUnsafe(r.Method) && !rt.isSameOrigin(r) {
		return domain.Forbiddenf("cross-origin request refused")
	}
	switch level {
	case accessPublic:
		return nil
	case accessAgent:
		return requireKind(who, domain.ActorAgent, "this endpoint needs an agent token")
	case accessHuman:
		return requireKind(who, domain.ActorHuman, "this endpoint is for people signed in to the board")
	default:
		if who.actor.Kind == "" {
			return auth.ErrUnauthorized
		}
		return nil
	}
}

func requireKind(who caller, kind domain.ActorKind, message string) error {
	if who.actor.Kind == "" {
		return auth.ErrUnauthorized
	}
	if who.actor.Kind != kind {
		return domain.Forbiddenf("%s", message)
	}
	return nil
}

// isSameOrigin is the CSRF defence for cookie requests, on top of SameSite=Lax:
// the Origin must be the board's public origin, or the request's own host when
// no public origin is configured.
func (rt *router) isSameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	if rt.origin != "" {
		return origin == rt.origin
	}
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Host == r.Host
}

func isUnsafe(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	scheme, token, isFound := strings.Cut(header, " ")
	if !isFound || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	return strings.TrimSpace(token), true
}
