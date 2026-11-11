package app_test

import (
	"strings"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

// A ticket may be designed once each dependency is being built on a branch
// (2026-11-10, owner): the design is written while the dependency waits on a
// slow verification queue. Its build still waits for the merge, and designing
// ahead goes only one level deep.
func TestADesignStartsWhileItsDependencyIsBuiltButTheBuildWaitsForTheMerge(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	dep, next, third := f.ticket("dependency", api.ID), f.ticket("next", api.ID), f.ticket("third", api.ID)
	for _, id := range []int64{dep, next, third} {
		f.toReady(id)
	}
	f.mustLink(owner, next, dep)
	f.mustLink(owner, third, next)
	f.toBuild(dep)
	if work := f.mustClaim(other, app.ClaimInput{}); work.Ticket.Ticket.ID != dep {
		t.Fatalf("claimed #%d, want the dependency #%d", work.Ticket.Ticket.ID, dep)
	}
	f.requireNoClaim(agent, "a design whose dependency has not started on a branch")

	branch := "ob/dependency"
	if _, err := f.board.Edit(f.ctx, owner, dep, domain.TicketEdit{Branch: &branch}); err != nil {
		t.Fatalf("set branch: %v", err)
	}
	work := f.mustClaim(agent, app.ClaimInput{})
	if work.Ticket.Ticket.ID != next || work.Action != domain.ActionDesign {
		t.Fatalf("claimed #%d for %s, want a design of #%d", work.Ticket.Ticket.ID, work.Action, next)
	}
	if !strings.Contains(work.Brief, "## Designing ahead") || !strings.Contains(work.Brief, "have not merged: #") {
		t.Fatalf("the brief does not say it designs ahead:\n%s", work.Brief)
	}
	if _, err := f.board.SaveDesign(f.ctx, agent, next, app.SaveTextInput{Text: "The plan.", IsSubmit: true}); err != nil {
		t.Fatalf("submit design: %v", err)
	}
	f.requireStage(next, domain.StageBuild)
	third2 := domain.Actor{Kind: domain.ActorAgent, ID: 9, Name: "loop-c"}
	// Neither the designed build (dependency unmerged) nor the ticket behind
	// it (its dependency has no branch) can be claimed.
	f.requireNoClaim(third2, "a designed-ahead build or a second level of designing ahead")

	f.move(owner, dep, domain.LaneMerged, nil)
	if work := f.mustClaim(third2, app.ClaimInput{}); work.Ticket.Ticket.ID != next || work.Action != domain.ActionImplement {
		t.Fatalf("claimed #%d for %s after the merge, want the build of #%d", work.Ticket.Ticket.ID, work.Action, next)
	}
}

// A designed-ahead autonomous build waiting for its dependency holds no
// worker, so it does not use up the in-flight cap.
func TestADesignedAheadBuildWaitingOnItsDependencyDoesNotFillTheCap(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	f.setInFlightCap(2)
	dep := f.mustCreateAutonomous(api.ID)
	waiting := f.mustCreateAutonomous(api.ID)
	f.mustLink(owner, waiting, dep)
	if work := f.mustClaim(other, app.ClaimInput{}); work.Ticket.Ticket.ID != dep {
		t.Fatalf("claimed #%d, want #%d", work.Ticket.Ticket.ID, dep)
	}
	if _, err := f.board.SaveDesign(f.ctx, other, dep, app.SaveTextInput{Text: "Plan.", IsSubmit: true}); err != nil {
		t.Fatalf("submit design: %v", err)
	}
	branch := "ob/dep"
	if _, err := f.board.Edit(f.ctx, owner, dep, domain.TicketEdit{Branch: &branch}); err != nil {
		t.Fatalf("set branch: %v", err)
	}
	if work := f.mustClaim(other, app.ClaimInput{}); work.Ticket.Ticket.ID != dep {
		t.Fatalf("claimed #%d, want the build of #%d", work.Ticket.Ticket.ID, dep)
	}
	if work := f.mustClaim(agent, app.ClaimInput{}); work.Ticket.Ticket.ID != waiting {
		t.Fatalf("claimed #%d, want the design of #%d", work.Ticket.Ticket.ID, waiting)
	}
	if _, err := f.board.SaveDesign(f.ctx, agent, waiting, app.SaveTextInput{Text: "Plan.", IsSubmit: true}); err != nil {
		t.Fatalf("submit design: %v", err)
	}
	f.requireStage(waiting, domain.StageBuild)
	// dep holds one slot; waiting does not, so a new autonomous ticket can start.
	fresh := f.mustCreateAutonomous(api.ID)
	third := domain.Actor{Kind: domain.ActorAgent, ID: 9, Name: "loop-c"}
	if work := f.mustClaim(third, app.ClaimInput{}); work.Ticket.Ticket.ID != fresh {
		t.Fatalf("claimed #%d, want the new autonomous #%d", work.Ticket.Ticket.ID, fresh)
	}
}
