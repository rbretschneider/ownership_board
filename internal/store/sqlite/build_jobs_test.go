package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestBuildJobsMigrationPreservesHosts(t *testing.T) {
	db, m := migratorAt(t, filepath.Join(t.TempDir(), "upgrade.db"))
	if e := m.Migrate(16); e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec(`INSERT INTO build_hosts(name,os,arch,public_key,created_at,running_job_ids,free_disk_bytes) VALUES('Builder','linux','amd64','key','2026-01-01T00:00:00.000Z','[1,42]',100)`); e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec(`INSERT INTO repos(provider,full_name,created_at,deploy_mode) VALUES('manual','acme/api','2026-01-01T00:00:00.000Z','agent')`); e != nil {
		t.Fatal(e)
	}
	before := dumpTable(t, db, "build_hosts")
	reposBefore := dumpTable(t, db, "repos")
	if e := m.Migrate(18); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(before, dumpTable(t, db, "build_hosts")) {
		t.Fatal("upgrade changed hosts")
	}
	for _, name := range []string{"build_jobs", "build_job_uploads", "build_jobs_active_assignments", "build_jobs_commit_history"} {
		var count int
		if e := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name=?`, name).Scan(&count); e != nil || count != 1 {
			t.Fatalf("missing queue schema %s: count=%d, err=%v", name, count, e)
		}
	}
	if e := m.Migrate(16); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(reposBefore, dumpTable(t, db, "repos")) {
		t.Fatal("queue round trip changed repository deployment settings")
	}
	if !reflect.DeepEqual(before, dumpTable(t, db, "build_hosts")) {
		t.Fatal("downgrade changed hosts")
	}
}

func TestBuildHostHashTelemetryRoundTrip(t *testing.T) {
	db, h := hostFixture(t)
	ctx := context.Background()
	h.RunningJobIDs = []domain.BuildHostJobID{"7", domain.BuildHostJobID(strings.Repeat("a", 64))}
	if e := db.Write(ctx, func(r app.Repos) error { return r.BuildHosts().Insert(ctx, &h) }); e != nil {
		t.Fatal(e)
	}
	if e := db.Read(ctx, func(r app.Repos) error {
		got, e := r.BuildHosts().Get(ctx, h.ID)
		if e == nil && !reflect.DeepEqual(got.RunningJobIDs, h.RunningJobIDs) {
			t.Fatal(got.RunningJobIDs)
		}
		return e
	}); e != nil {
		t.Fatal(e)
	}
	var raw string
	if e := db.db.QueryRow(`SELECT running_job_ids FROM build_hosts WHERE id=?`, h.ID).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	if raw != `[7,"`+strings.Repeat("a", 64)+`"]` {
		t.Fatal("scalar form changed", raw)
	}
}

func TestRunnerPresentationQueries(t *testing.T) {
	db, h := hostFixture(t)
	ctx := context.Background()
	if e := db.Write(ctx, func(r app.Repos) error { return r.BuildHosts().Insert(ctx, &h) }); e != nil {
		t.Fatal(e)
	}
	if _, e := db.db.Exec(`INSERT INTO tickets(id,kind,title,stage,lane,rank,created_by,created_at,updated_at,lane_changed_at) VALUES(1,'story','Example','ready','todo',1,'Owner','2026-01-01T00:00:00.000Z','2026-01-01T00:00:00.000Z','2026-01-01T00:00:00.000Z')`); e != nil {
		t.Fatal(e)
	}
	for i, status := range []string{"queued", "running", "passed", "failed", "cancelled", "superseded"} {
		for k, os := range []string{"windows", "linux", "macos"} {
			id := fmt.Sprintf("%064x", i*3+k+1)
			if _, e := db.db.Exec(`INSERT INTO build_jobs(id,ticket_id,repo,sha,preset,target,target_os,status,attempt,created_at,bundle_bytes,bundle_sha256) VALUES(?,1,'example/app','abcdef','cmake','{}',?,?,1,'2026-01-01T00:00:00.000Z',3,?)`, id, os, status, strings.Repeat("a", 64)); e != nil {
				t.Fatal(e)
			}
		}
	}
	// Intentionally reverse timestamp order relative to IDs and retain stopping
	// assignments while excluding released and unassigned jobs.
	for _, row := range []struct {
		id       int
		started  string
		released any
	}{{4, "2026-01-01T00:00:03.000Z", nil}, {13, "2026-01-01T00:00:01.000Z", nil}, {16, "2026-01-01T00:00:01.000Z", nil}, {7, "2026-01-01T00:00:00.000Z", "2026-01-01T00:00:04.000Z"}} {
		if _, e := db.db.Exec(`UPDATE build_jobs SET host_id=?,started_at=?,assignment_released_at=? WHERE id=?`, h.ID, row.started, row.released, fmt.Sprintf("%064x", row.id)); e != nil {
			t.Fatal(e)
		}
	}
	if e := db.Read(ctx, func(r app.Repos) error {
		counts, e := r.BuildJobs().QueuedCountsByOS(ctx)
		if e != nil {
			return e
		}
		for _, os := range []domain.BuildHostOS{"windows", "linux", "macos"} {
			if counts[os] != 1 {
				t.Fatal(counts)
			}
		}
		assignments, e := r.BuildJobs().RunnerAssignments(ctx)
		if e != nil {
			return e
		}
		ids := []string{}
		for _, a := range assignments {
			ids = append(ids, a.ID)
			if a.HostID != h.ID || a.TicketID != 1 || a.StartedAt == nil {
				t.Fatal(a)
			}
		}
		expected := []string{fmt.Sprintf("%064x", 13), fmt.Sprintf("%064x", 16), fmt.Sprintf("%064x", 4)}
		if !reflect.DeepEqual(ids, expected) {
			t.Fatal(ids)
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
}
