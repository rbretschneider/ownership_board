package httpapi_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnersRegistryCompatibility(t *testing.T) {
	c, token := signedIn(t)
	c.asAgent("").expect(401, "GET", "/api/v1/runners", "")
	c.asAgent(token).expect(403, "GET", "/api/v1/runners", "")
	linux := seedBuildHost(t, c)
	for _, status := range []string{"pending", "revoked"} {
		if status == "revoked" {
			c.expect(200, "POST", fmt.Sprintf("/api/v1/build-hosts/%d/revoke", linux), `{}`)
		}
		got := c.expect(200, "GET", "/api/v1/runners", "")
		if _, ok := got["hosts"]; ok {
			t.Fatal("unapproved host suppressed fallback", got)
		}
	}
	// Each registration retains its own identity.
	if _, err := c.store.SQL().Exec(`UPDATE build_hosts SET public_key='AgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=' WHERE id=?`, linux); err != nil {
		t.Fatal(err)
	}
	linux = seedBuildHost(t, c)
	c.expect(200, "POST", fmt.Sprintf("/api/v1/build-hosts/%d/approve", linux), `{}`)
	got := c.expect(200, "GET", "/api/v1/runners", "")
	if got["host"] != "Builder" || got["observed_at"] != nil || len(got["hosts"].([]any)) != 1 {
		t.Fatal(got)
	}
	// Create another identity with a unique key using a neutral registration fixture.
	if _, err := c.store.SQL().Exec(`UPDATE build_hosts SET public_key='AQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=' WHERE id=?`, linux); err != nil {
		t.Fatal(err)
	}
	windows := seedWindowsSettingsHost(t, c, false)
	if _, err := c.store.SQL().Exec(`UPDATE build_hosts SET name='Windows builder',facts='{"hidden":"secret-fact"}',last_seen_at='2026-01-01T00:01:00.000Z' WHERE id=?`, windows); err != nil {
		t.Fatal(err)
	}
	c.expect(201, "POST", "/api/v1/tickets", `{"kind":"story","title":"Example"}`)
	for i, status := range []string{"running", "cancelled", "superseded", "passed", "failed"} {
		var released any
		if i == 4 {
			released = "2026-01-01T00:00:30.000Z"
		}
		_, err := c.store.SQL().Exec(`INSERT INTO build_jobs(id,ticket_id,repo,sha,preset,target,target_os,status,host_id,attempt,created_at,started_at,assignment_released_at,phase,detail,bundle_bytes,bundle_sha256,result) VALUES(?,1,'example/tools',?,'cmake','{"os":"windows","required_capabilities":[]}','windows',?,?,1,'2026-01-01T00:00:00.000Z','2026-01-01T00:00:01.000Z',?,'unrecognized','<private>',3,?,'{"private":"secret-result"}')`, fmt.Sprintf("%064x", i), strings.Repeat("a", 64), status, windows, released, strings.Repeat("b", 64))
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.store.SQL().Exec(`INSERT INTO build_jobs(id,ticket_id,repo,sha,preset,target,target_os,status,attempt,created_at,bundle_bytes,bundle_sha256) VALUES(?,1,'example/tools',?,'script','{"os":"windows","required_capabilities":[]}','windows','queued',1,'2025-12-31T00:00:00.000Z',3,?)`, fmt.Sprintf("%064x", 10), strings.Repeat("c", 40), strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	got = c.expect(200, "GET", "/api/v1/runners", "")
	if got["host"] != "Windows builder" || got["blocked"] != "disabled" || got["enabled"] != false || got["parallel_builds"] != float64(3) {
		t.Fatal(got)
	}
	hosts := got["hosts"].([]any)
	if len(hosts) != 2 || hosts[0].(map[string]any)["id"] != float64(linux) || hosts[1].(map[string]any)["os"] != "windows" {
		t.Fatal(hosts)
	}
	slots := got["slots"].([]any)
	if len(slots) != 4 {
		t.Fatal(slots)
	} // Capacity reductions do not hide occupied assignments.
	for i, slot := range slots {
		v := slot.(map[string]any)
		if v["slot"] != float64(i+1) || v["sha"] != "aaaaaaaaaaaa" || v["phase"] != "verifying" || v["detail"] != "" || v["updated_at"] != "2026-01-01T00:01:00Z" || v["since"] != "2026-01-01T00:00:01Z" || v["orphan"] != (i == 1 || i == 2) {
			t.Fatal(v)
		}
	}
	queue := got["queue"].([]any)
	if len(queue) != 1 {
		t.Fatal(queue)
	}
	queued := queue[0].(map[string]any)
	if queued["position"] != float64(1) || queued["ticket_id"] != float64(1) || queued["repo"] != "example/tools" || queued["sha"] != "cccccccccccc" || queued["preset"] != "script" || queued["queued_at"] != "2025-12-31T00:00:00Z" || queued["deferred_until"] != nil {
		t.Fatal(queued)
	}
	raw, _ := json.Marshal(got)
	for _, secret := range []string{"public_key", "facts", "secret-fact", "secret-result", "bundle", "private"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("exposed", secret)
		}
	}
}

func TestRunnersRegistryFileFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runners.json")
	if err := os.WriteFile(path, []byte(`{"at":1793822451,"host":"File-builder","enabled":true,"parallel_builds":2,"slots":[],"queue":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	c := newServerWithRunnersFile(t, "", path)
	c.expect(200, "POST", "/api/v1/auth/setup", `{"name":"Owner","email":"owner@example.com","password":"a long enough password"}`)
	assertFile := func() {
		t.Helper()
		got := c.expect(200, "GET", "/api/v1/runners", "")
		if got["host"] != "File-builder" || got["hosts"] != nil {
			t.Fatal(got)
		}
	}
	assertFile()
	id := seedBuildHost(t, c)
	assertFile()
	c.expect(200, "POST", fmt.Sprintf("/api/v1/build-hosts/%d/approve", id), `{}`)
	got := c.expect(200, "GET", "/api/v1/runners", "")
	if got["host"] != "Builder" || got["hosts"] == nil || got["observed_at"] != nil {
		t.Fatal(got)
	}
	c.expect(200, "POST", fmt.Sprintf("/api/v1/build-hosts/%d/revoke", id), `{}`)
	assertFile()
	if _, err := c.store.SQL().Exec(`DROP TABLE build_hosts`); err != nil {
		t.Fatal(err)
	}
	c.expect(500, "GET", "/api/v1/runners", "")
}
