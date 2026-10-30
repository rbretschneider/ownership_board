package backup

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/clock/clocktest"
	"github.com/rbretschneider/ownership_board/internal/domain"
	"github.com/rbretschneider/ownership_board/internal/store/sqlite"
)

var owner = domain.Actor{Kind: domain.ActorHuman, ID: 1, Name: "Owner"}

// seedBoard writes two tickets, one depending on the other, and a changed
// board setting.
func seedBoard(t *testing.T, ctx context.Context, db *sqlite.DB, clk *clocktest.Clock) {
	t.Helper()
	err := db.Write(ctx, func(repos app.Repos) error {
		var ids []int64
		for _, title := range []string{"prerequisite", "dependant"} {
			ticket, err := domain.NewTicket(domain.TicketDraft{Kind: string(domain.KindStory), Title: title}, owner, clk.Now())
			if err != nil {
				return err
			}
			if err := repos.Tickets().Insert(ctx, &ticket); err != nil {
				return err
			}
			ids = append(ids, ticket.ID)
		}
		relation, err := domain.NewRelation(ids[1], ids[0], domain.RelationDependsOn, owner, clk.Now())
		if err != nil {
			return err
		}
		if err := repos.Relations().Insert(ctx, relation); err != nil {
			return err
		}
		return repos.Settings().Save(ctx, domain.BoardSettings{AutonomousInFlightCap: 3}, owner, clk.Now())
	})
	if err != nil {
		t.Fatalf("seed the board: %v", err)
	}
}

func TestABackupRestoresDependenciesAndSettings(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	clk := clocktest.New()
	db, err := sqlite.Open(ctx, filepath.Join(dir, "board.db"))
	if err != nil {
		t.Fatalf("open the board: %v", err)
	}
	defer db.Close()
	seedBoard(t, ctx, db, clk)

	backups := New(db.SQL(), filepath.Join(dir, "backups"), 2, clk, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := backups.backUpIfDue(ctx); err != nil {
		t.Fatalf("back up: %v", err)
	}
	files, err := backups.snapshots()
	if err != nil || len(files) != 1 {
		t.Fatalf("snapshots after one backup: %v, %v", files, err)
	}

	restoredPath := filepath.Join(dir, "restored.db")
	snapshot, err := os.ReadFile(files[0].path)
	if err != nil {
		t.Fatalf("read the snapshot: %v", err)
	}
	if err := os.WriteFile(restoredPath, snapshot, 0o600); err != nil {
		t.Fatalf("restore the snapshot: %v", err)
	}
	restored, err := sqlite.Open(ctx, restoredPath)
	if err != nil {
		t.Fatalf("open the restored board: %v", err)
	}
	defer restored.Close()

	err = restored.Read(ctx, func(repos app.Repos) error {
		waiting, err := repos.Relations().Waiting(ctx)
		if err != nil {
			return err
		}
		if got := waiting[2]; !slices.Equal(got, []int64{1}) {
			t.Errorf("restored ticket 2 waits on %v, want [1]", got)
		}
		settings, err := repos.Settings().Get(ctx)
		if err != nil {
			return err
		}
		if settings.AutonomousInFlightCap != 3 {
			t.Errorf("restored in-flight cap %d, want 3", settings.AutonomousInFlightCap)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read the restored board: %v", err)
	}
}
