package httpapi_test

import (
	"net/http"
	"testing"
)

func TestStatsAreForPeopleAndCountRecordedHistory(t *testing.T) {
	browser, token := signedIn(t)
	browser.expect(http.StatusCreated, "POST", "/api/v1/tickets", `{"kind":"story","title":"Refine me"}`)
	browser.asAgent(token).expect(http.StatusOK, "POST", "/api/v1/agent/claim", "")

	report := browser.expect(http.StatusOK, "GET", "/api/v1/stats?days=7&tz_offset=-300", "")

	if report["window_days"] != float64(7) {
		t.Fatalf("window_days = %v, want 7", report["window_days"])
	}
	owner := report["owner"].(map[string]any)
	if owner["created"] != float64(1) {
		t.Fatalf("owner created = %v, want 1", owner["created"])
	}
	claims := owner["claims"].([]any)
	if len(claims) != 1 || claims[0].(map[string]any)["name"] != "refine" {
		t.Fatalf("owner claims = %v, want one refine claim", claims)
	}
	if autonomous := report["autonomous"].(map[string]any); autonomous["created"] != float64(0) {
		t.Fatalf("autonomous created = %v, want 0", autonomous["created"])
	}
	browser.asAgent(token).expect(http.StatusForbidden, "GET", "/api/v1/stats", "")
}

func TestStatsRejectAWindowOutOfRange(t *testing.T) {
	browser, _ := signedIn(t)
	browser.expect(http.StatusBadRequest, "GET", "/api/v1/stats?days=0", "")
	browser.expect(http.StatusBadRequest, "GET", "/api/v1/stats?days=week", "")
	browser.expect(http.StatusBadRequest, "GET", "/api/v1/stats?tz_offset=9999", "")
}
