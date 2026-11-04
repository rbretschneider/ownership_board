package httpapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRunners(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "windows_runners.json")
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunnersSnapshotIsSanitized(t *testing.T) {
	path := writeRunners(t, `{"at":1793822451.5,"host":"Ryan-Desktop","enabled":true,"parallel_builds":3,"token":"secret-x",
		"slots":[
		 {"slot":1,"ticket":24,"repo":"acme/cad","sha":"8c19d1c048ff5f29fef5d6f60c32fab0d768a6c8","preset":"script","phase":"verifying","detail":"ASan · testing","since":1793822451,"updated":1793822460},
		 {"slot":1,"ticket":25,"repo":"acme/cad","sha":"8c19d1c0","preset":"script","phase":"verifying"},
		 {"slot":2,"ticket":27,"repo":"acme/cad","sha":"56f13b70","preset":"cmake","phase":"queued","detail":"<script>alert(1)</script>","orphan":true},
		 {"slot":3,"ticket":-4,"repo":"../etc","sha":"zz","preset":"evil","phase":"building"}],
		"queue":[{"ticket":76,"repo":"acme/mix","sha":"00d05479","preset":"dotnet","queued":1793822000,"retry_after":0},
		 {"ticket":null,"repo":"acme/cad","sha":"31674971","preset":"script","queued":1793822100,"retry_after":1793822400}]}`)
	got := readRunnersSnapshot(path)
	if got.ObservedAt == nil || got.Host != "Ryan-Desktop" || !got.Enabled || got.ParallelBuilds != 3 {
		t.Fatalf("header %+v", got)
	}
	if len(got.Slots) != 2 {
		t.Fatalf("slots %+v", got.Slots)
	}
	first := got.Slots[0]
	if *first.TicketID != 24 || first.SHA != "8c19d1c048ff" || first.Detail != "ASan · testing" || first.Since == nil || first.Orphan {
		t.Fatalf("first slot %+v", first)
	}
	if second := got.Slots[1]; second.Detail != "" || !second.Orphan || *second.TicketID != 27 {
		t.Fatalf("unsafe detail must be dropped and orphan kept: %+v", second)
	}
	if len(got.Queue) != 2 || got.Queue[0].Position != 1 || got.Queue[1].TicketID != nil || got.Queue[1].DeferredUntil == nil || got.Queue[0].DeferredUntil != nil {
		t.Fatalf("queue %+v", got.Queue)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "script>") {
		t.Fatal(string(raw))
	}
}

func TestRunnersSnapshotMissingOrMalformedIsEmpty(t *testing.T) {
	for _, path := range []string{"", filepath.Join(t.TempDir(), "absent.json"),
		writeRunners(t, `not-json`), writeRunners(t, `{"at":1793822451,"slots":[]} {"x":1}`), writeRunners(t, `{"slots":[]}`)} {
		got := readRunnersSnapshot(path)
		if got.ObservedAt != nil || len(got.Slots) != 0 || got.Slots == nil || got.Queue == nil {
			t.Fatalf("%s: %+v", path, got)
		}
	}
}
