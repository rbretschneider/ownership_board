package domain_test

import (
	"github.com/rbretschneider/ownership_board/internal/domain"
	"testing"
)

func TestDeploymentVocabulary(t *testing.T) {
	stage, err := domain.ParseStage("deployed")
	if err != nil || stage.Lane() != domain.Lane("deployed") || stage.Label() != "Deployed" {
		t.Fatalf("stage: %q %v", stage, err)
	}
	if stage.IsAgentStage() || stage.IsInFlight() || stage.MayTurnAutonomous() || stage.IsOwnerReview() {
		t.Fatal("deployed is active")
	}
	ticket := mustTicket(t)
	if got := ticket.DropStage(domain.Lane("deployed"), domain.Gates{}); got != stage {
		t.Fatalf("drop: %s", got)
	}
	ticket.MoveToStage(stage, 1, now)
	if ticket.Owner(0) != domain.OwnerNobody || ticket.IsClaimable(0, now) || ticket.NeedsOwner(0) {
		t.Fatal("completed ticket needs owner")
	}
}

func TestDeploymentCompletionAndLabels(t *testing.T) {
	for _, stage := range domain.Stages() {
		want := stage.ID == domain.StageDone || stage.ID == domain.StageDeploying || stage.ID == domain.StageDeployed
		if stage.ID.IsCompleted() != want {
			t.Fatalf("completion for %s", stage.ID)
		}
	}
	if domain.StageDone.LabelForRepo(nil) != "Merged" || domain.StageDone.LabelForRepo(&domain.Repo{DeployMode: domain.DeployModeOff}) != "Merged" || domain.StageDone.LabelForRepo(&domain.Repo{DeployMode: domain.DeployModeAgent}) != "Awaiting deploy" {
		t.Fatal("contextual merged label")
	}
	if domain.StageDeployed.LabelForRepo(nil) != "Deployed" {
		t.Fatal("deployed label")
	}
}

func TestDeploymentPolicyValidationAndRefresh(t *testing.T) {
	listing := domain.RepoListing{Provider: domain.ProviderGitHub, FullName: "acme/api"}
	repo, err := domain.NewSyncedRepo(listing, now)
	if err != nil || repo.DeployMode != domain.DeployModeOff {
		t.Fatalf("default: %+v %v", repo, err)
	}
	for _, value := range []string{"", "workflow", "AGENT", "true", " off"} {
		mode := domain.DeployMode(value)
		if err := repo.Restyle(domain.RepoStyle{DeployMode: &mode}); domain.KindOf(err) != domain.KindInvalid {
			t.Fatalf("accepted %q: %v", value, err)
		}
	}
	mode := domain.DeployModeAgent
	if err := repo.Restyle(domain.RepoStyle{DeployMode: &mode}); err != nil {
		t.Fatal(err)
	}
	if !(domain.RepoStyle{DeployMode: &mode}).ChangesGates() {
		t.Fatal("policy is not an owner setting")
	}
	if err := repo.Refresh(listing, now); err != nil || repo.DeployMode != mode {
		t.Fatalf("refresh: %v %+v", err, repo)
	}
}
