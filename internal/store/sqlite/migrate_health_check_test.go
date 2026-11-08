package sqlite

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestHealthCheckMigrationKeepsRepoSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.db")
	db, migrator := migratorAt(t, path)
	if err := migrator.Migrate(10); err != nil {
		t.Fatalf("migrate to 10: %v", err)
	}
	_, err := db.Exec(`INSERT INTO repos (provider, full_name, url, description, color, default_branch, is_private, is_archived, created_at,
			treat_as_public, require_spec_approval, require_design_approval, allow_agent_merge)
		VALUES ('manual', 'acme/api', '', 'one', '#aabbcc', 'main', 0, 0, '2026-01-01T00:00:00.000Z', 1, 0, 1, 1),
		       ('github', 'acme/web', '', 'two', '', 'trunk', 1, 1, '2026-01-02T00:00:00.000Z', 0, 1, 0, 0)`)
	if err != nil {
		t.Fatalf("insert repos: %v", err)
	}
	before := dumpTable(t, db, "repos")

	if err := migrator.Migrate(11); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	after := dumpTable(t, db, "repos")
	if len(after) != len(before) {
		t.Fatalf("repo count changed: %d -> %d", len(before), len(after))
	}
	for i, row := range after {
		for _, want := range []string{"health_check_every=int64:8", "health_checked_through_ticket=<nil>:<nil>", "health_checked_at=<nil>:<nil>"} {
			if !strings.Contains(row, want) {
				t.Errorf("row %d lacks %q: %s", i, want, row)
			}
		}
		if !strings.HasPrefix(row, before[i]) {
			t.Errorf("row %d changed:\nbefore %s\nafter  %s", i, before[i], row)
		}
	}

	if err := migrator.Migrate(10); err != nil {
		t.Fatalf("migrate down: %v", err)
	}
	if got := dumpTable(t, db, "repos"); !slices.Equal(got, before) {
		t.Fatalf("repos changed going down:\nbefore %v\nafter  %v", before, got)
	}
	if err := migrator.Migrate(11); err != nil {
		t.Fatalf("migrate up again: %v", err)
	}
}

func TestHealthCheckSettingsRoundTripAndSurviveSync(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "board.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	now := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	repo, err := domain.NewManualRepo(domain.ManualRepoDraft{FullName: "acme/api"}, now)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	var got domain.Repo
	err = db.Write(ctx, func(repos app.Repos) error {
		if err := repos.CodeRepos().Insert(ctx, &repo); err != nil {
			return err
		}
		fresh, err := repos.CodeRepos().Get(ctx, repo.ID)
		if err != nil {
			return err
		}
		if fresh.HealthCheckEvery != 8 || fresh.HealthCheckedThroughTicket != nil || fresh.HealthCheckedAt != nil {
			t.Errorf("fresh repo: every %d through %v at %v", fresh.HealthCheckEvery, fresh.HealthCheckedThroughTicket, fresh.HealthCheckedAt)
		}
		repo.HealthCheckEvery = 0
		if err := repos.CodeRepos().SetHealthCheck(ctx, repo); err != nil {
			return err
		}
		if err := repos.CodeRepos().RecordHealthCheck(ctx, repo.ID, 42, 5, now); err != nil {
			return err
		}
		repo.HealthCheckEvery = 30
		repo.HealthCheckedThroughTicket = nil
		if err := repos.CodeRepos().Update(ctx, repo); err != nil {
			return err
		}
		got, err = repos.CodeRepos().Get(ctx, repo.ID)
		return err
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if got.HealthCheckEvery != 0 {
		t.Errorf("Update changed the interval to %d", got.HealthCheckEvery)
	}
	if got.HealthCheckedThroughTicket == nil || *got.HealthCheckedThroughTicket != 42 || got.HealthCheckedAt == nil || !got.HealthCheckedAt.Equal(now) {
		t.Errorf("checkpoint not kept: %v %v", got.HealthCheckedThroughTicket, got.HealthCheckedAt)
	}
}
