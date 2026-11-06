package app_test

import (
	"testing"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func (f *fixture) toBuildWith(id int64, design string) {
	f.t.Helper()
	if _, err := f.board.SaveDesign(f.ctx, owner, id, app.SaveTextInput{Text: design}); err != nil {
		f.t.Fatalf("save design: %v", err)
	}
	f.move(owner, id, domain.LaneInProgress, nil)
	f.requireStage(id, domain.StageBuild)
}

func TestABuildSharingFilesWithOneUnderWayWaitsUntilItMerges(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	first, overlapping, separate := f.ticket("first", api.ID), f.ticket("overlapping", api.ID), f.ticket("separate", api.ID)
	for _, id := range []int64{first, overlapping, separate} {
		f.toReady(id)
	}
	// Each move lands on top of In Progress, so the first one moved is last.
	f.toBuildWith(separate, "Add src/export/src/StlWriter.cpp and its tests.")
	f.toBuildWith(overlapping, "Extend src/rebuild/src/RebuildEngine.cpp for suppression.")
	f.toBuildWith(first, "Change `src/rebuild/src/RebuildEngine.cpp` and src/rebuild/include/mc/rebuild/ModelState.h.")

	if work := f.mustClaim(agent, app.ClaimInput{}); work.Ticket.Ticket.ID != first {
		t.Fatalf("claimed #%d, want #%d", work.Ticket.Ticket.ID, first)
	}
	branch := "ob/first"
	if _, err := f.board.Edit(f.ctx, agent, first, domain.TicketEdit{Branch: &branch}); err != nil {
		t.Fatalf("set branch: %v", err)
	}
	if work := f.mustClaim(other, app.ClaimInput{}); work.Ticket.Ticket.ID != separate {
		t.Fatalf("claimed #%d, want the separate build #%d past the overlapping one", work.Ticket.Ticket.ID, separate)
	}
	third := domain.Actor{Kind: domain.ActorAgent, ID: 9, Name: "loop-c"}
	f.requireNoClaim(third, "a build sharing files with one under way")

	f.move(owner, first, domain.LaneMerged, nil)
	if work := f.mustClaim(third, app.ClaimInput{}); work.Ticket.Ticket.ID != overlapping {
		t.Fatalf("claimed #%d after the first merged, want #%d", work.Ticket.Ticket.ID, overlapping)
	}
}

func TestAStartedBuildIsNeverHeldBack(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	a, b := f.ticket("a", api.ID), f.ticket("b", api.ID)
	f.toReady(a)
	f.toReady(b)
	f.toBuildWith(a, "Edit src/core/Engine.cpp.")
	f.toBuildWith(b, "Edit src/core/Engine.cpp too.")
	for _, id := range []int64{a, b} {
		branch := "ob/x"
		if _, err := f.board.Edit(f.ctx, owner, id, domain.TicketEdit{Branch: &branch}); err != nil {
			t.Fatalf("set branch: %v", err)
		}
	}
	f.mustClaim(agent, app.ClaimInput{})
	f.mustClaim(other, app.ClaimInput{})
}
