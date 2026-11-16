package httpapi_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestVerdictsOverTheAPIAndMCP(t *testing.T) {
	browser, token := signedIn(t)
	if _, err := browser.store.SQL().Exec(`INSERT INTO repos(id,provider,full_name,created_at) VALUES(7,'manual','example/tools','2026-01-01T00:00:00.000Z')`); err != nil {
		t.Fatal(err)
	}
	browser.expect(http.StatusCreated, "POST", "/api/v1/tickets", `{"kind":"story","title":"Verify me","repo_id":7}`)
	agent := browser.asAgent(token)
	base, merge := strings.Repeat("a", 40), strings.Repeat("b", 40)
	report := fmt.Sprintf(`{"ticket_id":1,"repo":"example/tools","sha":%q,"merge_of":%q}`, merge, base)

	browser.expect(http.StatusBadRequest, "GET", "/api/v1/verdicts", "")
	browser.expect(http.StatusBadRequest, "GET", "/api/v1/verdicts?repo=example/tools&sha=abc", "")
	// Without the lease the board does not take an agent's word.
	agent.expect(http.StatusConflict, "POST", "/api/v1/verdicts/clean-merge", report)
	if _, err := browser.store.SQL().Exec(`UPDATE tickets SET lease_holder='loop', lease_expires_at='2099-01-01T00:00:00.000Z' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	// With the lease, but merge_of has no pass.
	agent.expect(http.StatusConflict, "POST", "/api/v1/verdicts/clean-merge", report)
	if _, err := browser.store.SQL().Exec(`INSERT INTO verdicts(repo,target,sha,status,job_id,ticket_id,source,reported_by,recorded_at)
		VALUES('example/tools','windows',?,'passed',?,1,'build_job','build-host/1','2026-01-01T00:00:00.000Z')`, base, strings.Repeat("c", 64)); err != nil {
		t.Fatal(err)
	}
	recorded := agent.expect(http.StatusOK, "POST", "/api/v1/verdicts/clean-merge", report)["verdicts"].([]any)
	if len(recorded) != 1 || recorded[0].(map[string]any)["status"] != "clean_merge" || recorded[0].(map[string]any)["reported_by"] != "loop" {
		t.Fatalf("clean merge: %v", recorded)
	}

	byCommit := browser.expect(http.StatusOK, "GET", "/api/v1/verdicts?repo=example/tools&sha="+strings.ToUpper(merge), "")["verdicts"].([]any)
	if len(byCommit) != 1 || byCommit[0].(map[string]any)["merge_of"] != base {
		t.Fatalf("by commit: %v", byCommit)
	}
	if byTicket := agent.expect(http.StatusOK, "GET", "/api/v1/verdicts?ticket=1", "")["verdicts"].([]any); len(byTicket) != 2 {
		t.Fatalf("by ticket: %v", byTicket)
	}
	latest := browser.expect(http.StatusOK, "GET", "/api/v1/tickets/1", "")["verdicts"].([]any)
	if len(latest) != 1 || latest[0].(map[string]any)["sha"] != merge {
		t.Fatalf("ticket summary verdicts: %v", latest)
	}

	reply := agent.expect(http.StatusOK, "POST", "/mcp", fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_verdicts","arguments":{"repo":"example/tools","sha":%q}}}`, merge))
	text := reply["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "passed (clean merge of "+base+")") {
		t.Fatalf("get_verdicts: %s", text)
	}
	reply = agent.expect(http.StatusOK, "POST", "/mcp", fmt.Sprintf(
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"report_clean_merge","arguments":{"ticket_id":1,"repo":"example/tools","sha":%q,"merge_of":%q}}}`,
		strings.Repeat("d", 40), strings.Repeat("e", 40)))
	if result := reply["result"].(map[string]any); result["isError"] != true {
		t.Fatalf("report_clean_merge of an unverified commit: %v", result)
	}
}
