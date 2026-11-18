package sqlite

import (
	"path/filepath"
	"testing"
)

// Batched verification is off for every existing repo, with batch size 4, and
// the migration reverses cleanly.
func TestBuildBatchesMigration(t *testing.T) {
	db, m := migratorAt(t, filepath.Join(t.TempDir(), "board.db"))
	if err := m.Migrate(26); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO repos(provider,full_name,created_at) VALUES('manual','example/app','2026-01-01T00:00:00.000Z')`); err != nil {
		t.Fatal(err)
	}
	if err := m.Migrate(27); err != nil {
		t.Fatal(err)
	}
	var on, size int
	if err := db.QueryRow(`SELECT batch_verify, batch_max FROM repos`).Scan(&on, &size); err != nil || on != 0 || size != 4 {
		t.Fatalf("existing repo batching = %d size %d (%v), want off and 4", on, size, err)
	}
	for _, bad := range []string{`UPDATE repos SET batch_max = 1`, `UPDATE repos SET batch_max = 9`, `UPDATE repos SET batch_verify = 2`} {
		if _, err := db.Exec(bad); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
	if err := m.Migrate(26); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT batch_verify FROM repos`).Scan(&on); err == nil {
		t.Fatal("column survived the down migration")
	}
	if _, err := db.Exec(`SELECT 1 FROM build_batches`); err == nil {
		t.Fatal("table survived the down migration")
	}
}
