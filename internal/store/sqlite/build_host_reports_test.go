package sqlite

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestHostReportAndConsumption(t *testing.T) {
	db, h := hostFixture(t)
	ctx := context.Background()
	_, e := db.db.Exec(`INSERT INTO users(id,name,email,created_at) VALUES(1,'Owner','o@example.com','2026-01-01T00:00:00.000Z')`)
	if e != nil {
		t.Fatal(e)
	}
	now := h.CreatedAt
	e = db.Write(ctx, func(r app.Repos) error {
		if e := r.BuildHosts().Insert(ctx, &h); e != nil {
			return e
		}
		return r.BuildHostPairingCodes().Insert(ctx, domain.BuildHostPairingCode{CodeHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CreatedBy: 1, CreatedAt: now, ExpiresAt: now.Add(time.Minute)})
	})
	if e != nil {
		t.Fatal(e)
	}
	e = db.Write(ctx, func(r app.Repos) error {
		return r.BuildHostPairingCodes().Consume(ctx, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", h.ID, now)
	})
	if e != nil {
		t.Fatal(e)
	}
	e = db.Write(ctx, func(r app.Repos) error {
		return r.BuildHostPairingCodes().Consume(ctx, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", h.ID, now)
	})
	if e == nil {
		t.Fatal("reused")
	}
	e = db.Write(ctx, func(r app.Repos) error {
		return r.BuildHosts().RecordHeartbeat(ctx, h.ID, domain.BuildHostReport{Facts: []byte(`{}`), RunningJobIDs: []domain.BuildHostJobID{}, FreeDiskBytes: 3}, now)
	})
	if e == nil {
		t.Fatal("pending heartbeat")
	}
}

func TestBuildHostTelemetryRoundTrip(t *testing.T) {
	db, h := hostFixture(t)
	ctx := context.Background()
	h.RunningJobIDs = []domain.BuildHostJobID{"3", "7"}
	free := int64(123)
	h.FreeDiskBytes = &free
	if e := db.Write(ctx, func(r app.Repos) error { return r.BuildHosts().Insert(ctx, &h) }); e != nil {
		t.Fatal(e)
	}
	if e := db.Read(ctx, func(r app.Repos) error {
		got, e := r.BuildHosts().Get(ctx, h.ID)
		if e == nil && (len(got.RunningJobIDs) != 2 || got.FreeDiskBytes == nil || *got.FreeDiskBytes != 123) {
			t.Fatal(got)
		}
		return e
	}); e != nil {
		t.Fatal(e)
	}
}

func TestBuildHostReportsMigrationRoundTrip(t *testing.T) {
	db, m := migratorAt(t, filepath.Join(t.TempDir(), "migration.db"))
	if e := m.Migrate(14); e != nil {
		t.Fatal(e)
	}
	for _, q := range []string{`INSERT INTO users(id,name,email,created_at) VALUES(1,'Owner','o@example.com','2026-01-01T00:00:00.000Z')`, `INSERT INTO build_hosts(name,os,arch,public_key,created_at) VALUES('Builder','linux','amd64','key','2026-01-01T00:00:00.000Z')`, `INSERT INTO build_host_pairing_codes(code_hash,created_by,created_at,expires_at) VALUES('aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',1,'2026-01-01T00:00:00.000Z','2026-01-01T00:15:00.000Z')`} {
		if _, e := db.Exec(q); e != nil {
			t.Fatal(e)
		}
	}
	tables := []string{"build_hosts", "build_host_pairing_codes", "users", "repos", "tickets", "board_settings"}
	before := map[string][]string{}
	for _, table := range tables {
		before[table] = dumpTable(t, db, table)
	}
	if e := m.Migrate(15); e != nil {
		t.Fatal(e)
	}
	for _, set := range []string{`running_job_ids='{}'`, `running_job_ids='null'`, `running_job_ids='invalid'`, `free_disk_bytes=-1`, `free_disk_bytes=1.5`} {
		if _, e := db.Exec(`UPDATE build_hosts SET ` + set); e == nil {
			t.Fatal("accepted", set)
		}
	}
	if _, e := db.Exec(`UPDATE build_hosts SET running_job_ids='[1,2]',free_disk_bytes=10`); e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec(`INSERT INTO build_host_events(host_id,event_type,data,created_at) VALUES(1,'toolchains_changed','{}','2026-01-01T00:00:00.000Z')`); e != nil {
		t.Fatal(e)
	}
	if e := m.Migrate(14); e != nil {
		t.Fatal(e)
	}
	for _, table := range tables {
		if !reflect.DeepEqual(before[table], dumpTable(t, db, table)) {
			t.Fatal("migration changed", table)
		}
	}
	if e := m.Migrate(15); e != nil {
		t.Fatal(e)
	}
	rows, e := db.Query(`PRAGMA foreign_key_check`)
	if e != nil {
		t.Fatal(e)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign key violation")
	}
	if e = rows.Err(); e != nil {
		t.Fatal(e)
	}
}
