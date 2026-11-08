package domain_test

import (
	"testing"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestDeployingIsActiveAndMergeComplete(t *testing.T) {
	stage, err := domain.ParseStage("deploying")
	if err != nil {
		t.Fatal(err)
	}
	if stage.Lane() != domain.LaneMerged || stage.Label() != "Deploying" || !stage.IsAgentStage() || !stage.IsCompleted() || stage.IsInFlight() || stage.MayTurnAutonomous() || stage.IsOwnerReview() {
		t.Fatalf("stage semantics: %s", stage)
	}
	ticket := mustTicket(t)
	ticket.MoveToStage(stage, 1, now)
	if ticket.Owner(0) != domain.OwnerAgent || !ticket.IsClaimable(0, now) {
		t.Fatal("deployment is not agent work")
	}
	action, workStage, ok := domain.ActionFor(stage)
	if !ok || action != "deploy" || workStage != stage || domain.ClaimOrder[0] != stage {
		t.Fatal("deployment dispatch/priority")
	}
}

func TestDeployRequestEligibility(t *testing.T) {
	ticket := mustTicket(t)
	ticket.RepoID = 1
	repo := domain.Repo{ID: 1, DeployMode: domain.DeployModeAgent}
	for _, info := range domain.Stages() {
		ticket.MoveToStage(info.ID, 1, now)
		err := ticket.CheckDeployRequest(&repo)
		if (err == nil) != (info.ID == domain.StageDone) {
			t.Fatalf("eligibility for %s: %v", info.ID, err)
		}
	}
	ticket.MoveToStage(domain.StageDone, 1, now)
	for _, r := range []*domain.Repo{nil, {ID: 2, DeployMode: domain.DeployModeAgent}, {ID: 1, DeployMode: domain.DeployModeOff}} {
		if err := ticket.CheckDeployRequest(r); domain.KindOf(err) != domain.KindConflict {
			t.Fatalf("invalid repo: %v", err)
		}
	}
	ticket.BlockedReason = "Needs attention"
	if err := ticket.CheckDeployRequest(&repo); domain.KindOf(err) != domain.KindConflict {
		t.Fatalf("blocked request: %v", err)
	}
}
