package httpapi_test

import (
	"fmt"
	"net/http"
	"testing"
)

func TestSharedTokenKeepsWorkerLeasesSeparate(t *testing.T) {
	browser, token := signedIn(t)
	for _, title := range []string{"First", "Second"} {
		browser.expect(http.StatusCreated, "POST", "/api/v1/tickets", `{"kind":"story","title":"`+title+`"}`)
	}
	first, second := browser.asAgent(token), browser.asAgent(token)
	first.worker, second.worker = "codex_1", "codex_2"
	first.expect(http.StatusOK, "POST", "/api/v1/agent/claim", "")
	second.expect(http.StatusOK, "POST", "/api/v1/agent/claim", "")
	first.expect(http.StatusOK, "POST", "/api/v1/agent/tickets/1/heartbeat", "")
	second.expect(http.StatusForbidden, "POST", "/api/v1/agent/tickets/1/heartbeat", "")
	second.expect(http.StatusForbidden, "POST", "/api/v1/agent/tickets/1/release", "")
	second.expect(http.StatusOK, "POST", "/api/v1/agent/tickets/2/heartbeat", "")
	other := browser.expect(http.StatusCreated, "POST", "/api/v1/tokens", `{"name":"other-account"}`)["secret"].(string)
	outsider := browser.asAgent(other)
	outsider.worker = "codex_1"
	outsider.expect(http.StatusForbidden, "POST", "/api/v1/agent/tickets/1/release", "")
	first.worker = "invalid/name"
	first.expect(http.StatusBadRequest, "POST", "/api/v1/agent/claim", "")
	browser.expect(http.StatusBadRequest, "POST", "/api/v1/tokens", `{"name":"@worker/1/codex_1"}`)
	repo := jsonNumber(browser.expect(http.StatusCreated, "POST", "/api/v1/repos", `{"full_name":"acme/app"}`)["id"])
	delegate := browser.expect(http.StatusCreated, "POST", "/api/v1/tokens", `{"name":"shared-delegate","is_delegate":true}`)["secret"].(string)
	executor := browser.asAgent(delegate)
	executor.worker = "codex_3"
	reply := executor.expect(http.StatusOK, "POST", "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_autonomous_ticket","arguments":{"kind":"story","title":"Cannot bypass filing budget","spec":"Spec","repo_id":`+repo+`}}}`)
	if reply["result"].(map[string]any)["isError"] != true {
		t.Fatal("worker identity inherited autonomous filing authority")
	}
}

func TestCodexPoolSettingsAreHumanOnlyAndPersistPause(t *testing.T) {
	browser, token := signedIn(t)
	browser.asAgent(token).expect(http.StatusForbidden, "PATCH", "/api/v1/settings", `{"codex_workers":6}`)
	for _, body := range []string{`{"codex_workers":-1}`, `{"codex_workers":7}`, `{"codex_workers":1.5}`} {
		browser.expect(http.StatusBadRequest, "PATCH", "/api/v1/settings", body)
	}
	browser.expect(http.StatusOK, "PATCH", "/api/v1/settings", `{"codex_workers":0}`)
	if got := browser.expect(http.StatusOK, "GET", "/api/v1/settings", "")["codex_workers"]; got != float64(0) {
		t.Fatalf("pause not persisted: %v", got)
	}
	browser.expect(http.StatusOK, "PATCH", "/api/v1/settings", `{"codex_workers":6,"autonomous_in_flight_cap":6}`)
	if got := browser.expect(http.StatusOK, "GET", "/api/v1/settings", "")["codex_workers"]; got != float64(6) {
		t.Fatalf("worker count not persisted: %v", got)
	}
}

func TestWindowsRunnerMasterSwitchIsHumanOnly(t *testing.T) {
	browser, token := signedIn(t)
	if got := browser.expect(http.StatusOK, "GET", "/api/v1/settings", "")["windows_runner_enabled"]; got != false {
		t.Fatal("Windows should default off")
	}
	browser.asAgent(token).expect(http.StatusForbidden, "PATCH", "/api/v1/settings", `{"windows_runner_enabled":true}`)
	seedWindowsSettingsHost(t, browser, false)
	browser.expect(http.StatusOK, "PATCH", "/api/v1/settings", `{"windows_runner_enabled":true}`)
	if got := browser.expect(http.StatusOK, "GET", "/api/v1/settings", "")["windows_runner_enabled"]; got != true {
		t.Fatal("enable not persisted")
	}
	browser.expect(http.StatusOK, "PATCH", "/api/v1/settings", `{"windows_runner_enabled":false}`)
	if got := browser.expect(http.StatusOK, "GET", "/api/v1/settings", "")["windows_runner_enabled"]; got != false {
		t.Fatal("disable not persisted")
	}
}

func TestAccountWeeklyCeilingsAndClaudePool(t *testing.T) {
	browser, token := signedIn(t)
	browser.asAgent(token).expect(http.StatusForbidden, "PATCH", "/api/v1/settings", `{"claude_workers":2,"codex_weekly_limit":50}`)
	for _, body := range []string{`{"claude_workers":7}`, `{"claude_weekly_limit":101}`, `{"codex_weekly_limit":-1}`, `{"codex_weekly_limit":50.5}`} {
		browser.expect(http.StatusBadRequest, "PATCH", "/api/v1/settings", body)
	}
	browser.expect(http.StatusOK, "PATCH", "/api/v1/settings", `{"claude_workers":2,"codex_weekly_limit":50,"claude_weekly_limit":100}`)
	got := browser.expect(http.StatusOK, "GET", "/api/v1/settings", "")
	if got["claude_workers"] != float64(2) || got["codex_weekly_limit"] != float64(50) || got["claude_weekly_limit"] != float64(100) {
		t.Fatalf("account settings not persisted: %v", got)
	}
	browser.expect(http.StatusOK, "PATCH", "/api/v1/settings", `{"codex_weekly_limit":0,"claude_workers":0}`)
}

func TestWindowsParallelBuildSettings(t *testing.T) {
	browser, token := signedIn(t)
	seedWindowsSettingsHost(t, browser, true)
	if got := browser.expect(http.StatusOK, "GET", "/api/v1/settings", "")["windows_parallel_builds"]; got != float64(3) {
		t.Fatalf("default parallelism %v", got)
	}
	browser.asAgent(token).expect(http.StatusForbidden, "PATCH", "/api/v1/settings", `{"windows_parallel_builds":6}`)
	for _, body := range []string{`{"windows_parallel_builds":0}`, `{"windows_parallel_builds":9}`, `{"windows_parallel_builds":1.5}`} {
		browser.expect(http.StatusBadRequest, "PATCH", "/api/v1/settings", body)
	}
	browser.expect(http.StatusOK, "PATCH", "/api/v1/settings", `{"windows_parallel_builds":6}`)
	got := browser.expect(http.StatusOK, "GET", "/api/v1/settings", "")
	if got["windows_parallel_builds"] != float64(6) || got["windows_runner_enabled"] != true {
		t.Fatalf("Windows scaling changed unrelated switch: %v", got)
	}
}

func seedWindowsSettingsHost(t *testing.T, c *apiClient, enabled bool) int64 {
	t.Helper()
	id := seedBuildHost(t, c)
	if _, err := c.store.SQL().Exec(`UPDATE build_hosts SET os='windows',max_parallel=3 WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/v1/build-hosts/%d", id)
	c.expect(200, "POST", path+"/approve", `{}`)
	c.expect(200, "PATCH", path, fmt.Sprintf(`{"enabled":%t}`, enabled))
	return id
}

func TestWindowsSettingsNoHostCompatibility(t *testing.T) {
	c, _ := signedIn(t)
	got := c.expect(200, "GET", "/api/v1/settings", "")
	if got["windows_parallel_builds"] != float64(0) || got["windows_parallel_build_limit"] != float64(0) || got["windows_runner_enabled"] != false {
		t.Fatal(got)
	}
	c.expect(409, "PATCH", "/api/v1/settings", `{"windows_runner_enabled":true}`)
	c.expect(409, "PATCH", "/api/v1/settings", `{"windows_parallel_builds":1}`)
	c.expect(200, "PATCH", "/api/v1/settings", `{"windows_runner_enabled":false}`)
	c.expect(200, "PATCH", "/api/v1/settings", `{"codex_workers":4}`)
}
