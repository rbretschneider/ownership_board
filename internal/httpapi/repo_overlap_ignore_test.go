package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestRepoOverlapIgnoreAPI(t *testing.T) {
	b, token := signedIn(t)
	repo := b.expect(http.StatusCreated, "POST", "/api/v1/repos", `{"full_name":"example/project"}`)
	path := "/api/v1/repos/" + jsonNumber(repo["id"])
	if targets, ok := repo["overlap_ignore"].([]any); !ok || len(targets) != 0 {
		t.Fatal(repo)
	}
	body := `{"overlap_ignore":[" src/messages/Catalog.cpp "]}`
	b.asAgent(token).expect(http.StatusForbidden, "PATCH", path, body)
	saved := b.expect(http.StatusOK, "PATCH", path, body)
	if saved["overlap_ignore"].([]any)[0] != "src/messages/Catalog.cpp" {
		t.Fatal(saved)
	}
	for _, raw := range []string{`["src/` + string([]byte{255}) + `.go"]`, `null`, `{}`, `[null]`, `[4]`, `["../file.go"]`, `["src/[bad.go"]`, `["/src/file.go"]`, `["` + strings.Repeat("é", 201) + `"]`, `[` + strings.Repeat(`"src/a.go",`, 100) + `"src/a.go"]`} {
		b.expect(http.StatusBadRequest, "PATCH", path, `{"description":"bad","overlap_ignore":`+raw+`}`)
		kept := b.expect(http.StatusOK, "PATCH", path, `{}`)
		if kept["description"] == "bad" || len(kept["overlap_ignore"].([]any)) != 1 {
			t.Fatal(kept)
		}
	}
	omitted := b.expect(http.StatusOK, "PATCH", path, `{"description":"updated"}`)
	if len(omitted["overlap_ignore"].([]any)) != 1 {
		t.Fatal(omitted)
	}
	b.asAgent(token).expect(http.StatusOK, "GET", "/api/v1/repos", "")
	board := b.asAgent(token).expect(http.StatusOK, "GET", "/api/v1/board", "")
	if len(board["repos"].([]any)[0].(map[string]any)["overlap_ignore"].([]any)) != 1 {
		t.Fatal(board)
	}
	b.expect(http.StatusCreated, "POST", "/api/v1/tickets", `{"kind":"story","title":"Example","repo_id":`+jsonNumber(repo["id"])+`}`)
	work := b.asAgent(token).expect(http.StatusOK, "POST", "/api/v1/agent/claim", "")
	if len(work["repo"].(map[string]any)["overlap_ignore"].([]any)) != 1 {
		t.Fatal(work)
	}
	cleared := b.expect(http.StatusOK, "PATCH", path, `{"overlap_ignore":[]}`)
	if len(cleared["overlap_ignore"].([]any)) != 0 {
		t.Fatal(cleared)
	}
}
