package reposource

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// CI limits.
const (
	// maxLogDownload caps how much of a job log the board reads to find its
	// tail; beyond it the log is cut and the tail is the last bytes read.
	maxLogDownload = 64 << 20
	logChunk       = 64 << 10
	// maxBranchLength matches git's practical ref limit.
	maxBranchLength = 255
	// ciListLimit caps one listing page.
	ciListLimit = 100
)

var (
	repoFullName = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}/[A-Za-z0-9_.-]{1,100}$`)
	ansiEscape   = regexp.MustCompile("\x1b\\[[0-9;]*[A-Za-z]")
	// logTimestamp is the RFC 3339 stamp GitHub puts on every log line.
	logTimestamp = regexp.MustCompile(`(?m)^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(\.\d+)?Z `)
)

// post sends a body-less POST, as CI actions are, and expects a 2xx answer.
func (g jsonGetter) post(ctx context.Context, target string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, nil)
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
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxIdentityBytes))
	if response.StatusCode/100 != 2 {
		return statusError(response.StatusCode)
	}
	return nil
}

// redirectTarget asks target for a redirect, as GitHub answers a log
// download, and returns where it points. The redirect is not followed with
// the token: the caller fetches it without credentials.
func (g jsonGetter) redirectTarget(ctx context.Context, target string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", providerError("could not build the request")
	}
	for key, value := range g.headers {
		request.Header.Set(key, value)
	}
	response, err := g.http.Do(request)
	if err != nil {
		return "", unreachable(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusFound && response.StatusCode != http.StatusSeeOther && response.StatusCode != http.StatusTemporaryRedirect {
		return "", statusError(response.StatusCode)
	}
	location, err := url.Parse(response.Header.Get("Location"))
	if err != nil || location.Scheme != "https" || location.Host == "" {
		return "", providerError("the log download did not point to an https address")
	}
	return location.String(), nil
}

// fetchTail downloads target without credentials and returns its last
// maxBytes, cleaned for reading.
func fetchTail(ctx context.Context, client *http.Client, target string, maxBytes int) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", providerError("could not build the request")
	}
	response, err := client.Do(request)
	if err != nil {
		return "", unreachable(err)
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return "", statusError(response.StatusCode)
	}
	text, err := readFailuresAndTail(io.LimitReader(response.Body, maxLogDownload), maxBytes)
	if err != nil {
		return "", unreachable(err)
	}
	return text, nil
}

// readTail keeps the last maxBytes of a stream without holding all of it.
// It keeps twice the tail before trimming, so trimming is rare.
func readTail(reader io.Reader, maxBytes int) ([]byte, error) {
	var kept []byte
	chunk := make([]byte, logChunk)
	for {
		n, err := reader.Read(chunk)
		kept = append(kept, chunk[:n]...)
		if len(kept) > 2*maxBytes+logChunk {
			kept = append([]byte(nil), kept[len(kept)-maxBytes:]...)
		}
		if err == io.EOF {
			return kept, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

// cleanLog strips colour codes and per-line timestamps (tokens the reader
// pays for and never needs), then keeps whole lines from the last maxBytes.
func cleanLog(raw []byte, maxBytes int) string {
	text := ansiEscape.ReplaceAllString(string(raw), "")
	text = logTimestamp.ReplaceAllString(strings.ReplaceAll(text, "\r\n", "\n"), "")
	if len(text) > maxBytes {
		text = text[len(text)-maxBytes:]
		if newline := strings.IndexByte(text, '\n'); newline >= 0 {
			text = text[newline+1:]
		}
	}
	return strings.ToValidUTF8(strings.TrimSpace(text), "")
}

// CheckRepoName refuses a repository name that could walk a request off the
// provider's API.
func CheckRepoName(fullName string) error {
	if !repoFullName.MatchString(fullName) || strings.Contains(fullName, "..") {
		return domain.Invalidf("%q is not an owner/name repository", fullName)
	}
	return nil
}

// escapeRef escapes a branch name segment by segment, keeping its slashes.
// Git already forbids ".." and empty segments in a branch name; refusing
// them here keeps a crafted name from walking off the API path.
func escapeRef(ref string) (string, error) {
	segments := strings.Split(ref, "/")
	for i, segment := range segments {
		if segment == "" || segment == "." || segment == ".." || len(ref) > maxBranchLength {
			return "", domain.Invalidf("%q is not a branch name", ref)
		}
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/"), nil
}
