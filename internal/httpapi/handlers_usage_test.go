package httpapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUsageCacheIsSanitizedAndMissingIsUnknown(t *testing.T) {
	if got := readUsageSnapshot(""); len(got.Accounts) != 0 {
		t.Fatal(got)
	}
	path := filepath.Join(t.TempDir(), "usage.json")
	data := `{"accounts":[{"provider":"codex","access_token":"secret-should-never-leak","observed_at":"2026-11-02T15:00:00Z","windows":[{"label":"week","used_percent":13,"resets_at":null},{"label":"bad","used_percent":101}]},{"provider":"attacker","windows":[]},{"provider":"claude","observed_at":null,"windows":[]}]}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	got := readUsageSnapshot(path)
	if len(got.Accounts) != 2 || len(got.Accounts[0].Windows) != 1 || *got.Accounts[0].Windows[0].UsedPercent != 13 {
		t.Fatal(got)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "access_token") {
		t.Fatal(string(raw))
	}
	if got.Accounts[1].ObservedAt != nil || len(got.Accounts[1].Windows) != 0 {
		t.Fatal("missing usage must remain unknown")
	}
}

func TestUsageCacheRejectsMalformedOrTrailingJSON(t *testing.T) {
	for _, data := range []string{`not-json`, `{"accounts":[]} {"extra":true}`, `{"accounts":[{"provider":"codex","observed_at":"invalid"}]}`} {
		path := filepath.Join(t.TempDir(), "usage.json")
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if got := readUsageSnapshot(path); len(got.Accounts) != 0 {
			t.Fatal(got)
		}
	}
}
