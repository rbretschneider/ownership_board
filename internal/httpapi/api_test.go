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
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/auth"
	"github.com/rbretschneider/ownership_board/internal/broker"
	"github.com/rbretschneider/ownership_board/internal/clock"
	"github.com/rbretschneider/ownership_board/internal/connections"
	"github.com/rbretschneider/ownership_board/internal/httpapi"
	"github.com/rbretschneider/ownership_board/internal/mcp"
	"github.com/rbretschneider/ownership_board/internal/notify"
	"github.com/rbretschneider/ownership_board/internal/reposource"
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
		Repos:  app.NewRepoService(db, clk, noSources{}, logger),
	}
	catalog, err := connections.NewCatalog(connections.CatalogDeps{
		Store: sqlite.NewConnectionStore(db), MasterSecret: "0123456789abcdef0123456789abcdef",
		GitHub: reposource.NewGitHubApp(http.DefaultClient, "", "", "", time.Now), HTTP: http.DefaultClient, Clock: clk, Logger: logger,
	})
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	handler := httpapi.New(httpapi.Deps{
		Board: services.Board, Agents: services.Agents, Repos: services.Repos, Auth: authService,
		OIDC: auth.NewOIDC(auth.OIDCConfig{}, authService), Broker: hub, Health: db, Logger: logger,
		MCP:         mcp.NewServer("test", logger, mcp.BoardTools(services)...),
		Connections: connections.NewService(catalog, services.Repos, notify.Noop{}, connections.Policy{}),
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
	if result["isError"].(bool) || !strings.Contains(text, "# Ticket #1") || !strings.Contains(text, "Refine me") {
		t.Fatalf("claim_work result: %v", result)
	}
}

func TestOnlyPeopleChangeAnAutonomousTicketsFlagOrRepo(t *testing.T) {
	browser, token := signedIn(t)
	api := jsonNumber(browser.expect(http.StatusCreated, "POST", "/api/v1/repos", `{"full_name":"acme/api"}`)["id"])
	web := jsonNumber(browser.expect(http.StatusCreated, "POST", "/api/v1/repos", `{"full_name":"acme/web"}`)["id"])
	delegate := browser.expect(http.StatusCreated, "POST", "/api/v1/tokens", `{"name":"desktop","is_delegate":true}`)
	if delegate["is_delegate"] != true {
		t.Fatalf("token is not a delegate: %v", delegate)
	}
	reply := browser.asAgent(delegate["secret"].(string)).expect(http.StatusOK, "POST", "/mcp",
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_autonomous_ticket","arguments":`+
			`{"kind":"story","title":"Agreed","spec":"Spec.","repo_id":`+api+`}}}`)
	if reply["result"].(map[string]any)["isError"].(bool) {
		t.Fatalf("create_autonomous_ticket: %v", reply)
	}

	agent := browser.asAgent(token)
	agent.expect(http.StatusOK, "POST", "/api/v1/agent/claim", "")
	agent.expect(http.StatusForbidden, "PATCH", "/api/v1/tickets/1", `{"is_autonomous":false}`)
	agent.expect(http.StatusForbidden, "PATCH", "/api/v1/tickets/1", `{"repo_id":`+web+`}`)
	if edited := browser.expect(http.StatusOK, "PATCH", "/api/v1/tickets/1", `{"is_autonomous":false}`); edited["is_autonomous"] != false {
		t.Fatalf("the owner could not turn autonomy off: %v", edited["is_autonomous"])
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

type noSources struct{}

func (noSources) Sources(context.Context) ([]app.RepoSource, error) {
	return nil, nil
}

func TestConnectionsArePeopleOnly(t *testing.T) {
	browser, token := signedIn(t)
	agent := browser.asAgent(token)
	routes := []struct{ method, path, body string }{
		{"GET", "/api/v1/connections", ""},
		{"POST", "/api/v1/connections/sync", ""},
		{"POST", "/api/v1/connections/github/device", ""},
		{"POST", "/api/v1/connections/github/device/x/poll", ""},
		{"POST", "/api/v1/connections/github/device/x/confirm", ""},
		{"DELETE", "/api/v1/connections/github/device/x", ""},
		{"PUT", "/api/v1/connections/forgejo", `{"base_url":"http://192.168.1.50:3021","token":"x"}`},
		{"PUT", "/api/v1/connections/bitbucket", `{"email":"","token":"x"}`},
		{"DELETE", "/api/v1/connections/github", ""},
	}
	for _, route := range routes {
		agent.expect(http.StatusForbidden, route.method, route.path, route.body)
	}
}

func TestConnectionListShowsEveryProvider(t *testing.T) {
	browser, _ := signedIn(t)
	status, _ := browser.do("GET", "/api/v1/connections", "")
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	request, _ := http.NewRequest("GET", browser.base+"/api/v1/connections", nil)
	response, err := browser.http.Do(request)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	defer response.Body.Close()
	var views []map[string]any
	if err := json.NewDecoder(response.Body).Decode(&views); err != nil || len(views) != 3 {
		t.Fatalf("views %v err %v", views, err)
	}
	if views[0]["provider"] != "github" || views[0]["is_available"] != false {
		t.Fatalf("github view %v", views[0])
	}
}

func TestGitHubSignInWithoutClientIDExplainsItself(t *testing.T) {
	browser, _ := signedIn(t)
	body := browser.expect(http.StatusConflict, "POST", "/api/v1/connections/github/device", "")
	if !strings.Contains(body["error"].(map[string]any)["message"].(string), "GITHUB_APP_CLIENT_ID") {
		t.Fatalf("error %v", body)
	}
}

func TestAgentsCannotDeleteTraceRecords(t *testing.T) {
	browser, token := signedIn(t)
	browser.expect(http.StatusCreated, "POST", "/api/v1/tickets", `{"kind":"story","title":"x"}`)
	agent := browser.asAgent(token)
	for _, path := range []string{"/api/v1/tickets/1/work-links/1", "/api/v1/tickets/1/decisions/1", "/api/v1/tickets/1/relations/caused_by/2"} {
		agent.expect(http.StatusForbidden, "DELETE", path, "")
	}
	agent.expect(http.StatusForbidden, "POST", "/api/v1/tickets/1/decisions", `{"summary":"agent without lease"}`)
}

func TestSettingsAreReadByAnyoneAndChangedByPeople(t *testing.T) {
	browser, token := signedIn(t)
	agent := browser.asAgent(token)
	if got := agent.expect(http.StatusOK, "GET", "/api/v1/settings", "")["autonomous_in_flight_cap"]; got != float64(1) {
		t.Fatalf("default cap %v, want 1", got)
	}
	agent.expect(http.StatusForbidden, "PATCH", "/api/v1/settings", `{"autonomous_in_flight_cap":2}`)
	for _, body := range []string{
		`{"autonomous_in_flight_cap":0}`, `{"autonomous_in_flight_cap":11}`, `{"autonomous_in_flight_cap":-1}`,
		`{"autonomous_in_flight_cap":99999999999999999999}`, `{"autonomous_in_flight_cap":"2"}`, `{"autonomous_in_flight_cap":1.5}`,
		`{"in_flight":2}`,
	} {
		browser.expect(http.StatusBadRequest, "PATCH", "/api/v1/settings", body)
	}
	if got := browser.expect(http.StatusOK, "PATCH", "/api/v1/settings", `{"autonomous_in_flight_cap":2}`)["autonomous_in_flight_cap"]; got != float64(2) {
		t.Fatalf("cap after the change %v, want 2", got)
	}
	if got := browser.expect(http.StatusOK, "GET", "/api/v1/settings", "")["autonomous_in_flight_cap"]; got != float64(2) {
		t.Fatalf("stored cap %v, want 2", got)
	}
}

func TestTicketsListWhatTheyAreWaitingOn(t *testing.T) {
	browser, _ := signedIn(t)
	api := jsonNumber(browser.expect(http.StatusCreated, "POST", "/api/v1/repos", `{"full_name":"acme/api"}`)["id"])
	b := jsonNumber(browser.expect(http.StatusCreated, "POST", "/api/v1/tickets", `{"kind":"story","title":"b","repo_id":`+api+`}`)["id"])
	a := browser.expect(http.StatusCreated, "POST", "/api/v1/tickets", `{"kind":"story","title":"a","repo_id":`+api+`,"depends_on":[`+b+`]}`)
	if got := jsonNumber(a["waiting_on"]); got != "["+b+"]" {
		t.Fatalf("created ticket waiting_on %s, want [%s]", got, b)
	}
	body := browser.expect(http.StatusBadRequest, "POST", "/api/v1/tickets", `{"kind":"story","title":"c","repo_id":`+api+`,"depends_on":[999]}`)
	if !strings.Contains(body["error"].(map[string]any)["message"].(string), "999") {
		t.Fatalf("error does not name the unknown id: %v", body)
	}

	board := browser.expect(http.StatusOK, "GET", "/api/v1/board", "")
	tickets := board["tickets"].([]any)
	if len(tickets) != 2 {
		t.Fatalf("%d tickets on the board, want 2 (a refused create added one)", len(tickets))
	}
	for _, raw := range tickets {
		ticket := raw.(map[string]any)
		want := "[]"
		if jsonNumber(ticket["id"]) == jsonNumber(a["id"]) {
			want = "[" + b + "]"
		}
		if got := jsonNumber(ticket["waiting_on"]); got != want {
			t.Fatalf("ticket %v waiting_on %s, want %s", ticket["id"], got, want)
		}
	}
	browser.expect(http.StatusConflict, "PATCH", "/api/v1/tickets/"+b, `{"repo_id":null}`)
}

func TestAgentsRemoveOnlyTheDependencyLinksTheyAdded(t *testing.T) {
	browser, token := signedIn(t)
	api := jsonNumber(browser.expect(http.StatusCreated, "POST", "/api/v1/repos", `{"full_name":"acme/api"}`)["id"])
	b := jsonNumber(browser.expect(http.StatusCreated, "POST", "/api/v1/tickets", `{"kind":"story","title":"b","repo_id":`+api+`}`)["id"])
	delegate := browser.asAgent(browser.expect(http.StatusCreated, "POST", "/api/v1/tokens", `{"name":"desktop","is_delegate":true}`)["secret"].(string))
	reply := delegate.expect(http.StatusOK, "POST", "/mcp",
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_autonomous_ticket","arguments":`+
			`{"kind":"story","title":"Agreed","spec":"Spec.","repo_id":`+api+`,"depends_on":[`+b+`]}}}`)
	if reply["result"].(map[string]any)["isError"].(bool) {
		t.Fatalf("create_autonomous_ticket with depends_on: %v", reply)
	}
	a := "2"
	agent := browser.asAgent(token)
	agent.expect(http.StatusForbidden, "DELETE", "/api/v1/tickets/"+a+"/relations/depends_on/"+b, "")
	agent.expect(http.StatusForbidden, "DELETE", "/api/v1/tickets/"+b+"/relations/depends_on/"+a, "")
	agent.expect(http.StatusForbidden, "POST", "/api/v1/tickets/"+a+"/relations", `{"kind":"depends_on","ticket_id":`+b+`}`)
	removed := delegate.expect(http.StatusOK, "DELETE", "/api/v1/tickets/"+b+"/relations/depends_on/"+a, "")
	if got := jsonNumber(removed["relations"]); got != "[]" {
		t.Fatalf("relations after removal %s", got)
	}
	delegate.expect(http.StatusOK, "POST", "/api/v1/tickets/"+a+"/relations", `{"kind":"depends_on","ticket_id":`+b+`}`)
	delegate.expect(http.StatusForbidden, "POST", "/api/v1/tickets/"+b+"/relations", `{"kind":"depends_on","ticket_id":`+a+`}`)
	cycle := browser.expect(http.StatusConflict, "POST", "/api/v1/tickets/"+b+"/relations", `{"kind":"depends_on","ticket_id":`+a+`}`)
	if got := cycle["error"].(map[string]any)["message"].(string); !strings.Contains(got, "#"+b+" -> #"+a+" -> #"+b) {
		t.Fatalf("cycle error does not give the path: %q", got)
	}
	browser.expect(http.StatusOK, "DELETE", "/api/v1/tickets/"+a+"/relations/depends_on/"+b, "")
}

func TestLinkTicketsToolAddsADependencyShownFromBothEnds(t *testing.T) {
	browser, _ := signedIn(t)
	api := jsonNumber(browser.expect(http.StatusCreated, "POST", "/api/v1/repos", `{"full_name":"acme/api"}`)["id"])
	delegate := browser.asAgent(browser.expect(http.StatusCreated, "POST", "/api/v1/tokens", `{"name":"desktop","is_delegate":true}`)["secret"].(string))
	tool := func(name, arguments string) string {
		t.Helper()
		reply := delegate.expect(http.StatusOK, "POST", "/mcp",
			`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"`+name+`","arguments":`+arguments+`}}`)
		result := reply["result"].(map[string]any)
		text := result["content"].([]any)[0].(map[string]any)["text"].(string)
		if result["isError"].(bool) {
			t.Fatalf("%s: %s", name, text)
		}
		return text
	}
	tool("create_autonomous_ticket", `{"kind":"story","title":"First","spec":"Spec.","repo_id":`+api+`}`)
	tool("create_autonomous_ticket", `{"kind":"story","title":"Second","spec":"Spec.","repo_id":`+api+`}`)
	tool("link_tickets", `{"id":2,"kind":"depends_on","other_id":1}`)

	if got := tool("get_ticket", `{"id":2}`); !strings.Contains(got, "depends on #1") {
		t.Fatalf("the dependant's brief does not show the dependency:\n%s", got)
	}
	if got := tool("get_ticket", `{"id":1}`); !strings.Contains(got, "needed by #2") {
		t.Fatalf("the prerequisite's brief does not show its dependant:\n%s", got)
	}
	if got := tool("list_tickets", `{}`); !strings.Contains(got, "waiting on #1") {
		t.Fatalf("the ticket list does not show what #2 waits on:\n%s", got)
	}
}
