package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestRepoOverlapIgnoreMigration(t *testing.T) {
	db, m := migratorAt(t, filepath.Join(t.TempDir(), "board.db"))
	if err := m.Migrate(19); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO repos(provider,full_name,created_at) VALUES('manual','example/app','2026-01-01T00:00:00.000Z')`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := m.Migrate(20); err != nil {
			t.Fatal(err)
		}
		var raw, name string
		var count int
		if err := db.QueryRow(`SELECT overlap_ignore,full_name FROM repos`).Scan(&raw, &name); err != nil || raw != "[]" || name != "example/app" {
			t.Fatal(raw, name, err)
		}
		if err := db.QueryRow(`SELECT count(*) FROM repos`).Scan(&count); err != nil || count != 1 {
			t.Fatal(count, err)
		}
		// The adapter reads every current repo column, so it needs the latest schema.
		if err := m.Up(); err != nil {
			t.Fatal(err)
		}
		adapter := codeRepoRepo{q: db}
		ctx := context.Background()
		if err := adapter.SetOverlapIgnore(ctx, domain.Repo{ID: 1, OverlapIgnore: []string{"src/*/Catalog.cpp"}}); err != nil {
			t.Fatal(err)
		}
		repo, err := adapter.Get(ctx, 1)
		if err != nil || len(repo.OverlapIgnore) != 1 || repo.OverlapIgnore[0] != "src/*/Catalog.cpp" {
			t.Fatal(repo, err)
		}
		for _, bad := range []string{"broken", "null", "{}", "[" + strings.Repeat(`"src/a.go",`, 100) + `"src/a.go"]`} {
			if _, err := db.Exec(`UPDATE repos SET overlap_ignore=?`, bad); err == nil {
				t.Fatal("accepted", bad)
			}
		}
		for _, bad := range []string{`["../file.go"]`, `[null]`, `[5]`, `["src/[bad.go"]`} {
			if _, err := db.Exec(`UPDATE repos SET overlap_ignore=?`, bad); err != nil {
				t.Fatal(err)
			}
			if _, err := adapter.Get(ctx, 1); err == nil {
				t.Fatal("ignored corrupt configuration", bad)
			}
		}
		if err := m.Migrate(19); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(`SELECT overlap_ignore FROM repos`).Scan(&raw); err == nil {
			t.Fatal("column survived")
		}
		if err := db.QueryRow(`SELECT full_name FROM repos`).Scan(&name); err != nil || name != "example/app" {
			t.Fatal(name, err)
		}
	}
}
