package httpapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCIRunners(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ci_runners.json")
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCIRunnersAreSanitized(t *testing.T) {
	path := writeCIRunners(t, `{"at":1793910000,"token":"secret-x","runners":[
	 {"repo":"acme/app","name":"linux-1","status":"online","busy":true,"labels":["self-hosted","linux","X64","<script>"],"os":"secret-os"},
	 {"repo":"acme/app","name":"linux-1","status":"online","busy":false,"labels":[]},
	 {"repo":"acme/app","name":"win-1","status":"offline","busy":true,"labels":["windows"]},
	 {"repo":"acme/app","name":"bad name","status":"online","labels":[]},
	 {"repo":"https://evil.example","name":"x","status":"online","labels":[]},
	 {"repo":"acme/app","name":"weird","status":"exploded","labels":[]}]}`)
	got := readCIRunners(path)
	if got.ObservedAt == nil || len(got.Runners) != 2 {
		t.Fatalf("%+v", got)
	}
	if first := got.Runners[0]; first.Name != "linux-1" || !first.Busy || strings.Join(first.Labels, ",") != "self-hosted,linux,X64" {
		t.Fatalf("first %+v", first)
	}
	// An offline runner cannot be busy, whatever the file says.
	if second := got.Runners[1]; second.Name != "win-1" || second.Status != "offline" || second.Busy {
		t.Fatalf("second %+v", second)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "script") || strings.Contains(string(raw), "evil") {
		t.Fatal(string(raw))
	}
}

func TestCIRunnersMissingOrMalformedIsEmpty(t *testing.T) {
	for _, path := range []string{"", filepath.Join(t.TempDir(), "absent.json"),
		writeCIRunners(t, `{"at":0,"runners":[]}`), writeCIRunners(t, `not json`), writeCIRunners(t, `{"at":1793910000,"runners":[]} {"x":1}`)} {
		got := readCIRunners(path)
		if got.ObservedAt != nil || got.Runners == nil || len(got.Runners) != 0 {
			t.Fatalf("%q: %+v", path, got)
		}
	}
}
