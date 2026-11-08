package sqlite

import (
	"context"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestHealthCheckFilingMigrationBackfillAndDowngrade(t *testing.T) {
	db, m := migratorAt(t, filepath.Join(t.TempDir(), "board.db"))
	if err := m.Migrate(11); err != nil {
		t.Fatal(err)
	}
	exec := func(query string) {
		t.Helper()
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO repos (id, provider, full_name, created_at, health_check_every, health_checked_through_ticket, health_checked_at) VALUES
 (1, 'manual', 'acme/api', '2026-01-01T00:00:00.000Z', 3, 1, '2026-01-04T00:00:00.000Z'),
 (2, 'manual', 'acme/web', '2026-01-01T00:00:00.000Z', 0, 999, '2026-01-02T00:00:00.000Z'),
 (3, 'manual', 'acme/tools', '2026-01-01T00:00:00.000Z', 8, NULL, NULL)`)
	exec(`INSERT INTO tickets (id, repo_id, kind, title, stage, lane, rank, created_by, created_at, updated_at, lane_changed_at) VALUES
 (1, 1, 'story', 'one', 'done', 'merged', 1, 'owner', '2026-01-01T00:00:00.000Z', '2026-01-09T00:00:00.000Z', '2026-01-09T00:00:00.000Z'),
 (2, 1, 'story', 'two', 'done', 'merged', 2, 'owner', '2026-01-01T00:00:00.000Z', '2026-01-02T00:00:00.000Z', '2026-01-02T00:00:00.000Z'),
 (3, 1, 'story', 'open', 'ready', 'todo', 3, 'owner', '2026-01-01T00:00:00.000Z', '2026-01-02T00:00:00.000Z', '2026-01-02T00:00:00.000Z'),
 (4, 2, 'story', 'four', 'done', 'merged', 4, 'owner', '2026-01-01T00:00:00.000Z', '2026-01-02T00:00:00.000Z', '2026-01-02T00:00:00.000Z'),
 (5, 2, 'story', 'five', 'done', 'merged', 5, 'owner', '2026-01-01T00:00:00.000Z', '2026-01-03T00:00:00.000Z', '2026-01-03T00:00:00.000Z'),
 (6, 1, 'story', 'equal time', 'done', 'merged', 6, 'owner', '2026-01-01T00:00:00.000Z', '2026-01-02T00:00:00.000Z', '2026-01-02T00:00:00.000Z'),
 (7, NULL, 'story', 'unassigned', 'done', 'merged', 7, 'owner', '2026-01-01T00:00:00.000Z', '2026-01-02T00:00:00.000Z', '2026-01-02T00:00:00.000Z')`)
	exec(`INSERT INTO timeline (ticket_id, type, actor_kind, actor_name, data, created_at) VALUES
 (1, 'stage', 'human', 'owner', '{"to":"done"}', '2026-01-01T00:00:00.000Z'),
 (1, 'stage', 'human', 'owner', '{"to":"done"}', '2026-01-09T00:00:00.000Z')`)
	reposBefore, ticketsBefore, timelineBefore := dumpTable(t, db, "repos"), dumpTable(t, db, "tickets"), dumpTable(t, db, "timeline")
	verify := func() {
		t.Helper()
		rows, err := db.Query(`SELECT id, COALESCE(health_check_merge_sequence, 0), is_health_check FROM tickets ORDER BY id`)
		if err != nil {
			t.Fatal(err)
		}
		var got [][3]int64
		for rows.Next() {
			var row [3]int64
			if err := rows.Scan(&row[0], &row[1], &row[2]); err != nil {
				t.Fatal(err)
			}
			got = append(got, row)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		want := [][3]int64{{1, 1, 0}, {2, 2, 0}, {3, 0, 0}, {4, 1, 0}, {5, 2, 0}, {6, 3, 0}, {7, 0, 0}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("backfill: %v", got)
		}
		for _, want := range [][4]int64{{1, 3, 1, 3}, {2, 2, 1, 0}, {3, 0, 0, 8}} {
			var got [4]int64
			if err := db.QueryRow(`SELECT id, merge_sequence, health_checked_through_merge_sequence, health_check_every FROM repos WHERE id = ?`, want[0]).Scan(&got[0], &got[1], &got[2], &got[3]); err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("repo: %v want %v", got, want)
			}
		}
	}
	if err := m.Migrate(12); err != nil {
		t.Fatal(err)
	}
	verify()
	if err := m.Migrate(11); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string][]string{"repos": reposBefore, "tickets": ticketsBefore, "timeline": timelineBefore} {
		if got := dumpTable(t, db, name); !slices.Equal(got, want) {
			t.Fatalf("%s changed across down: %v vs %v", name, got, want)
		}
	}
	if err := m.Migrate(12); err != nil {
		t.Fatal(err)
	}
	verify()
}

func TestHealthCheckRangePredicatesAndPurposeRoundTrip(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "board.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	err = db.Write(ctx, func(repos app.Repos) error {
		repo, _ := domain.NewManualRepo(domain.ManualRepoDraft{FullName: "acme/api"}, time.Now())
		if err := repos.CodeRepos().Insert(ctx, &repo); err != nil {
			return err
		}
		other, _ := domain.NewManualRepo(domain.ManualRepoDraft{FullName: "acme/web"}, time.Now())
		if err := repos.CodeRepos().Insert(ctx, &other); err != nil {
			return err
		}
		var ids []int64
		for i := 1; i <= 6; i++ {
			ticket, _ := domain.NewTicket(domain.TicketDraft{Kind: "story", Title: "change", RepoID: repo.ID}, domain.SystemActor, time.Now())
			ticket.Stage, ticket.Lane, ticket.HealthCheckMergeSequence = domain.StageDone, domain.LaneMerged, int64(i)
			switch i {
			case 2:
				ticket.Stage, ticket.Lane = domain.StageReady, domain.LaneTodo
			case 3:
				ticket.IsHealthCheck = true
			case 4:
				ticket.RepoID = other.ID
			}
			if err := repos.Tickets().Insert(ctx, &ticket); err != nil {
				return err
			}
			ids = append(ids, ticket.ID)
			got, err := repos.Tickets().Get(ctx, ticket.ID)
			if err != nil {
				return err
			}
			if got.IsHealthCheck != ticket.IsHealthCheck || got.HealthCheckMergeSequence != ticket.HealthCheckMergeSequence {
				t.Fatal("metadata not persisted")
			}
		}
		got, err := repos.Tickets().MergedSinceHealthCheck(ctx, repo.ID, 1)
		if err != nil {
			return err
		}
		if len(got) != 2 || got[0].ID != ids[4] || got[1].ID != ids[5] {
			t.Fatalf("range: %+v", got)
		}
		open, err := repos.Tickets().HasOpenHealthCheck(ctx, repo.ID)
		if err != nil {
			return err
		}
		if open {
			t.Fatal("done check was open")
		}
		check, err := repos.Tickets().Get(ctx, ids[2])
		if err != nil {
			return err
		}
		check.Stage, check.Lane, check.BlockedReason = domain.StageBuild, domain.LaneInProgress, "blocked"
		if err := repos.Tickets().Update(ctx, check); err != nil {
			return err
		}
		open, err = repos.Tickets().HasOpenHealthCheck(ctx, repo.ID)
		if err != nil {
			return err
		}
		if !open {
			t.Fatal("blocked check was not open")
		}
		open, err = repos.Tickets().HasOpenHealthCheck(ctx, other.ID)
		if open {
			t.Fatal("open check leaked across repos")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
