package httpapi_test

import (
	"net/http"
	"testing"
)

func TestSplitOverTheAPIShowsTheEpicAndItsStories(t *testing.T) {
	browser, _ := signedIn(t)
	api := jsonNumber(browser.expect(http.StatusCreated, "POST", "/api/v1/repos", `{"full_name":"acme/api"}`)["id"])
	parent := jsonNumber(browser.expect(http.StatusCreated, "POST", "/api/v1/tickets", `{"kind":"story","title":"big","repo_id":`+api+`}`)["id"])

	browser.expect(http.StatusBadRequest, "POST", "/api/v1/tickets/"+parent+"/split",
		`{"reason":"too big","children":[{"title":"a","spec":"1. a"}]}`)
	result := browser.expect(http.StatusCreated, "POST", "/api/v1/tickets/"+parent+"/split",
		`{"reason":"too big","children":[{"title":"a","spec":"1. a"},{"title":"b","spec":"1. b","after":[]}]}`)

	epic := result["parent"].(map[string]any)
	if epic["stage"] != "epic" || epic["lane"] != "in_progress" {
		t.Fatalf("parent stage=%v lane=%v, want an epic in progress", epic["stage"], epic["lane"])
	}
	children := epic["split_into"].([]any)
	ids := result["child_ids"].([]any)
	if len(children) != 2 || len(ids) != 2 {
		t.Fatalf("split_into=%v child_ids=%v", children, ids)
	}
	first := children[0].(map[string]any)
	if first["title"] != "a" || first["stage"] != "spec_review" || first["lane"] != "todo" {
		t.Fatalf("first story = %v", first)
	}
	child := browser.expect(http.StatusOK, "GET", "/api/v1/tickets/"+jsonNumber(ids[1]), "")
	if jsonNumber(child["split_from"]) != parent || jsonNumber(child["waiting_on"]) != "[]" {
		t.Fatalf("second story split_from=%v waiting_on=%v", child["split_from"], child["waiting_on"])
	}
	if epic["split_from"] != nil {
		t.Fatalf("the epic claims a parent: %v", epic["split_from"])
	}
}
