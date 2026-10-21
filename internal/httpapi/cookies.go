package httpapi

import (
	"net/http"
	"time"

	"github.com/rbretschneider/ownership_board/internal/auth"
)

// Cookie names and paths. The refresh cookie is scoped to the auth endpoints
// so it is never sent with ordinary API calls.
const (
	accessCookie   = "ob_access"
	refreshCookie  = "ob_refresh"
	oidcCookie     = "ob_oidc"
	accessPath     = "/"
	refreshPath    = "/api/v1/auth"
	oidcCookiePath = "/api/v1/auth/oidc"
	oidcCookieLife = 10 * time.Minute
)

type cookieJar struct {
	network network
}

func (c cookieJar) setSession(w http.ResponseWriter, r *http.Request, session auth.Session) {
	c.set(w, r, accessCookie, accessPath, session.AccessToken, session.AccessExpires)
	c.set(w, r, refreshCookie, refreshPath, session.RefreshToken, session.RefreshExpiry)
}

func (c cookieJar) clearSession(w http.ResponseWriter, r *http.Request) {
	c.clear(w, r, accessCookie, accessPath)
	c.clear(w, r, refreshCookie, refreshPath)
}

func (c cookieJar) setFlow(w http.ResponseWriter, r *http.Request, sealed string) {
	c.set(w, r, oidcCookie, oidcCookiePath, sealed, time.Now().Add(oidcCookieLife))
}

func (c cookieJar) clearFlow(w http.ResponseWriter, r *http.Request) {
	c.clear(w, r, oidcCookie, oidcCookiePath)
}

func (c cookieJar) set(w http.ResponseWriter, r *http.Request, name, path, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: value, Path: path, Expires: expires, MaxAge: int(time.Until(expires).Seconds()),
		HttpOnly: true, Secure: c.network.isHTTPS(r), SameSite: http.SameSiteLaxMode,
	})
}

func (c cookieJar) clear(w http.ResponseWriter, r *http.Request, name, path string) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: "", Path: path, MaxAge: -1,
		HttpOnly: true, Secure: c.network.isHTTPS(r), SameSite: http.SameSiteLaxMode,
	})
}

func cookieValue(r *http.Request, name string) string {
	cookie, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}
