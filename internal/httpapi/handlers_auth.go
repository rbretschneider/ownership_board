package httpapi

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/rbretschneider/ownership_board/internal/auth"
)

// Where the OIDC callback sends the browser, read by the web app's hash router.
const (
	afterSignIn     = "/#/"
	afterLink       = "/#/settings?linked=1"
	signInErrorPage = "/#/login?error="
)

// SSO failure codes shown on the login page.
const (
	ssoNotLinked = "sso_not_linked"
	ssoDenied    = "sso_denied"
	ssoFailed    = "sso_failed"
)

func (s *server) authRoutes(rt *router) {
	rt.public("GET /api/v1/auth/status", s.authStatus)
	rt.public("POST /api/v1/auth/setup", s.setup)
	rt.public("POST /api/v1/auth/login", s.login)
	rt.public("POST /api/v1/auth/refresh", s.refresh)
	rt.public("POST /api/v1/auth/logout", s.logout)
	rt.public("GET /api/v1/auth/oidc/start", s.oidcStart)
	rt.public("GET /api/v1/auth/oidc/callback", s.oidcCallback)
	rt.anyone("GET /api/v1/me", s.me)
}

// authStatus also renews an expired access cookie from a valid refresh
// cookie, so opening the app after a while lands straight on the board.
func (s *server) authStatus(w http.ResponseWriter, r *http.Request, who caller) error {
	isSetupRequired, err := s.deps.Auth.IsSetupRequired(r.Context())
	if err != nil {
		return err
	}
	status := authStatusDTO{
		IsSetupRequired: isSetupRequired, IsPasswordLogin: true,
		OIDC: oidcDTO{IsEnabled: s.deps.OIDC.IsEnabled(), Label: s.deps.OIDC.ButtonLabel()},
	}
	if user, isSignedIn := s.currentUser(w, r, who); isSignedIn {
		dto := toUser(user)
		status.User = &dto
	}
	writeJSON(w, http.StatusOK, status)
	return nil
}

func (s *server) currentUser(w http.ResponseWriter, r *http.Request, who caller) (auth.User, bool) {
	if who.actor.IsHuman() {
		return who.user, true
	}
	refresh := cookieValue(r, refreshCookie)
	if refresh == "" {
		return auth.User{}, false
	}
	session, err := s.deps.Auth.Refresh(r.Context(), refresh)
	if err != nil {
		s.cookies.clearSession(w, r)
		return auth.User{}, false
	}
	s.cookies.setSession(w, r, session)
	return session.User, true
}

func (s *server) setup(w http.ResponseWriter, r *http.Request, _ caller) error {
	var body struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	session, err := s.deps.Auth.Setup(r.Context(), auth.SetupInput{Name: body.Name, Email: body.Email, Password: body.Password})
	if err != nil {
		return err
	}
	return s.signedIn(w, r, session)
}

func (s *server) login(w http.ResponseWriter, r *http.Request, _ caller) error {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	session, err := s.deps.Auth.Login(r.Context(), s.network.clientIP(r), body.Email, body.Password)
	if err != nil {
		return err
	}
	return s.signedIn(w, r, session)
}

func (s *server) refresh(w http.ResponseWriter, r *http.Request, _ caller) error {
	session, err := s.deps.Auth.Refresh(r.Context(), cookieValue(r, refreshCookie))
	if err != nil {
		s.cookies.clearSession(w, r)
		return err
	}
	return s.signedIn(w, r, session)
}

func (s *server) logout(w http.ResponseWriter, r *http.Request, _ caller) error {
	err := s.deps.Auth.Logout(r.Context(), cookieValue(r, refreshCookie))
	s.cookies.clearSession(w, r)
	if err != nil {
		return err
	}
	writeNoContent(w)
	return nil
}

func (s *server) me(w http.ResponseWriter, _ *http.Request, who caller) error {
	writeJSON(w, http.StatusOK, who.actor)
	return nil
}

func (s *server) signedIn(w http.ResponseWriter, r *http.Request, session auth.Session) error {
	s.cookies.setSession(w, r, session)
	writeJSON(w, http.StatusOK, toUser(session.User))
	return nil
}

// oidcStart redirects to the identity provider. With link=1 a signed-in user
// is attaching SSO to their account.
func (s *server) oidcStart(w http.ResponseWriter, r *http.Request, who caller) error {
	var linkUserID int64
	if r.URL.Query().Get("link") == "1" {
		if !who.actor.IsHuman() {
			return auth.ErrUnauthorized
		}
		linkUserID = who.user.ID
	}
	redirect, sealed, err := s.deps.OIDC.Start(r.Context(), linkUserID)
	if err != nil {
		s.deps.Logger.Error("sso start failed", "error", err)
		http.Redirect(w, r, signInErrorPage+ssoFailed, http.StatusFound)
		return nil
	}
	s.cookies.setFlow(w, r, sealed)
	http.Redirect(w, r, redirect, http.StatusFound)
	return nil
}

// oidcCallback always ends in a redirect: errors become a code the login page
// explains, because the browser arrived here by navigation, not fetch.
func (s *server) oidcCallback(w http.ResponseWriter, r *http.Request, _ caller) error {
	query := r.URL.Query()
	sealed := cookieValue(r, oidcCookie)
	s.cookies.clearFlow(w, r)
	if query.Get("error") != "" {
		http.Redirect(w, r, signInErrorPage+ssoDenied, http.StatusFound)
		return nil
	}
	identity, flow, err := s.deps.OIDC.Finish(r.Context(), sealed, query.Get("state"), query.Get("code"))
	if err != nil {
		s.deps.Logger.Warn("sso callback rejected", "error", err)
		http.Redirect(w, r, signInErrorPage+ssoFailed, http.StatusFound)
		return nil
	}
	destination := s.completeSSO(w, r, identity, flow)
	http.Redirect(w, r, destination, http.StatusFound)
	return nil
}

func (s *server) completeSSO(w http.ResponseWriter, r *http.Request, identity auth.Identity, flow auth.OIDCFlow) string {
	if flow.LinkUserID != 0 {
		if err := s.deps.Auth.LinkIdentity(r.Context(), flow.LinkUserID, identity); err != nil {
			s.deps.Logger.Warn("sso link failed", "user", flow.LinkUserID, "error", err)
			return signInErrorPage + url.QueryEscape(ssoFailed)
		}
		return afterLink
	}
	session, err := s.deps.Auth.SignInWithIdentity(r.Context(), identity)
	if errors.Is(err, auth.ErrNotLinked) {
		return signInErrorPage + ssoNotLinked
	}
	if err != nil {
		s.deps.Logger.Error("sso sign-in failed", "error", err)
		return signInErrorPage + ssoFailed
	}
	s.cookies.setSession(w, r, session)
	return afterSignIn
}
