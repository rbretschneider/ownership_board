package sqlite

import (
	"context"
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
