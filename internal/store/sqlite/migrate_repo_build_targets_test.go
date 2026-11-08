package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoBuildTargetsMigration(t *testing.T) {
	db, m := migratorAt(t, filepath.Join(t.TempDir(), "board.db"))
	if err := m.Migrate(18); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO repos(provider,full_name,created_at) VALUES('manual','example/app','2026-01-01T00:00:00.000Z')`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := m.Migrate(19); err != nil {
			t.Fatal(err)
		}
		var raw, name string
		if err := db.QueryRow(`SELECT build_targets,full_name FROM repos`).Scan(&raw, &name); err != nil || raw != "[]" || name != "example/app" {
			t.Fatal(raw, name, err)
		}
		for _, bad := range []string{"broken", "null", "{}", "[" + strings.Repeat("{},", 32) + "{}]"} {
			if _, err := db.Exec(`UPDATE repos SET build_targets=?`, bad); err == nil {
				t.Fatal("accepted", bad)
			}
		}
		if _, err := db.Exec(`UPDATE repos SET build_targets='[{"os":"other","preset":"cmake","capabilities":[]}]'`); err != nil {
			t.Fatal(err)
		}
		// The current adapter reads columns added after this migration too.
		if err := m.Migrate(20); err != nil {
			t.Fatal(err)
		}
		if _, err := (codeRepoRepo{q: db}).Get(context.Background(), 1); err == nil {
			t.Fatal("corrupt target was ignored")
		}
		if err := m.Migrate(18); err != nil {
			t.Fatal(err)
		}
	}
}
