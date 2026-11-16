package httpapi_test

import (
	"net/http"
	"testing"
)

func TestRunnerNamesListAndRename(t *testing.T) {
	browser, token := signedIn(t)
	worker := browser.asAgent(token)
	worker.worker = "claude_1"
	// A claim names the worker even when there is no work.
	worker.expect(http.StatusNoContent, "POST", "/api/v1/agent/claim", "")

	listed := browser.expect(http.StatusOK, "GET", "/api/v1/runner-names", "")["runners"].([]any)
	if len(listed) != 1 {
		t.Fatalf("runners = %v", listed)
	}
	runner := listed[0].(map[string]any)
	if runner["worker"] != "claude_1" || runner["family"] != "claude" || runner["display_name"] != "Michael" || runner["position"] != float64(1) {
		t.Fatalf("runner = %v", runner)
	}

	worker.expect(http.StatusForbidden, "PATCH", "/api/v1/runner-names/claude_1", `{"display_name":"Scott"}`)
	browser.expect(http.StatusBadRequest, "PATCH", "/api/v1/runner-names/claude_1", `{"display_name":""}`)
	renamed := browser.expect(http.StatusOK, "PATCH", "/api/v1/runner-names/claude_1", `{"display_name":"Scott"}`)
	if renamed["display_name"] != "Scott" || renamed["worker"] != "claude_1" {
		t.Fatalf("renamed = %v", renamed)
	}
	agentView := worker.expect(http.StatusOK, "GET", "/api/v1/runner-names", "")["runners"].([]any)
	if agentView[0].(map[string]any)["display_name"] != "Scott" {
		t.Fatalf("after rename = %v", agentView)
	}
}
