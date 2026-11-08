package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestDeployingPersistenceAndCompletionPredicates(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "board.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var id, dependentID, healthID int64
	err = db.Write(ctx, func(r app.Repos) error {
		repo := domain.Repo{Provider: domain.ProviderManual, FullName: "acme/service", DeployMode: domain.DeployModeAgent, CreatedAt: now}
		if err := r.CodeRepos().Insert(ctx, &repo); err != nil {
			return err
		}
		ticket, err := domain.NewTicket(domain.TicketDraft{Kind: "story", Title: "Delivery", RepoID: repo.ID}, domain.Actor{Kind: domain.ActorHuman, Name: "Owner"}, now)
		if err != nil {
			return err
		}
		ticket.MoveToStage(domain.StageDeploying, 10, now)
		ticket.HealthCheckMergeSequence = 1
		if err := ticket.Claim("worker", time.Hour, now); err != nil {
			return err
		}
		if err := r.Tickets().Insert(ctx, &ticket); err != nil {
			return err
		}
		id = ticket.ID
		ticket.ID = 0
		ticket.Title = "Review"
		ticket.IsHealthCheck = true
		ticket.HealthCheckMergeSequence = 0
		if err := r.Tickets().Insert(ctx, &ticket); err != nil {
			return err
		}
		healthID = ticket.ID
		dependent, err := domain.NewTicket(domain.TicketDraft{Kind: "story", Title: "Dependent", RepoID: repo.ID}, domain.Actor{Kind: domain.ActorHuman, Name: "Owner"}, now)
		if err != nil {
			return err
		}
		if err := r.Tickets().Insert(ctx, &dependent); err != nil {
			return err
		}
		dependentID = dependent.ID
		relation := domain.Relation{FromID: dependentID, ToID: id, Kind: domain.RelationDependsOn, AddedAt: now, AddedBy: domain.Actor{Kind: domain.ActorHuman, Name: "Owner"}}
		if err := r.Relations().Insert(ctx, relation); err != nil {
			return err
		}
		got, err := r.Tickets().Get(ctx, id)
		if err != nil {
			return err
		}
		if got.Stage != domain.StageDeploying || got.Lease == nil || got.Lease.Holder != "worker" || !got.Lease.ExpiresAt.Equal(now.Add(time.Hour)) {
			t.Fatalf("round trip: %+v", got)
		}
		waiting, err := r.Relations().Waiting(ctx)
		if err != nil {
			return err
		}
		if len(waiting[dependentID]) != 0 {
			t.Fatal("deploying prerequisite is waiting")
		}
		waitingOn, err := r.Relations().WaitingOn(ctx, dependentID)
		if err != nil {
			return err
		}
		if len(waitingOn) != 0 {
			t.Fatal("deploying prerequisite is unresolved")
		}
		candidate, found, err := r.Tickets().NextClaimable(ctx, domain.StageRefine, app.TicketFilter{}, now)
		if err != nil {
			return err
		}
		if !found || candidate.ID != dependentID {
			t.Fatal("SQL claim excluded completed prerequisite")
		}
		merged, err := r.Tickets().MergedSinceHealthCheck(ctx, repo.ID, 0)
		if err != nil {
			return err
		}
		if len(merged) != 1 || merged[0].ID != id {
			t.Fatalf("merged range: %+v", merged)
		}
		open, err := r.Tickets().HasOpenHealthCheck(ctx, repo.ID)
		if err != nil {
			return err
		}
		if open {
			t.Fatal("deploying health check is open")
		}
		_, found, err = r.Tickets().NextClaimable(ctx, domain.StageDeploying, app.TicketFilter{}, now)
		if err != nil {
			return err
		}
		if found {
			t.Fatal("live deployment lease reclaimed")
		}
		candidate, found, err = r.Tickets().NextClaimable(ctx, domain.StageDeploying, app.TicketFilter{}, now.Add(time.Hour))
		if err != nil {
			return err
		}
		if !found || (candidate.ID != id && candidate.ID != healthID) {
			t.Fatal("expired deployment missing")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
