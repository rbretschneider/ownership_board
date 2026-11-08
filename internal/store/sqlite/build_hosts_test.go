package sqlite

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func hostFixture(t *testing.T) (*DB, domain.BuildHost) {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "hosts.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	h, err := domain.NewBuildHost(domain.BuildHostDraft{Name: "Builder", OS: "linux", Arch: "arm64", PublicKey: base64.StdEncoding.EncodeToString(make([]byte, 32))}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return db, h
}

func TestBuildHostPersistence(t *testing.T) {
	db, h := hostFixture(t)
	ctx := context.Background()
	if _, err := db.db.Exec(`INSERT INTO users (id,name,email,password_hash,created_at) VALUES (1,'Owner','owner@example.com','hash','2026-01-01T00:00:00.000Z')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Write(ctx, func(r app.Repos) error { return r.BuildHosts().Insert(ctx, &h) }); err != nil {
		t.Fatal(err)
	}
	if err := db.Read(ctx, func(r app.Repos) error {
		got, err := r.BuildHosts().Get(ctx, h.ID)
		if err == nil && !reflect.DeepEqual(got, h) {
			t.Fatalf("round trip: %+v vs %+v", got, h)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err := db.Write(ctx, func(r app.Repos) error { copy := h; return r.BuildHosts().Insert(ctx, &copy) })
	if domain.KindOf(err) != domain.KindConflict {
		t.Fatal(err)
	}
	err = db.Write(ctx, func(r app.Repos) error {
		if err := h.Approve(1, h.CreatedAt, domain.BuildHostEdit{}); err != nil {
			return err
		}
		if err := r.BuildHosts().Approve(ctx, h); err != nil {
			return err
		}
		return errors.New("rollback")
	})
	if err == nil {
		t.Fatal("expected rollback")
	}
	err = db.Write(ctx, func(r app.Repos) error {
		got, err := r.BuildHosts().Get(ctx, h.ID)
		if err != nil {
			return err
		}
		if got.Status != domain.BuildHostPending {
			t.Fatal("rollback lost")
		}
		if err := got.Approve(1, h.CreatedAt, domain.BuildHostEdit{}); err != nil {
			return err
		}
		if err := r.BuildHosts().Approve(ctx, got); err != nil {
			return err
		}
		if domain.KindOf(r.BuildHosts().Approve(ctx, got)) != domain.KindConflict {
			t.Fatal("approved twice")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = db.Read(ctx, func(r app.Repos) error {
		_, err := r.BuildHosts().Get(ctx, 999)
		if domain.KindOf(err) != domain.KindNotFound {
			t.Fatal(err)
		}
		list, err := r.BuildHosts().List(ctx)
		if len(list) != 1 {
			t.Fatal(list)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBuildHostMigrationRoundTrip(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source.db")
	db, m := migratorAt(t, sourcePath)
	if err := m.Migrate(12); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (id,name,email,password_hash,created_at) VALUES (1,'Owner','owner@example.com','hash','2026-01-01T00:00:00.000Z')`); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`INSERT INTO repos (id,provider,full_name,created_at) VALUES (1,'manual','example/tools','2026-01-01T00:00:00.000Z')`,
		`INSERT INTO tickets (id,repo_id,kind,title,stage,lane,rank,created_by,created_at,updated_at,lane_changed_at) VALUES (1,1,'story','Example task','ready','todo',1,'Owner','2026-01-01T00:00:00.000Z','2026-01-01T00:00:00.000Z','2026-01-01T00:00:00.000Z')`,
		`INSERT INTO timeline (ticket_id,type,actor_kind,actor_name,data,created_at) VALUES (1,'comment','human','Owner','{"text":"Example history"}','2026-01-01T00:00:00.000Z')`,
		`INSERT INTO board_settings (key,value,updated_by,updated_at) VALUES ('example','value','Owner','2026-01-01T00:00:00.000Z')`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	copyPath := filepath.Join(t.TempDir(), "copy.db")
	if err := os.WriteFile(copyPath, contents, 0600); err != nil {
		t.Fatal(err)
	}
	db, m = migratorAt(t, copyPath)
	before := map[string][]string{}
	for _, table := range []string{"users", "repos", "tickets", "board_settings", "timeline"} {
		before[table] = dumpTable(t, db, table)
	}
	if err := m.Migrate(13); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`INSERT INTO build_hosts (name,os,arch,public_key,created_at) VALUES ('Host','linux','amd64','key','2026-01-01T00:00:00.000Z')`,
		`INSERT INTO build_host_pairing_codes (code_hash,created_by,created_at,expires_at) VALUES ('aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',1,'2026-01-01T00:00:00.000Z','2026-01-01T00:15:00.000Z')`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	for _, set := range []string{"os='bsd'", "max_parallel=0", "max_parallel=9", "enabled=2", "capabilities='{}'", "capabilities='broken'", "facts='[]'", "status='approved'", "status='revoked'", "approved_by=999,approved_at='2026-01-01T00:00:00.000Z'", "max_parallel=1.5", "facts='broken'"} {
		if _, err := db.Exec(`UPDATE build_hosts SET ` + set); err == nil {
			t.Fatal("constraint accepted", set)
		}
	}
	if _, err := db.Exec(`UPDATE build_host_pairing_codes SET used_at='2026-01-01T00:00:00.000Z'`); err == nil {
		t.Fatal("partial use accepted")
	}
	for _, set := range []string{"code_hash='short'", "code_hash='AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA'", "created_by=999", "used_by_host=999,used_at='2026-01-01T00:00:00.000Z'"} {
		if _, err := db.Exec(`UPDATE build_host_pairing_codes SET ` + set); err == nil {
			t.Fatal("invalid code accepted", set)
		}
	}
	if _, err := db.Exec(`DELETE FROM users WHERE id=1`); err == nil {
		t.Fatal("pairing provenance deleted")
	}
	if err := m.Migrate(12); err != nil {
		t.Fatal(err)
	}
	for table, want := range before {
		if !reflect.DeepEqual(dumpTable(t, db, table), want) {
			t.Fatal("changed", table)
		}
	}
	for _, table := range []string{"build_hosts", "build_host_pairing_codes"} {
		if _, err := db.Query(`SELECT * FROM ` + table); err == nil {
			t.Fatal("table survived", table)
		}
	}
	if err := m.Migrate(13); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign key violation")
	}
}

func TestBuildHostManagementPreservesReportsAndPairingRecords(t *testing.T) {
	db, h := hostFixture(t)
	ctx := context.Background()
	if _, err := db.db.Exec(`INSERT INTO users (id,name,email,created_at) VALUES (1,'Owner','owner@example.com','2026-01-01T00:00:00.000Z')`); err != nil {
		t.Fatal(err)
	}
	seen := h.CreatedAt.Add(time.Hour)
	h.LastSeenAt = &seen
	h.Facts = []byte(`{"cores":4}`)
	h.AgentVersion = "1.2.3"
	h.Capabilities = []string{"gpu"}
	err := db.Write(ctx, func(r app.Repos) error {
		if err := r.BuildHosts().Insert(ctx, &h); err != nil {
			return err
		}
		stale := h
		name := "Renamed"
		if err := stale.ApplyEdit(domain.BuildHostEdit{Name: &name}); err != nil {
			return err
		}
		// Simulate a stale management copy: report columns must never be written by management SQL.
		stale.LastSeenAt = nil
		stale.Facts = []byte(`{}`)
		stale.AgentVersion = ""
		if err := r.BuildHosts().Update(ctx, stale); err != nil {
			return err
		}
		if err := stale.Approve(1, seen, domain.BuildHostEdit{}); err != nil {
			return err
		}
		if err := r.BuildHosts().Approve(ctx, stale); err != nil {
			return err
		}
		if err := r.BuildHosts().Revoke(ctx, h.ID, seen); err != nil {
			return err
		}
		if err := r.BuildHosts().Revoke(ctx, h.ID, seen.Add(time.Hour)); err != nil {
			return err
		}
		got, err := r.BuildHosts().Get(ctx, h.ID)
		if err != nil {
			return err
		}
		if got.LastSeenAt == nil || !got.LastSeenAt.Equal(seen) || string(got.Facts) != string(h.Facts) || got.AgentVersion != h.AgentVersion || got.Name != name || got.Enabled || !got.RevokedAt.Equal(seen) || got.ApprovedBy == nil {
			t.Fatal("lost host fields", got)
		}
		c := domain.BuildHostPairingCode{CodeHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CreatedBy: 1, CreatedAt: h.CreatedAt, ExpiresAt: seen, UsedAt: &seen, UsedByHost: &h.ID}
		if err := r.BuildHostPairingCodes().Insert(ctx, c); err != nil {
			return err
		}
		if domain.KindOf(r.BuildHostPairingCodes().Insert(ctx, c)) != domain.KindConflict {
			t.Fatal("duplicate digest accepted")
		}
		duplicate := h
		if domain.KindOf(r.BuildHosts().Insert(ctx, &duplicate)) != domain.KindConflict {
			t.Fatal("revoked identity reused")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var used string
	var by int64
	if err := db.db.QueryRow(`SELECT used_at,used_by_host FROM build_host_pairing_codes`).Scan(&used, &by); err != nil || used != formatTime(seen) || by != h.ID {
		t.Fatal(used, by, err)
	}
}

func TestBuildHostRepositoryMissingTransitions(t *testing.T) {
	db, h := hostFixture(t)
	ctx := context.Background()
	h.ID = 999
	if err := h.Approve(1, h.CreatedAt, domain.BuildHostEdit{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Write(ctx, func(r app.Repos) error {
		for _, err := range []error{r.BuildHosts().Approve(ctx, h), r.BuildHosts().Update(ctx, h), r.BuildHosts().Revoke(ctx, h.ID, h.CreatedAt)} {
			if domain.KindOf(err) != domain.KindNotFound {
				t.Fatalf("missing transition: %v", err)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestBuildHostCorruptRowsAreStorageErrors(t *testing.T) {
	for _, set := range []string{"capabilities='null'", "capabilities='[\"GPU\"]'", "facts='[]'", "os='other'", "last_seen_at=''", "created_at='invalid'", "public_key='invalid'"} {
		t.Run(set, func(t *testing.T) {
			db, h := hostFixture(t)
			ctx := context.Background()
			if err := db.Write(ctx, func(r app.Repos) error { return r.BuildHosts().Insert(ctx, &h) }); err != nil {
				t.Fatal(err)
			}
			if _, err := db.db.Exec(`PRAGMA ignore_check_constraints=ON`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.db.Exec(`UPDATE build_hosts SET ` + set); err != nil {
				t.Fatal(err)
			}
			if err := db.Read(ctx, func(r app.Repos) error {
				_, err := r.BuildHosts().Get(ctx, h.ID)
				if err == nil || domain.KindOf(err) != 0 {
					t.Fatalf("corruption was accepted or treated as user input: %v", err)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBuildHostListingAndCanonicalIdentity(t *testing.T) {
	db, h := hostFixture(t)
	ctx := context.Background()
	if err := db.Write(ctx, func(r app.Repos) error {
		if err := r.BuildHosts().Insert(ctx, &h); err != nil {
			return err
		}
		duplicate := h
		duplicate.PublicKey = duplicate.PublicKey[:20] + "\n" + duplicate.PublicKey[20:]
		if domain.KindOf(r.BuildHosts().Insert(ctx, &duplicate)) != domain.KindConflict {
			t.Fatal("alternate encoding reused identity")
		}
		second := h
		key := make([]byte, 32)
		key[0] = 1
		second.PublicKey = base64.StdEncoding.EncodeToString(key)
		second.Name = "Second"
		if err := r.BuildHosts().Insert(ctx, &second); err != nil {
			return err
		}
		list, err := r.BuildHosts().List(ctx)
		if err != nil {
			return err
		}
		if len(list) != 2 || list[0].ID != h.ID || list[1].ID != second.ID || list[0].ID >= list[1].ID {
			t.Fatal("unordered listing", list)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
