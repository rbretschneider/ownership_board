package httpapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePRStatus(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pr_status.json")
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPRStatusIsSanitized(t *testing.T) {
	path := writePRStatus(t, `{"at":1793910000,"token":"secret-x","prs":[
	 {"ticket":92,"url":"https://github.com/acme/mix/pull/25","verdict":"running","reason":"","may_merge":true,"acts":true,"gl_waived":false,
	  "draft":false,"mergeable":"MERGEABLE","review":"","head":"9c7c3cf829b4","unresolved":true,"manual_check":true,"body":"secret-body",
	  "checks":[{"name":"build","status":"IN_PROGRESS","conclusion":"","started":"2026-11-05T20:51:10Z","completed":""},
	            {"name":"<script>","status":"COMPLETED","conclusion":"SUCCESS","started":"","completed":""},
	            {"name":"lint","status":"COMPLETED","conclusion":"SUCCESS","started":"nope","completed":"2026-11-05T20:52:00Z"}]},
	 {"ticket":92,"url":"https://github.com/acme/mix/pull/25","verdict":"merged","reason":""},
	 {"ticket":31,"url":"https://github.com/acme/cad/pull/32","verdict":"repair","reason":"checks_failed","mergeable":"WEIRD","head":"zz"},
	 {"ticket":7,"url":"https://evil.example/pull/1","verdict":"running","reason":""},
	 {"ticket":8,"url":"https://github.com/acme/cad/pull/9","verdict":"explode","reason":""},
	 {"ticket":9,"url":"https://github.com/acme/cad/pull/9","verdict":"repair","reason":""},
	 {"ticket":10,"url":"https://github.com/acme/cad/pull/10","verdict":"running","reason":"conflict"},
	 {"ticket":-1,"url":"https://github.com/acme/cad/pull/11","verdict":"running","reason":""}]}`)
	got := readPRStatus(path)
	if got.ObservedAt == nil || len(got.PRs) != 2 {
		t.Fatalf("%+v", got)
	}
	first := got.PRs[0]
	if first.TicketID != 92 || first.Verdict != "running" || !first.Unresolved || !first.ManualCheck || first.Head != "9c7c3cf829b4" || len(first.Checks) != 2 {
		t.Fatalf("first %+v", first)
	}
	if first.Checks[0].StartedAt == nil || first.Checks[1].Name != "lint" || first.Checks[1].StartedAt != nil || first.Checks[1].CompletedAt == nil {
		t.Fatalf("checks %+v", first.Checks)
	}
	if second := got.PRs[1]; second.Reason != "checks_failed" || second.Mergeable != "" || second.Head != "" {
		t.Fatalf("second %+v", second)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "script>") || strings.Contains(string(raw), "evil") {
		t.Fatal(string(raw))
	}
}

func TestPRStatusMissingOrMalformedIsEmpty(t *testing.T) {
	for _, path := range []string{"", filepath.Join(t.TempDir(), "absent.json"),
		writePRStatus(t, `{"at":0,"prs":[]}`), writePRStatus(t, `not json`), writePRStatus(t, `{"at":1793910000,"prs":[]} {"x":1}`)} {
		got := readPRStatus(path)
		if got.ObservedAt != nil || got.PRs == nil || len(got.PRs) != 0 {
			t.Fatalf("%q: %+v", path, got)
		}
	}
}
