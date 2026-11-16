package sqlite

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// The verdicts migration carries over what finished build jobs already say,
// skipping failures the host blamed on itself.
func TestVerdictsMigrationCarriesOverJobVerdicts(t *testing.T) {
	db, m := migratorAt(t, filepath.Join(t.TempDir(), "board.db"))
	if err := m.Migrate(24); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO repos(id,provider,full_name,created_at) VALUES(1,'manual','example/tools','2026-01-01T00:00:00.000Z')`,
		`INSERT INTO tickets(id,repo_id,kind,title,stage,lane,rank,created_by,created_at,updated_at,lane_changed_at) VALUES(1,1,'story','Example','ready','todo',1,'Owner','2026-01-01T00:00:00.000Z','2026-01-01T00:00:00.000Z','2026-01-01T00:00:00.000Z')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	job := func(n int, status, target, result string) {
		t.Helper()
		var res any
		if result != "" {
			res = result
		}
		_, err := db.Exec(`INSERT INTO build_jobs(id,ticket_id,repo,sha,preset,target,target_os,status,attempt,created_at,finished_at,result,bundle_bytes,bundle_sha256)
			VALUES(?,1,'example/tools',?,'cmake',?,json_extract(?,'$.os'),?,1,'2026-01-01T00:00:00.000Z','2026-01-01T01:00:00.000Z',?,3,?)`,
			fmt.Sprintf("%064x", n), fmt.Sprintf("%040x", n), target, target, status, res, strings.Repeat("f", 64))
		if err != nil {
			t.Fatal(err)
		}
	}
	job(1, "passed", `{"os":"windows","required_capabilities":["msvc","cuda"]}`, `{"commands":[{"label":"b","exit_code":0,"log":""}]}`)
	job(2, "failed", `{"os":"linux","required_capabilities":[]}`, `{"commands":[{"label":"t","exit_code":1,"log":""}]}`)
	job(3, "failed", `{"os":"linux","required_capabilities":[]}`, `{"commands":[],"failure_kind":"infrastructure"}`)
	job(4, "queued", `{"os":"linux","required_capabilities":[]}`, "")
	if err := m.Migrate(25); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`SELECT sha, target, status, job_id FROM verdicts ORDER BY sha`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var sha, target, status, jobID string
		if err := rows.Scan(&sha, &target, &status, &jobID); err != nil {
			t.Fatal(err)
		}
		got = append(got, fmt.Sprintf("%s %s %s %s", sha[38:], target, status, jobID[62:]))
	}
	want := []string{"01 windows:cuda,msvc passed 01", "02 linux failed 02"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("verdicts %q, want %q", got, want)
	}
	if err := m.Migrate(24); err != nil {
		t.Fatal(err)
	}
}
