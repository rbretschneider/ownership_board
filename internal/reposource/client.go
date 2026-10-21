// Package reposource lists repositories from hosting providers.
package reposource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Shared limits for provider calls.
const (
	// maxPages stops a misbehaving API from paging forever.
	maxPages = 100
	// maxErrorBody is how much of a failed response is kept for the message.
	maxErrorBody = 300
	// maxResponseBytes caps one page so a hostile server cannot exhaust memory.
	maxResponseBytes = 16 << 20
)

// jsonGetter fetches a URL and decodes its JSON body.
type jsonGetter struct {
	http    *http.Client
	headers map[string]string
}

func (g jsonGetter) get(ctx context.Context, url string, out any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	for key, value := range g.headers {
		request.Header.Set(key, value)
	}
	response, err := g.http.Do(request)
	if err != nil {
		return fmt.Errorf("call %s: %w", hostOf(url), err)
	}
	defer response.Body.Close()
	body := io.LimitReader(response.Body, maxResponseBytes)
	if response.StatusCode/100 != 2 {
		return statusError(response.StatusCode, body)
	}
	if err := json.NewDecoder(body).Decode(out); err != nil {
		return fmt.Errorf("decode response from %s: %w", hostOf(url), err)
	}
	return nil
}

func statusError(status int, body io.Reader) error {
	snippet, _ := io.ReadAll(io.LimitReader(body, maxErrorBody))
	text := strings.TrimSpace(string(snippet))
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return fmt.Errorf("the token was rejected (HTTP %d); check it is valid and can read repositories: %s", status, text)
	}
	return fmt.Errorf("HTTP %d: %s", status, text)
}

func hostOf(rawURL string) string {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(rawURL, "https://"), "http://")
	host, _, _ := strings.Cut(trimmed, "/")
	return host
}
