package sqlite

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// dumpTable renders every row of table, in rowid order, as text.
func dumpTable(t *testing.T, db *sql.DB, table string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT * FROM ` + table + ` ORDER BY rowid`)
	if err != nil {
		t.Fatalf("dump %s: %v", table, err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatalf("columns of %s: %v", table, err)
	}
	var dump []string
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			t.Fatalf("scan %s: %v", table, err)
		}
		parts := make([]string, len(values))
		for i, value := range values {
			parts[i] = fmt.Sprintf("%s=%T:%v", columns[i], value, value)
		}
		dump = append(dump, strings.Join(parts, " "))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows of %s: %v", table, err)
	}
	return dump
}

func tableExists(t *testing.T, db *sql.DB, kind, name string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = ? AND name = ?`, kind, name).Scan(&count); err != nil {
		t.Fatalf("sqlite_master: %v", err)
	}
	return count == 1
}

func storedCap(t *testing.T, db *sql.DB) string {
	t.Helper()
	var value string
	if err := db.QueryRow(`SELECT value FROM board_settings WHERE key = 'autonomous_in_flight_cap'`).Scan(&value); err != nil {
		t.Fatalf("read cap: %v", err)
	}
	return value
}

func TestDependsOnMigrationLeavesExistingRowsAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.db")
	db, migrator := migratorAt(t, path)
	if err := migrator.Migrate(7); err != nil {
		t.Fatalf("migrate to 7: %v", err)
	}
	for i, stage := range []string{"refine", "ready", "build", "done"} {
		lane := map[string]string{"refine": "todo", "ready": "todo", "build": "in_progress", "done": "merged"}[stage]
		_, err := db.Exec(`INSERT INTO tickets (kind, title, lane, stage, rank, created_by, created_at, updated_at, lane_changed_at, is_autonomous)
			VALUES ('story', ?, ?, ?, ?, 'Owner', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z', ?)`,
			"ticket "+stage, lane, stage, float64(i), i%2)
		if err != nil {
			t.Fatalf("insert ticket: %v", err)
		}
	}
	_, err := db.Exec(`INSERT INTO relations (from_id, to_id, kind, added_by_kind, added_by_id, added_by_name, added_at)
		VALUES (1, 2, 'caused_by', 'human', 1, 'Owner', '2026-01-01T00:00:00.000Z'),
		       (3, 4, 'relates_to', 'agent', 2, 'loop', '2026-01-01T00:00:00.000Z')`)
	if err != nil {
		t.Fatalf("insert relations: %v", err)
	}
	tickets, relations := dumpTable(t, db, "tickets"), dumpTable(t, db, "relations")

	if err := migrator.Up(); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	if got := dumpTable(t, db, "tickets"); !slices.Equal(got, tickets) {
		t.Fatalf("tickets changed by the migration:\nbefore %v\nafter  %v", tickets, got)
	}
	if got := dumpTable(t, db, "relations"); !slices.Equal(got, relations) {
		t.Fatalf("relations changed by the migration:\nbefore %v\nafter  %v", relations, got)
	}
	if got := storedCap(t, db); got != "1" {
		t.Fatalf("seeded cap %q, want 1", got)
	}
	if !tableExists(t, db, "index", "relations_depends_on") {
		t.Fatal("the depends_on index is missing")
	}

	_, err = db.Exec(`INSERT INTO relations (from_id, to_id, kind, added_by_kind, added_by_id, added_by_name, added_at)
		VALUES (2, 3, 'depends_on', 'human', 1, 'Owner', '2026-01-02T00:00:00.000Z')`)
	if err != nil {
		t.Fatalf("insert depends_on: %v", err)
	}
	if err := migrator.Migrate(7); err != nil {
		t.Fatalf("migrate down to 7: %v", err)
	}
	if got := dumpTable(t, db, "relations"); !slices.Equal(got, relations) {
		t.Fatalf("going down should drop only depends_on links:\nwant %v\ngot  %v", relations, got)
	}
	if got := dumpTable(t, db, "tickets"); !slices.Equal(got, tickets) {
		t.Fatalf("tickets changed going down:\nbefore %v\nafter  %v", tickets, got)
	}
	if tableExists(t, db, "table", "board_settings") || tableExists(t, db, "index", "relations_depends_on") {
		t.Fatal("going down left the settings table or the index behind")
	}

	if err := migrator.Up(); err != nil {
		t.Fatalf("migrate up again: %v", err)
	}
	if got := storedCap(t, db); got != "1" {
		t.Fatalf("cap after up, down, up %q, want 1", got)
	}
}
