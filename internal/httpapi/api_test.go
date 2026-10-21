package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/auth"
	"github.com/rbretschneider/ownership_board/internal/broker"
	"github.com/rbretschneider/ownership_board/internal/clock"
	"github.com/rbretschneider/ownership_board/internal/httpapi"
	"github.com/rbretschneider/ownership_board/internal/mcp"
	"github.com/rbretschneider/ownership_board/internal/store/sqlite"
)

// apiClient talks to a test server as a browser (cookies + Origin) or as an
// agent (bearer token).
type apiClient struct {
	t      *testing.T
	base   string
	http   *http.Client
	token  string
	origin string
}

func newServer(t *testing.T) *apiClient {
	t.Helper()
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	clk := clock.System{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	authService, err := auth.NewService(sqlite.NewAuthStore(db), "0123456789abcdef0123456789abcdef", clk, auth.NewThrottle(clk))
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	hub := broker.New()
	services := mcp.Services{
		Board:  app.NewBoardService(db, clk, hub),
		Agents: app.NewAgentService(db, clk, hub, hub),
		Repos:  app.NewRepoService(db, clk, nil, logger),
	}
	handler := httpapi.New(httpapi.Deps{
		Board: services.Board, Agents: services.Agents, Repos: services.Repos, Auth: authService,
		OIDC: auth.NewOIDC(auth.OIDCConfig{}, authService), Broker: hub, Health: db, Logger: logger,
		MCP: mcp.NewServer("test", logger, mcp.BoardTools(services)...),
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	jar, _ := cookiejar.New(nil)
	return &apiClient{t: t, base: server.URL, http: &http.Client{Jar: jar}, origin: server.URL}
}

func (c *apiClient) asAgent(token string) *apiClient {
	return &apiClient{t: c.t, base: c.base, http: &http.Client{}, token: token}
}

func (c *apiClient) withOrigin(origin string) *apiClient {
	copied := *c
	copied.origin = origin
	return &copied
}

func (c *apiClient) do(method, path, body string) (int, map[string]any) {
	c.t.Helper()
	request, err := http.NewRequest(method, c.base+path, strings.NewReader(body))
	if err != nil {
		c.t.Fatalf("request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if c.origin != "" {
		request.Header.Set("Origin", c.origin)
	}
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.http.Do(request)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer response.Body.Close()
	var decoded map[string]any
	_ = json.NewDecoder(response.Body).Decode(&decoded)
	return response.StatusCode, decoded
}

func (c *apiClient) expect(status int, method, path, body string) map[string]any {
	c.t.Helper()
	got, decoded := c.do(method, path, body)
	if got != status {
		c.t.Fatalf("%s %s = %d (%v), want %d", method, path, got, decoded, status)
	}
	return decoded
}

// signedIn returns a browser client with an account and an agent token.
func signedIn(t *testing.T) (*apiClient, string) {
	t.Helper()
	browser := newServer(t)
	browser.expect(http.StatusOK, "POST", "/api/v1/auth/setup", `{"name":"Owner","email":"owner@example.com","password":"a long enough password"}`)
	token := browser.expect(http.StatusCreated, "POST", "/api/v1/tokens", `{"name":"loop"}`)["secret"].(string)
	return browser, token
}

func TestHealthIsPublic(t *testing.T) {
	newServer(t).expect(http.StatusOK, "GET", "/health", "")
}

func TestAPIIsDeniedWithoutCredentials(t *testing.T) {
	anonymous := newServer(t)
	for _, path := range []string{"/api/v1/board", "/api/v1/tokens", "/api/v1/repos", "/api/v1/events/stream"} {
		anonymous.expect(http.StatusUnauthorized, "GET", path, "")
	}
	anonymous.expect(http.StatusNotFound, "GET", "/api/v1/not-a-route", "")
}

func TestCookieWritesNeedTheBoardsOrigin(t *testing.T) {
	browser, _ := signedIn(t)
	browser.withOrigin("https://evil.example").expect(http.StatusForbidden, "POST", "/api/v1/tickets", `{"kind":"bug","title":"x"}`)
	browser.withOrigin("").expect(http.StatusForbidden, "POST", "/api/v1/tickets", `{"kind":"bug","title":"x"}`)
	browser.expect(http.StatusCreated, "POST", "/api/v1/tickets", `{"kind":"bug","title":"x"}`)
}

func TestAgentTokensCannotUsePeopleOnlyRoutes(t *testing.T) {
	browser, token := signedIn(t)
	agent := browser.asAgent(token)
	agent.expect(http.StatusOK, "GET", "/api/v1/board", "")
	agent.expect(http.StatusForbidden, "GET", "/api/v1/tokens", "")
	agent.expect(http.StatusForbidden, "POST", "/api/v1/tickets/1/answers", `{"answers":[]}`)
	browser.expect(http.StatusForbidden, "POST", "/api/v1/agent/claim", `{}`)
	browser.asAgent("ob_pat_forged").expect(http.StatusUnauthorized, "GET", "/api/v1/board", "")
}

func TestClaimReturnsNoContentWhenIdle(t *testing.T) {
	browser, token := signedIn(t)
	browser.asAgent(token).expect(http.StatusNoContent, "POST", "/api/v1/agent/claim", "")
}

func TestUnknownFieldsAreRejected(t *testing.T) {
	browser, _ := signedIn(t)
	body := browser.expect(http.StatusBadRequest, "POST", "/api/v1/tickets", `{"kind":"bug","title":"x","priority":"high"}`)
	if !strings.Contains(body["error"].(map[string]any)["message"].(string), "priority") {
		t.Fatalf("error does not name the bad field: %v", body)
	}
}

func TestEditCanClearTheRepo(t *testing.T) {
	browser, _ := signedIn(t)
	repo := browser.expect(http.StatusCreated, "POST", "/api/v1/repos", `{"full_name":"acme/api"}`)
	ticket := browser.expect(http.StatusCreated, "POST", "/api/v1/tickets", `{"kind":"story","title":"x","repo_id":`+jsonNumber(repo["id"])+`}`)
	path := "/api/v1/tickets/" + jsonNumber(ticket["id"])
	if edited := browser.expect(http.StatusOK, "PATCH", path, `{"title":"y"}`); edited["repo_id"] == nil {
		t.Fatal("an absent repo_id cleared the repo")
	}
	if cleared := browser.expect(http.StatusOK, "PATCH", path, `{"repo_id":null}`); cleared["repo_id"] != nil {
		t.Fatalf("repo_id null did not clear: %v", cleared["repo_id"])
	}
}

func TestMCPToolCallThroughTheEndpoint(t *testing.T) {
	browser, token := signedIn(t)
	browser.expect(http.StatusCreated, "POST", "/api/v1/tickets", `{"kind":"story","title":"Refine me"}`)
	reply := browser.asAgent(token).expect(http.StatusOK, "POST", "/mcp",
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"claim_work","arguments":{}}}`)
	result := reply["result"].(map[string]any)
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if result["isError"].(bool) || !strings.Contains(text, "# #1 Refine me") {
		t.Fatalf("claim_work result: %v", result)
	}
}

func TestMCPRejectsPeople(t *testing.T) {
	browser, _ := signedIn(t)
	browser.expect(http.StatusForbidden, "POST", "/mcp", `{"jsonrpc":"2.0","id":1,"method":"ping"}`)
}

func jsonNumber(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
