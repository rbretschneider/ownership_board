package sqlite

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/app"
)

func TestThreeLanesMigrationRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.db")
	db, migrator := migratorAt(t, path)
	if err := migrator.Migrate(13); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"review", "pr"}, {"review", "pr"}, {"in_progress", "pr"}, {"in_progress", "design"}, {"in_progress", "build"}, {"todo", "ready"}, {"merged", "done"}} {
		_, err := db.Exec(`INSERT INTO tickets (kind,title,lane,stage,rank,spec,design,spec_revision,design_revision,approved_spec_revision,approved_design_revision,branch,pr_url,is_autonomous,lease_holder,lease_expires_at,created_by,created_at,updated_at,lane_changed_at) VALUES ('story','example',?,?,1,'spec','design',2,3,2,3,'feature','https://example.com/pr/1',1,'worker','2099-01-01T00:00:00.000Z','Owner','2026-01-01T00:00:00.000Z','2026-01-02T00:00:00.000Z','2026-01-03T00:00:00.000Z')`, pair[0], pair[1])
		if err != nil {
			t.Fatal(err)
		}
	}
	snapshot := func() [][]any {
		t.Helper()
		rows, err := db.Query(`SELECT * FROM tickets ORDER BY id`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		cols, _ := rows.Columns()
		var result [][]any
		for rows.Next() {
			values := make([]any, len(cols))
			dest := make([]any, len(cols))
			for i := range values {
				dest[i] = &values[i]
			}
			if err := rows.Scan(dest...); err != nil {
				t.Fatal(err)
			}
			for i, col := range cols {
				if col == "lane" {
					values[i] = nil
				}
			}
			result = append(result, values)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return result
	}
	before := snapshot()
	check := func(prLane string) {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM tickets WHERE stage='pr' AND lane<>?`, prLane).Scan(&n); err != nil || n != 0 {
			t.Fatalf("PR lanes: %d %v", n, err)
		}
		var controls int
		if err := db.QueryRow(`SELECT count(*) FROM tickets WHERE (stage IN ('design','build') AND lane='in_progress') OR (stage='ready' AND lane='todo') OR (stage='done' AND lane='merged')`).Scan(&controls); err != nil || controls != 4 {
			t.Fatalf("control lanes: %d %v", controls, err)
		}
		if !reflect.DeepEqual(before, snapshot()) {
			t.Fatal("migration changed other ticket fields")
		}
	}
	if err := migrator.Up(); err != nil {
		t.Fatal(err)
	}
	check("in_progress")
	var retired int
	if err := db.QueryRow(`SELECT count(*) FROM tickets WHERE lane='review'`).Scan(&retired); err != nil || retired != 0 {
		t.Fatalf("retired lanes: %d %v", retired, err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatal(err)
	}
	check("review")
	if err := migrator.Up(); err != nil {
		t.Fatal(err)
	}
	check("in_progress")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Read(context.Background(), func(repos app.Repos) error {
		_, err := repos.Tickets().List(context.Background(), app.TicketFilter{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
