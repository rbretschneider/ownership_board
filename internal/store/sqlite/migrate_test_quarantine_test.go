package sqlite

import (
	"path/filepath"
	"testing"
)

// Existing jobs are commit jobs; the quarantine migration reverses cleanly.
func TestTestQuarantineMigration(t *testing.T) {
	db, m := migratorAt(t, filepath.Join(t.TempDir(), "board.db"))
	if err := m.Migrate(28); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO repos(id,provider,full_name,created_at) VALUES(1,'manual','example/app','2026-01-01T00:00:00.000Z')`); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		`INSERT INTO test_quarantine(repo_id,test,status,source,created_at,updated_at) VALUES(1,'t','paused','seeded','x','x')`,
		`INSERT INTO test_quarantine(repo_id,test,status,source,created_at,updated_at) VALUES(1,'','active','seeded','x','x')`,
	} {
		if _, err := db.Exec(bad); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
	if _, err := db.Exec(`INSERT INTO test_quarantine(repo_id,test,status,source,created_at,updated_at) VALUES(1,'t','active','seeded','x','x')`); err != nil {
		t.Fatal(err)
	}
	if err := m.Migrate(27); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`SELECT 1 FROM test_quarantine`); err == nil {
		t.Fatal("table survived the down migration")
	}
}
