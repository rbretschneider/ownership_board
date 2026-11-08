package sqlite

import (
	"context"
	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDeploymentMigrationRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.db")
	db, m := migratorAt(t, path)
	if err := m.Migrate(15); err != nil {
		t.Fatal(err)
	}
	exec := func(sql string) {
		t.Helper()
		if _, err := db.Exec(sql); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO repos (id,provider,full_name,created_at) VALUES (1,'manual','acme/api','2026-01-01T00:00:00.000Z')`)
	exec(`INSERT INTO tickets (id,repo_id,kind,title,lane,stage,rank,spec,spec_revision,approved_spec_revision,health_check_merge_sequence,created_by,created_at,updated_at,lane_changed_at) VALUES
 (1,1,'story','release','merged','done',42,'plan',2,2,5,'Owner','2026-01-01T00:00:00.000Z','2026-01-02T00:00:00.000Z','2026-01-03T00:00:00.000Z'),
 (2,NULL,'story','unassigned','merged','done',43,'',0,0,NULL,'Owner','2026-01-01T00:00:00.000Z','2026-01-02T00:00:00.000Z','2026-01-03T00:00:00.000Z'),
 (3,1,'story','active','in_progress','pr',44,'',0,0,NULL,'Owner','2026-01-01T00:00:00.000Z','2026-01-02T00:00:00.000Z','2026-01-03T00:00:00.000Z')`)
	exec(`INSERT INTO timeline (ticket_id,type,actor_kind,actor_name,data,created_at) VALUES (1,'stage','agent','worker','{"from":"pr","to":"done"}','2026-01-02T00:00:00.000Z')`)
	ticketsBefore, historyBefore, reposBefore := dumpTable(t, db, "tickets"), dumpTable(t, db, "timeline"), dumpTable(t, db, "repos")
	verify := func() {
		t.Helper()
		var mode string
		if err := db.QueryRow(`SELECT deploy_mode FROM repos WHERE id=1`).Scan(&mode); err != nil || mode != "off" {
			t.Fatalf("policy %s: %v", mode, err)
		}
		got := dumpTable(t, db, "tickets")
		want := append([]string(nil), ticketsBefore...)
		for i := 0; i < 2; i++ {
			want[i] = strings.ReplaceAll(strings.ReplaceAll(want[i], "lane=string:merged", "lane=string:deployed"), "stage=string:done", "stage=string:deployed")
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("tickets: %v want %v", got, want)
		}
		if !reflect.DeepEqual(historyBefore, dumpTable(t, db, "timeline")) {
			t.Fatal("history changed")
		}
	}
	if err := m.Migrate(16); err != nil {
		t.Fatal(err)
	}
	verify()
	for _, mode := range []string{"workflow", "", "AGENT"} {
		if _, err := db.Exec(`UPDATE repos SET deploy_mode=?`, mode); err == nil {
			t.Fatalf("database accepted %q", mode)
		}
	}
	exec(`UPDATE repos SET deploy_mode='agent'`)
	if err := m.Steps(-1); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ticketsBefore, dumpTable(t, db, "tickets")) || !reflect.DeepEqual(reposBefore, dumpTable(t, db, "repos")) {
		t.Fatal("down failed to restore rows")
	}
	if _, err := db.Exec(`SELECT deploy_mode FROM repos`); err == nil {
		t.Fatal("column survived down")
	}
	if err := m.Up(); err != nil {
		t.Fatal(err)
	}
	verify()
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Read(context.Background(), func(r app.Repos) error {
		repo, err := r.CodeRepos().Get(context.Background(), 1)
		if err != nil {
			return err
		}
		if repo.DeployMode != domain.DeployModeOff {
			t.Fatal("store policy")
		}

		merged, err := r.Tickets().MergedSinceHealthCheck(context.Background(), 1, 0)
		if err != nil {
			return err
		}
		if len(merged) != 1 || merged[0].ID != 1 {
			t.Fatalf("migrated completion range: %+v", merged)
		}
		ticket, err := r.Tickets().Get(context.Background(), 1)
		if err == nil && ticket.Stage != domain.StageDeployed {
			t.Fatal("store stage")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
