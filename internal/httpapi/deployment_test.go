package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestDeploymentPolicyPATCH(t *testing.T) {
	browser, token := signedIn(t)
	repo := browser.expect(http.StatusCreated, "POST", "/api/v1/repos", `{"full_name":"acme/release"}`)
	if repo["deploy_mode"] != "off" {
		t.Fatalf("default: %v", repo)
	}
	path := "/api/v1/repos/" + jsonNumber(repo["id"])
	browser.asAgent(token).expect(http.StatusForbidden, "PATCH", path, `{"deploy_mode":"agent","description":"changed"}`)
	for _, value := range []string{`"workflow"`, `""`, `true`, `12`, `"AGENT"`} {
		browser.expect(http.StatusBadRequest, "PATCH", path, `{"deploy_mode":`+value+`,"description":"changed"}`)
	}
	got := browser.expect(http.StatusOK, "PATCH", path, `{"deploy_mode":null}`)
	if got["deploy_mode"] != "off" || got["description"] != "" {
		t.Fatalf("failed writes changed repo: %v", got)
	}
	got = browser.expect(http.StatusOK, "PATCH", path, `{"deploy_mode":"agent"}`)
	if got["deploy_mode"] != "agent" {
		t.Fatalf("setting: %v", got)
	}
	got = browser.expect(http.StatusOK, "PATCH", path, `{"color":"#112233"}`)
	if got["deploy_mode"] != "agent" {
		t.Fatal("omitted setting changed")
	}
}

func TestDeploymentReadContractsAndCompletionStats(t *testing.T) {
	browser, token := signedIn(t)
	repo := browser.expect(http.StatusCreated, "POST", "/api/v1/repos", `{"full_name":"acme/api"}`)
	rid := jsonNumber(repo["id"])
	browser.expect(http.StatusOK, "PATCH", "/api/v1/repos/"+rid, `{"deploy_mode":"agent"}`)
	ticket := browser.expect(http.StatusCreated, "POST", "/api/v1/tickets", `{"kind":"story","title":"release","repo_id":`+rid+`}`)
	path := "/api/v1/tickets/" + jsonNumber(ticket["id"])
	agent := browser.asAgent(token)
	work := agent.expect(http.StatusOK, "POST", "/api/v1/agent/claim", `{}`)
	if work["repo"].(map[string]any)["deploy_mode"] != "agent" {
		t.Fatalf("work repo: %v", work)
	}
	moved := browser.expect(http.StatusOK, "POST", path+"/move", `{"lane":"deployed"}`)
	if moved["stage"] != "deployed" || moved["lane"] != "deployed" || moved["owner"] != "nobody" || moved["lease"] != nil {
		t.Fatalf("deployed: %v", moved)
	}
	var found bool
	for _, value := range moved["timeline"].([]any) {
		entry := value.(map[string]any)
		if entry["type"] == "stage" && entry["data"].(map[string]any)["to"] == "deployed" {
			found = true
			if entry["body"] != "Marked deployed by Owner" {
				t.Fatalf("history: %v", entry)
			}
		}
	}
	if !found {
		t.Fatal("missing deployment history")
	}
	board := agent.expect(http.StatusOK, "GET", "/api/v1/board", "")
	if board["repos"].([]any)[0].(map[string]any)["deploy_mode"] != "agent" {
		t.Fatal("board repo policy missing")
	}
	reply := agent.expect(http.StatusOK, "POST", "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_repos","arguments":{}}}`)
	result := reply["result"].(map[string]any)
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "deploy_mode=agent") {
		t.Fatalf("listing: %v", reply)
	}
	reply = agent.expect(http.StatusOK, "POST", "/mcp", `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_tickets","arguments":{"lane":"deployed"}}}`)
	result = reply["result"].(map[string]any)
	if result["isError"] == true || !strings.Contains(result["content"].([]any)[0].(map[string]any)["text"].(string), "release") {
		t.Fatalf("deployed filter: %v", reply)
	}
	report := browser.expect(http.StatusOK, "GET", "/api/v1/stats?days=7", "")
	if report["all"].(map[string]any)["merged"] != float64(1) {
		t.Fatalf("completed stats: %v", report)
	}
	for _, value := range report["repos"].([]any) {
		r := value.(map[string]any)
		if r["owner"].(map[string]any)["open"] != float64(0) {
			t.Fatalf("open completed ticket: %v", r)
		}
	}
	// Updating deployment policy has no retroactive effect on ticket state.
	browser.expect(http.StatusOK, "PATCH", "/api/v1/repos/"+rid, `{"deploy_mode":"off"}`)
	if detail := browser.expect(http.StatusOK, "GET", path, ""); detail["stage"] != "deployed" {
		t.Fatal("policy moved existing ticket")
	}
}
