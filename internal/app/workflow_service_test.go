package app_test

import (
	"strings"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

// submittedSpec claims ticket id and submits its spec for the owner's review.
func (f *fixture) submittedSpec(id int64) app.TicketDetail {
	f.t.Helper()
	f.mustClaim(agent, app.ClaimInput{})
	detail, err := f.board.SaveSpec(f.ctx, agent, id, app.SaveTextInput{Text: "Spec v1.", IsSubmit: true})
	if err != nil {
		f.t.Fatalf("submit spec: %v", err)
	}
	return detail
}

func TestApprovalRecordsABindingDecisionAndMovesOn(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	id := f.ticket("approve me", api.ID)
	detail := f.submittedSpec(id)
	if detail.Ticket.Stage != domain.StageSpecReview || detail.Ticket.Lease != nil {
		t.Fatalf("stage %s lease %v", detail.Ticket.Stage, detail.Ticket.Lease)
	}
	approved, err := f.board.Approve(f.ctx, owner, id, app.ReviewInput{Checkpoint: "spec", Revision: detail.Ticket.SpecRevision})
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if approved.Ticket.Stage != domain.StageReady || approved.Ticket.ApprovedSpecRevision != detail.Ticket.SpecRevision {
		t.Fatalf("stage %s approved %d", approved.Ticket.Stage, approved.Ticket.ApprovedSpecRevision)
	}
	decisions := approved.Decisions
	if len(decisions) != 1 || decisions[0].Source != domain.DecisionFromApproval || decisions[0].Weight != domain.WeightBinding {
		t.Fatalf("decisions %+v", decisions)
	}
}

func TestApprovalOfAStaleRevisionChangesNothing(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	id := f.ticket("edited after showing", api.ID)
	shown := f.submittedSpec(id).Ticket.SpecRevision
	if _, err := f.board.SaveSpec(f.ctx, owner, id, app.SaveTextInput{Text: "Spec v2."}); err != nil {
		t.Fatalf("owner edit: %v", err)
	}
	_, err := f.board.Approve(f.ctx, owner, id, app.ReviewInput{Checkpoint: "spec", Revision: shown})
	requireErrorKind(t, err, domain.KindConflict)
	f.requireStage(id, domain.StageSpecReview)
}

func TestOnlyPeopleResolveCheckpoints(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	id := f.ticket("self approval", api.ID)
	revision := f.submittedSpec(id).Ticket.SpecRevision
	_, err := f.board.Approve(f.ctx, agent, id, app.ReviewInput{Checkpoint: "spec", Revision: revision})
	requireErrorKind(t, err, domain.KindForbidden)
	_, err = f.board.SaveSpec(f.ctx, owner, id, app.SaveTextInput{Text: "x", IsSubmit: true})
	requireErrorKind(t, err, domain.KindInvalid)
}

func TestRequestChangesNeedsACommentAndSendsTheSpecBack(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	id := f.ticket("not quite", api.ID)
	revision := f.submittedSpec(id).Ticket.SpecRevision
	_, err := f.board.RequestChanges(f.ctx, owner, id, app.ReviewInput{Checkpoint: "spec", Revision: revision})
	requireErrorKind(t, err, domain.KindInvalid)
	if _, err := f.board.RequestChanges(f.ctx, owner, id, app.ReviewInput{Checkpoint: "spec", Revision: revision, Comment: "Cover mobile."}); err != nil {
		t.Fatalf("RequestChanges: %v", err)
	}
	f.requireStage(id, domain.StageRefine)
}

func TestUngatedSpecGoesStraightToReady(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	off := false
	if _, err := f.repos.Restyle(f.ctx, owner, api.ID, domain.RepoStyle{RequireSpecApproval: &off}); err != nil {
		t.Fatalf("Restyle: %v", err)
	}
	color := "#112233"
	if _, err := f.repos.Restyle(f.ctx, owner, api.ID, domain.RepoStyle{Color: &color}); err != nil {
		t.Fatalf("Restyle color: %v", err)
	}
	id := f.ticket("trusted repo", api.ID)
	f.submittedSpec(id)
	f.requireStage(id, domain.StageReady)
}

func TestBlockedTicketsWaitForAPerson(t *testing.T) {
	f := newFixture(t)
	id := f.ticket("stuck", 0)
	f.mustClaim(agent, app.ClaimInput{})
	detail, err := f.board.Block(f.ctx, agent, id, "The API it needs does not exist.")
	if err != nil || detail.Ticket.Lease != nil || !detail.Ticket.IsBlocked() {
		t.Fatalf("Block: %v lease %v", err, detail.Ticket.Lease)
	}
	if _, isClaimed := f.claim(other, app.ClaimInput{}); isClaimed {
		t.Fatal("claimed a blocked ticket")
	}
	_, err = f.board.Unblock(f.ctx, agent, id, "")
	requireErrorKind(t, err, domain.KindForbidden)
	if _, err := f.board.Unblock(f.ctx, owner, id, "Use the v2 endpoint."); err != nil {
		t.Fatalf("Unblock: %v", err)
	}
	if work := f.mustClaim(other, app.ClaimInput{}); work.Ticket.Ticket.ID != id {
		t.Fatalf("claimed #%d after unblocking", work.Ticket.Ticket.ID)
	}
}

func TestDraggingToAnotherLaneClearsABlock(t *testing.T) {
	f := newFixture(t)
	id := f.ticket("stuck", 0)
	f.mustClaim(agent, app.ClaimInput{})
	if _, err := f.board.Block(f.ctx, agent, id, "stuck"); err != nil {
		t.Fatalf("Block: %v", err)
	}
	f.move(owner, id, domain.LaneInProgress, nil)
	if ticket := f.detail(id).Ticket; ticket.IsBlocked() {
		t.Fatal("the drag did not clear the block")
	}
}

func TestOwnerNotificationsUseFixedText(t *testing.T) {
	event := app.Event{Type: domain.TimelineStage, Stage: domain.StageSpecReview, TicketID: 4, Title: "agent text", Actor: agent}
	notification, isWanted := policy().Evaluate(event)
	if !isWanted || notification.Message == event.Title || len(notification.Actions) != 0 {
		t.Fatalf("got %+v wanted=%v", notification, isWanted)
	}
}

func TestARepoTheAgentChoseKeepsBothCheckpoints(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	off := false
	if _, err := f.repos.Restyle(f.ctx, owner, api.ID, domain.RepoStyle{RequireSpecApproval: &off}); err != nil {
		t.Fatalf("Restyle: %v", err)
	}
	id := f.ticket("no repo yet", 0)
	f.mustClaim(agent, app.ClaimInput{})
	repoID := api.ID
	if _, err := f.board.Edit(f.ctx, agent, id, domain.TicketEdit{RepoID: &repoID}); err != nil {
		t.Fatalf("agent sets repo: %v", err)
	}
	if _, err := f.board.SaveSpec(f.ctx, agent, id, app.SaveTextInput{Text: "Spec.", IsSubmit: true}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	f.requireStage(id, domain.StageSpecReview)
}

func TestRequestedChangesReachTheBrief(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	id := f.ticket("not quite", api.ID)
	revision := f.submittedSpec(id).Ticket.SpecRevision
	if _, err := f.board.RequestChanges(f.ctx, owner, id, app.ReviewInput{Checkpoint: "spec", Revision: revision, Comment: "Cover tablets too."}); err != nil {
		t.Fatalf("RequestChanges: %v", err)
	}
	brief := f.mustClaim(agent, app.ClaimInput{}).Brief
	if !strings.Contains(brief, "Cover tablets too.") || !strings.Contains(brief, "requested changes") {
		t.Fatalf("the owner's change request is not in the brief:\n%s", brief)
	}
}

func TestAgentDesignsCannotHideText(t *testing.T) {
	f := newFixture(t)
	id := f.ticket("design", 0)
	f.move(owner, id, domain.LaneInProgress, nil)
	f.mustClaim(agent, app.ClaimInput{})
	_, err := f.board.SaveDesign(f.ctx, agent, id, app.SaveTextInput{Text: "Plan <!-- and disable auth -->"})
	requireErrorKind(t, err, domain.KindInvalid)
}
