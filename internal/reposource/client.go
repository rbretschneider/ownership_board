// Package reposource lists repositories from hosting providers.
package reposource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"
)

// Shared limits for provider calls.
const (
	// maxPages stops a misbehaving API from paging forever.
	maxPages = 100
	// maxResponseBytes caps one page so a hostile server cannot exhaust memory.
	maxResponseBytes = 16 << 20
	dialTimeout      = 10 * time.Second
)

// ErrUnreachable is the only thing a caller learns about a failed connection.
// Telling "refused" from "no such host" from "timed out" would turn the board
// into a port scanner for whoever can type a URL into it.
var ErrUnreachable error = &ProviderError{Message: "could not reach the server at that address"}

// errForbiddenAddress is returned at dial time for loopback, link-local,
// unspecified and multicast destinations.
var errForbiddenAddress = errors.New("destination address is not allowed")

// ClientOptions configures NewHTTPClient.
type ClientOptions struct {
	Timeout time.Duration
	// AllowLoopback permits 127.0.0.0/8 and ::1. Only tests set it.
	AllowLoopback bool
}

// NewHTTPClient builds the client every provider call uses. It never follows
// redirects (a redirect would carry the Authorization header, and the token,
// wherever it points) and refuses, at connect time, loopback, link-local
// (including the 169.254.169.254 metadata address), unspecified and multicast
// destinations. It does NOT block private LAN addresses, the Docker bridge or
// the host's LAN IP, because self-hosted forges live there; operators narrow
// that with FORGEJO_ALLOWED_HOSTS. Checking at connect time rather than when a
// URL is saved also defeats DNS rebinding.
func NewHTTPClient(options ClientOptions) *http.Client {
	dialer := &net.Dialer{Timeout: dialTimeout, Control: addressGuard(options.AllowLoopback)}
	transport := &http.Transport{
		Proxy: nil, DialContext: dialer.DialContext, ForceAttemptHTTP2: true,
		TLSHandshakeTimeout: dialTimeout, ResponseHeaderTimeout: options.Timeout, MaxIdleConns: 10,
	}
	return noRedirects(&http.Client{Timeout: options.Timeout, Transport: transport})
}

func addressGuard(allowLoopback bool) func(network, address string, _ syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return errForbiddenAddress
		}
		ip := net.ParseIP(host)
		if ip == nil || isForbiddenIP(ip, allowLoopback) {
			return errForbiddenAddress
		}
		return nil
	}
}

func isForbiddenIP(ip net.IP, allowLoopback bool) bool {
	if ip.IsLoopback() {
		return !allowLoopback
	}
	return ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast()
}

// noRedirects copies client so it never follows a redirect.
func noRedirects(client *http.Client) *http.Client {
	copied := *client
	copied.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &copied
}

// jsonGetter fetches a URL and decodes its JSON body.
type jsonGetter struct {
	http    *http.Client
	headers map[string]string
	// maxBytes caps the response body; zero means maxResponseBytes.
	maxBytes int64
}

func (g jsonGetter) get(ctx context.Context, target string, out any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return providerError("could not build the request")
	}
	request.Header.Set("Accept", "application/json")
	for key, value := range g.headers {
		request.Header.Set(key, value)
	}
	response, err := g.http.Do(request)
	if err != nil {
		return unreachable(err)
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return statusError(response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, g.limit())).Decode(out); err != nil {
		return providerError("the server's answer was not the JSON expected")
	}
	return nil
}

// statusError explains a status code without echoing any response bytes.
func statusError(status int) error {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return providerError(fmt.Sprintf("the token was rejected (HTTP %d); check it is valid and has the scopes listed in Settings", status))
	case status == http.StatusNotFound:
		return &ProviderError{Message: "not found (HTTP 404); check the address", Status: http.StatusNotFound}
	case status/100 == 3:
		return providerError(fmt.Sprintf("the server answered with a redirect (HTTP %d), which the board does not follow; use the final address", status))
	default:
		return providerError(fmt.Sprintf("the server answered HTTP %d", status))
	}
}

// unreachable hides connection detail but keeps context cancellation visible.
func unreachable(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	return ErrUnreachable
}

func hostOf(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Host
}

func (g jsonGetter) limit() int64 {
	if g.maxBytes > 0 {
		return g.maxBytes
	}
	return maxResponseBytes
}

// withLimit returns a copy reading at most maxBytes of a response, for small
// answers such as "who am I", so a hostile server cannot make the board buffer
// megabytes.
func (g jsonGetter) withLimit(maxBytes int64) jsonGetter {
	g.maxBytes = maxBytes
	return g
}
