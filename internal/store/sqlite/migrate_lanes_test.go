package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// migratorAt opens path and returns a migrator over the embedded migrations.
func migratorAt(t *testing.T, path string) (*sql.DB, *migrate.Migrate) {
	t.Helper()
	db, err := sql.Open(driverName, dsn(path))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	source, err := iofs.New(migrations, "migrations")
	if err != nil {
		t.Fatalf("source: %v", err)
	}
	driver, err := migratesqlite.WithInstance(db, &migratesqlite.Config{})
	if err != nil {
		t.Fatalf("driver: %v", err)
	}
	migrator, err := migrate.NewWithInstance("iofs", source, driverName, driver)
	if err != nil {
		t.Fatalf("migrator: %v", err)
	}
	return db, migrator
}

func insertOldTicket(t *testing.T, db *sql.DB, lane string, rank float64, spec string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO tickets (kind, title, spec, lane, rank, lease_holder, lease_expires_at, created_by, created_at, updated_at, lane_changed_at)
		VALUES ('story', ?, ?, ?, ?, 'loop-a', '2099-01-01T00:00:00Z', 'Owner', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		lane, spec, lane, rank)
	if err != nil {
		t.Fatalf("insert %s: %v", lane, err)
	}
}

func TestLanesMigrationMapsEveryOldLane(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.db")
	db, migrator := migratorAt(t, path)
	if err := migrator.Migrate(4); err != nil {
		t.Fatalf("migrate to 4: %v", err)
	}
	insertOldTicket(t, db, "new", 1, "")
	insertOldTicket(t, db, "answers", 0, "")
	insertOldTicket(t, db, "ready", 5, "spec")
	insertOldTicket(t, db, "in_progress", 1, "spec")
	insertOldTicket(t, db, "review", 1, "spec")
	insertOldTicket(t, db, "done", 1, "spec")
	if err := migrator.Up(); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	want := map[string][2]string{
		"new": {"todo", "refine"}, "answers": {"todo", "refine"}, "ready": {"todo", "spec_review"},
		"in_progress": {"in_progress", "build"}, "review": {"review", "pr"}, "done": {"merged", "done"},
	}
	rows, err := db.Query(`SELECT title, lane, stage, lease_holder, spec_revision FROM tickets ORDER BY lane, rank`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var todoOrder []string
	for rows.Next() {
		var title, lane, stage, holder string
		var revision int
		if err := rows.Scan(&title, &lane, &stage, &holder, &revision); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if got := [2]string{lane, stage}; got != want[title] {
			t.Errorf("%s became %v, want %v", title, got, want[title])
		}
		if holder != "" {
			t.Errorf("%s kept its lease", title)
		}
		if title != "new" && title != "answers" && revision != 1 {
			t.Errorf("%s spec revision %d, want 1", title, revision)
		}
		if lane == "todo" {
			todoOrder = append(todoOrder, title)
		}
	}
	if len(todoOrder) != 3 || todoOrder[0] != "ready" || todoOrder[1] != "new" || todoOrder[2] != "answers" {
		t.Fatalf("todo order %v, want ready, new, answers", todoOrder)
	}
}

func TestLanesMigrationRefusesUnknownLanes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.db")
	db, migrator := migratorAt(t, path)
	if err := migrator.Migrate(4); err != nil {
		t.Fatalf("migrate to 4: %v", err)
	}
	insertOldTicket(t, db, "backlog", 1, "")
	if err := migrator.Up(); err == nil {
		t.Fatal("the migration accepted a lane it does not know")
	}
	var lane string
	if err := db.QueryRowContext(context.Background(), `SELECT lane FROM tickets`).Scan(&lane); err != nil || lane != "backlog" {
		t.Fatalf("the failed migration changed the ticket: %q %v", lane, err)
	}
}
