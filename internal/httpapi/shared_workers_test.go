package httpapi_test

import (
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
