package httpapi_test

import (
	"net/http"
	"testing"
)

func TestRepoBuildTargetAPI(t *testing.T) {
	b, token := signedIn(t)
	repo := b.expect(http.StatusCreated, "POST", "/api/v1/repos", `{"full_name":"example/project"}`)
	path := "/api/v1/repos/" + jsonNumber(repo["id"])
	if targets, ok := repo["build_targets"].([]any); !ok || len(targets) != 0 {
		t.Fatal(repo)
	}
	for _, raw := range []string{`null`, `{}`, `[null]`, `[{}]`, `[{"os":"linux","preset":"cmake"}]`, `[{"os":"linux","preset":"cmake","capabilities":null}]`, `[{"os":"linux","preset":"cmake","capabilities":[null]}]`, `[{"os":"linux","preset":"cmake","capabilities":[],"extra":true}]`, `[{"os":"other","preset":"cmake","capabilities":[]}]`, `[{"os":"linux","preset":"bad","capabilities":[]}]`, `[{"os":"linux","preset":"cmake","capabilities":["Bad"]}]`} {
		b.expect(http.StatusBadRequest, "PATCH", path, `{"build_targets":`+raw+`}`)
	}
	body := `{"build_targets":[{"os":"linux","preset":"cmake","capabilities":["msvc","gl","gl"]}]}`
	b.asAgent(token).expect(http.StatusForbidden, "PATCH", path, body)
	saved := b.expect(http.StatusOK, "PATCH", path, body)
	if caps := saved["build_targets"].([]any)[0].(map[string]any)["capabilities"].([]any); len(caps) != 2 || caps[0] != "gl" {
		t.Fatal(saved)
	}
	omitted := b.expect(http.StatusOK, "PATCH", path, `{"description":"updated"}`)
	if len(omitted["build_targets"].([]any)) != 1 {
		t.Fatal(omitted)
	}
	b.asAgent(token).expect(http.StatusOK, "GET", "/api/v1/repos", "")
	cleared := b.expect(http.StatusOK, "PATCH", path, `{"build_targets":[]}`)
	if len(cleared["build_targets"].([]any)) != 0 {
		t.Fatal(cleared)
	}
}
