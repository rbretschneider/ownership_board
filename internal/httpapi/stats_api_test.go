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

func TestStatsHealthWireContract(t *testing.T) {
	browser, _ := signedIn(t)
	repoID := jsonNumber(browser.expect(http.StatusCreated, "POST", "/api/v1/repos", `{"full_name":"acme/app"}`)["id"])
	browser.expect(http.StatusCreated, "POST", "/api/v1/tickets", `{"kind":"story","title":"unassigned"}`)
	health := func() map[string]any {
		t.Helper()
		report := browser.expect(http.StatusOK, "GET", "/api/v1/stats?days=90", "")
		var found map[string]any
		for _, v := range report["repos"].([]any) {
			r := v.(map[string]any)
			if r["repo_id"] == float64(0) {
				if value, ok := r["health"]; !ok || value != nil {
					t.Fatalf("unassigned health: %v", r)
				}
				continue
			}
			if jsonNumber(r["repo_id"]) == repoID {
				var ok bool
				found, ok = r["health"].(map[string]any)
				if !ok {
					t.Fatalf("health object: %v", r)
				}
			}
		}
		if found == nil {
			t.Fatal("idle configured repository missing")
		}
		return found
	}
	h := health()
	if len(h) != 6 || h["every"] != float64(8) || h["merges_since_check"] != float64(0) || h["next_in"] != float64(8) || h["last_check_ticket"] != nil || h["last_checked_at"] != nil || h["open_follow_ups"] != float64(0) {
		t.Fatalf("initial: %v", h)
	}
	browser.expect(http.StatusOK, "PATCH", "/api/v1/repos/"+repoID, `{"health_check_every":1}`)
	ticket := browser.expect(http.StatusCreated, "POST", "/api/v1/tickets", `{"kind":"story","title":"delivery","repo_id":`+repoID+`}`)
	browser.expect(http.StatusOK, "POST", "/api/v1/tickets/"+jsonNumber(ticket["id"])+"/move", `{"lane":"merged"}`)
	h = health()
	if _, ok := h["last_check_ticket"].(float64); !ok {
		t.Fatalf("ticket numeric: %v", h)
	}
	if _, ok := h["last_checked_at"].(string); !ok {
		t.Fatalf("timestamp string: %v", h)
	}
	if h["last_check_ticket"] == ticket["id"] || h["next_in"] != float64(1) || h["merges_since_check"] != float64(0) {
		t.Fatalf("recorded: %v", h)
	}
	browser.expect(http.StatusOK, "PATCH", "/api/v1/repos/"+repoID, `{"health_check_every":0}`)
	h = health()
	if value, ok := h["next_in"]; !ok || value != nil || h["every"] != float64(0) || h["last_check_ticket"] == nil {
		t.Fatalf("disabled: %v", h)
	}
}
