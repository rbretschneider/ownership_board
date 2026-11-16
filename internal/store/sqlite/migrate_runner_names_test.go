package sqlite

import (
	"path/filepath"
	"slices"
	"testing"
)

// The runner names migration seeds each family's pool in display order, the
// first six (one per worker slot under the cap) first, and leaves out the
// board owner's name.
func TestRunnerNamesMigrationSeedsPoolsInOrder(t *testing.T) {
	db, m := migratorAt(t, filepath.Join(t.TempDir(), "board.db"))
	if err := m.Migrate(26); err != nil {
		t.Fatal(err)
	}
	pool := func(family string) []string {
		t.Helper()
		rows, err := db.Query(`SELECT name FROM runner_names WHERE family=? AND worker IS NULL ORDER BY position`, family)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var names []string
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				t.Fatal(err)
			}
			names = append(names, name)
		}
		return names
	}
	office, raymond := pool("claude"), pool("codex")
	if got, want := office[:6], []string{"Michael", "Dwight", "Jim", "Pam", "Angela", "Andy"}; !slices.Equal(got, want) {
		t.Errorf("claude first six = %q, want %q", got, want)
	}
	if got, want := raymond[:6], []string{"Ray", "Debra", "Robert", "Marie", "Frank", "Amy"}; !slices.Equal(got, want) {
		t.Errorf("codex first six = %q, want %q", got, want)
	}
	if len(office) != 18 || len(raymond) != 14 {
		t.Errorf("pool sizes = %d, %d, want 18, 14", len(office), len(raymond))
	}
	if slices.Contains(office, "Ryan") {
		t.Error("the claude pool must not hold the owner's name")
	}
	if err := m.Migrate(25); err != nil {
		t.Fatal(err)
	}
}

// A worker holds at most one name, and a name at most one worker.
func TestRunnerNamesRejectsSecondAssignmentOfWorker(t *testing.T) {
	db, m := migratorAt(t, filepath.Join(t.TempDir(), "board.db"))
	if err := m.Migrate(26); err != nil {
		t.Fatal(err)
	}
	assign := `UPDATE runner_names SET worker='claude_1', assigned_at='2026-01-01T00:00:00.000Z' WHERE family='claude' AND position=?`
	if _, err := db.Exec(assign, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(assign, 2); err == nil {
		t.Fatal("a second name for the same worker was accepted")
	}
}
