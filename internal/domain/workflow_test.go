package domain_test

import (
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

var bothGates = domain.Gates{Spec: true, Design: true}

func refinedTicket(t *testing.T) domain.Ticket {
	t.Helper()
	ticket := mustTicket(t)
	ticket.RepoID = 7
	return ticket
}

func TestSubmittedSpecWaitsForOwnerWhenGated(t *testing.T) {
	ticket := refinedTicket(t)
	next, err := ticket.SaveSpec("Do X", true, bothGates, 0, now)
	if err != nil || next != domain.StageSpecReview || ticket.SpecRevision != 1 {
		t.Fatalf("next %q rev %d err %v", next, ticket.SpecRevision, err)
	}
	ungated := refinedTicket(t)
	next, err = ungated.SaveSpec("Do X", true, domain.Gates{}, 0, now)
	if err != nil || next != domain.StageReady {
		t.Fatalf("ungated next %q err %v", next, err)
	}
}

func TestSpecSubmitNeedsRepoNoQuestionsAndNoBlock(t *testing.T) {
	noRepo := mustTicket(t)
	_, err := noRepo.SaveSpec("Do X", true, bothGates, 0, now)
	requireKind(t, err, domain.KindInvalid)

	asking := refinedTicket(t)
	_, err = asking.SaveSpec("Do X", true, bothGates, 2, now)
	requireKind(t, err, domain.KindConflict)

	blocked := refinedTicket(t)
	if err := blocked.Block("stuck", now); err != nil {
		t.Fatalf("Block: %v", err)
	}
	_, err = blocked.SaveSpec("Do X", true, bothGates, 0, now)
	requireKind(t, err, domain.KindConflict)
}

func TestSpecCanOnlyBeSavedWhileRefining(t *testing.T) {
	ticket := refinedTicket(t)
	ticket.MoveToStage(domain.StageBuild, 1, now)
	_, err := ticket.SaveSpec("Do X", false, bothGates, 0, now)
	requireKind(t, err, domain.KindConflict)
}

func TestApprovalMustMatchTheRevisionShown(t *testing.T) {
	ticket := refinedTicket(t)
	next, _ := ticket.SaveSpec("Do X", true, bothGates, 0, now)
	ticket.MoveToStage(next, 1, now)
	if err := ticket.EditSpecAsOwner("Do Y", now); err != nil {
		t.Fatalf("EditSpecAsOwner: %v", err)
	}
	_, err := ticket.Approve(domain.CheckpointSpec, 1)
	requireKind(t, err, domain.KindConflict)
	next, err = ticket.Approve(domain.CheckpointSpec, 2)
	if err != nil || next != domain.StageReady || ticket.ApprovedSpecRevision != 2 {
		t.Fatalf("next %q approved %d err %v", next, ticket.ApprovedSpecRevision, err)
	}
}

func TestApproveOutsideReviewConflicts(t *testing.T) {
	ticket := refinedTicket(t)
	_, err := ticket.Approve(domain.CheckpointSpec, 0)
	requireKind(t, err, domain.KindConflict)
}

func TestRequestChangesSendsDesignBack(t *testing.T) {
	ticket := refinedTicket(t)
	ticket.MoveToStage(domain.StageDesign, 1, now)
	next, err := ticket.SaveDesign("Plan", true, bothGates, 0, now)
	if err != nil || next != domain.StageDesignReview {
		t.Fatalf("SaveDesign next %q err %v", next, err)
	}
	ticket.MoveToStage(next, 1, now)
	next, err = ticket.RequestChanges(domain.CheckpointDesign, 1)
	if err != nil || next != domain.StageDesign {
		t.Fatalf("RequestChanges next %q err %v", next, err)
	}
}

func TestBlockDropsLeaseAndStopsClaims(t *testing.T) {
	ticket := refinedTicket(t)
	if err := ticket.Claim("agent-a", time.Hour, now); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if err := ticket.Block("need a\nhuman", now); err != nil {
		t.Fatalf("Block: %v", err)
	}
	if ticket.Lease != nil || ticket.IsClaimable(0, now) || !ticket.NeedsOwner(0) {
		t.Fatalf("lease %v claimable %v", ticket.Lease, ticket.IsClaimable(0, now))
	}
	if err := ticket.Unblock(now); err != nil || !ticket.IsClaimable(0, now) {
		t.Fatalf("Unblock: %v", err)
	}
}

func TestSubmitForReviewNeedsBuildAndPR(t *testing.T) {
	ticket := refinedTicket(t)
	_, err := ticket.SubmitForReview(0)
	requireKind(t, err, domain.KindConflict)
	ticket.MoveToStage(domain.StageBuild, 1, now)
	_, err = ticket.SubmitForReview(0)
	requireKind(t, err, domain.KindInvalid)
	ticket.PRURL = "https://example.com/pr/1"
	if next, err := ticket.SubmitForReview(0); err != nil || next != domain.StagePR {
		t.Fatalf("next %q err %v", next, err)
	}
}

func TestDropRespectsApprovedRevisions(t *testing.T) {
	ticket := refinedTicket(t)
	ticket.Spec, ticket.SpecRevision = "Do X", 1
	if got := ticket.DropStage(domain.LaneTodo, bothGates); got != domain.StageRefine {
		t.Fatalf("unapproved spec dropped into %s", got)
	}
	if got := ticket.DropStage(domain.LaneTodo, domain.Gates{}); got != domain.StageReady {
		t.Fatalf("ungated spec dropped into %s", got)
	}
	ticket.ApprovedSpecRevision = 1
	if got := ticket.DropStage(domain.LaneTodo, bothGates); got != domain.StageReady {
		t.Fatalf("approved spec dropped into %s", got)
	}
	if got := ticket.DropStage(domain.LaneInProgress, bothGates); got != domain.StageDesign {
		t.Fatalf("no design dropped into %s", got)
	}
}

func TestPullRequestClosedBlocksBuild(t *testing.T) {
	ticket := refinedTicket(t)
	ticket.MoveToStage(domain.StagePR, 1, now)
	ticket.PullRequestClosed(2, now)
	if ticket.Stage != domain.StageBuild || !ticket.IsBlocked() {
		t.Fatalf("stage %s blocked %v", ticket.Stage, ticket.IsBlocked())
	}
}

func TestAgentEditsAreLimitedByStage(t *testing.T) {
	ticket := refinedTicket(t)
	if err := ticket.AgentMayEdit([]string{"title", "repo"}); err != nil {
		t.Fatalf("refine edit: %v", err)
	}
	requireKind(t, ticket.AgentMayEdit([]string{"pr_url"}), domain.KindForbidden)
	ticket.MoveToStage(domain.StageBuild, 1, now)
	requireKind(t, ticket.AgentMayEdit([]string{"title"}), domain.KindForbidden)
	if err := ticket.AgentMayEdit([]string{"branch", "pr_url"}); err != nil {
		t.Fatalf("build edit: %v", err)
	}
}

func TestNoRepoMeansBothGates(t *testing.T) {
	if got := domain.GatesFor(nil); got != bothGates {
		t.Fatalf("gates %+v", got)
	}
}

func TestClaimOrderAndActions(t *testing.T) {
	action, stage, ok := domain.ActionFor(domain.StageReady)
	if !ok || action != domain.ActionDesign || stage != domain.StageDesign {
		t.Fatalf("ready: %s %s %v", action, stage, ok)
	}
	if _, _, ok := domain.ActionFor(domain.StageSpecReview); ok {
		t.Fatal("owner stage produced an agent action")
	}
	for _, stage := range domain.ClaimOrder {
		if !stage.IsAgentStage() {
			t.Errorf("claim order includes %s", stage)
		}
	}
}
