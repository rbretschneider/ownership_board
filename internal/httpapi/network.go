package httpapi

import (
	"net"
	"net/http"
	"strings"
)

// network reads client facts from a request, trusting forwarded headers only
// from a private-range proxy and only as many hops as configured.
type network struct {
	trustedHops int
}

// clientIP is the address used to key sign-in throttling.
func (n network) clientIP(r *http.Request) string {
	remote := remoteHost(r)
	if n.trustedHops == 0 || !isPrivate(remote) {
		return remote
	}
	hops := forwardedFor(r)
	if len(hops) < n.trustedHops {
		return remote
	}
	return hops[len(hops)-n.trustedHops]
}

// isHTTPS decides the Secure cookie flag from the real request scheme, so the
// board still works over plain HTTP on a LAN.
func (n network) isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if n.trustedHops == 0 || !isPrivate(remoteHost(r)) {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]), "https")
}

func remoteHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func forwardedFor(r *http.Request) []string {
	var hops []string
	for _, header := range r.Header.Values("X-Forwarded-For") {
		for _, part := range strings.Split(header, ",") {
			if hop := strings.TrimSpace(part); hop != "" {
				hops = append(hops, hop)
			}
		}
	}
	return hops
}

func isPrivate(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}
