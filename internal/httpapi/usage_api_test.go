package httpapi_test

import (
	"net/http"
	"testing"
)

func TestAgentsReportTokenUsageShownOnTheTicket(t *testing.T) {
	browser, token := signedIn(t)
	browser.expect(http.StatusCreated, "POST", "/api/v1/tickets", `{"kind":"story","title":"Heavy"}`)
	reporter := browser.asAgent(token)
	reporter.worker = "usage_reporter"

	claude := `{"call_key":"claude_1/1/100/author_0","provider":"claude","model":"claude-sonnet-5-5",` +
		`"input_tokens":300,"cache_write_tokens":40000,"cache_read_tokens":2000000,"output_tokens":9700,"cost_usd":1.25}`
	got := reporter.expect(http.StatusOK, "POST", "/api/v1/agent/tickets/1/usage", claude)
	if jsonNumber(got["calls"]) != "1" || jsonNumber(got["weighted_tokens"]) != "250000" {
		t.Fatalf("usage after one call = %v, want 1 call and 250000 weighted tokens", got)
	}
	// Resending the same call is ignored, so a reporter can sweep logs again.
	reporter.expect(http.StatusOK, "POST", "/api/v1/agent/tickets/1/usage", claude)
	reporter.expect(http.StatusOK, "POST", "/api/v1/agent/tickets/1/usage",
		`{"call_key":"codex_1/1/200/review_0","provider":"codex","model":"gpt-6.1-sol","input_tokens":50000}`)

	board := browser.expect(http.StatusOK, "GET", "/api/v1/board", "")
	usage := board["tickets"].([]any)[0].(map[string]any)["usage"].(map[string]any)
	if jsonNumber(usage["calls"]) != "2" || jsonNumber(usage["weighted_tokens"]) != "300000" || jsonNumber(usage["cache_read_tokens"]) != "2000000" {
		t.Fatalf("board usage = %v, want 2 calls, 300000 weighted, 2000000 cache reads", usage)
	}
	detail := browser.expect(http.StatusOK, "GET", "/api/v1/tickets/1", "")
	if jsonNumber(detail["usage"].(map[string]any)["weighted_tokens"]) != "300000" {
		t.Fatalf("detail usage = %v", detail["usage"])
	}

	reporter.expect(http.StatusBadRequest, "POST", "/api/v1/agent/tickets/1/usage", `{"call_key":"x","provider":"gemini"}`)
	reporter.expect(http.StatusBadRequest, "POST", "/api/v1/agent/tickets/1/usage", `{"call_key":"x","provider":"codex","input_tokens":-1}`)
	reporter.expect(http.StatusBadRequest, "POST", "/api/v1/agent/tickets/1/usage", `{"call_key":"","provider":"codex"}`)
	reporter.expect(http.StatusNotFound, "POST", "/api/v1/agent/tickets/99/usage", `{"call_key":"y","provider":"codex"}`)
	browser.expect(http.StatusForbidden, "POST", "/api/v1/agent/tickets/1/usage", `{"call_key":"z","provider":"codex"}`)
}
