package sqlite

import (
	"path/filepath"
	"testing"
)

func TestRepoReviewBlockingMigration(t *testing.T) {
	db, m := migratorAt(t, filepath.Join(t.TempDir(), "board.db"))
	if err := m.Migrate(21); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO repos(provider,full_name,created_at) VALUES('manual','example/app','2026-01-01T00:00:00.000Z')`); err != nil {
		t.Fatal(err)
	}
	if err := m.Migrate(22); err != nil {
		t.Fatal(err)
	}
	var level string
	if err := db.QueryRow(`SELECT review_blocking FROM repos`).Scan(&level); err != nil || level != "p1" {
		t.Fatalf("existing repo blocks at %q (%v), want p1", level, err)
	}
	if _, err := db.Exec(`UPDATE repos SET review_blocking = 'p9'`); err == nil {
		t.Fatal("accepted an unknown blocking level")
	}
	if _, err := db.Exec(`UPDATE repos SET review_blocking = 'p3'`); err != nil {
		t.Fatal(err)
	}
	if err := m.Migrate(21); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT review_blocking FROM repos`).Scan(&level); err == nil {
		t.Fatal("column survived the down migration")
	}
}
