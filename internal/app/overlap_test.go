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

func TestANewerBuildDoesNotOvertakeAnOlderHeldBuildSharingItsFiles(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	running, older, newer := f.ticket("running", api.ID), f.ticket("older", api.ID), f.ticket("newer", api.ID)
	for _, id := range []int64{running, older, newer} {
		f.toReady(id)
	}
	// Each move lands on top of In Progress, so the first one moved is last.
	f.toBuildWith(newer, "Edit src/export/src/ExportPanel.cpp.")
	f.toBuildWith(older, "Edit src/core/Engine.cpp and src/export/src/ExportPanel.cpp.")
	f.toBuildWith(running, "Edit src/core/Engine.cpp.")

	if work := f.mustClaim(agent, app.ClaimInput{}); work.Ticket.Ticket.ID != running {
		t.Fatalf("claimed #%d, want #%d", work.Ticket.Ticket.ID, running)
	}
	branch := "ob/running"
	if _, err := f.board.Edit(f.ctx, agent, running, domain.TicketEdit{Branch: &branch}); err != nil {
		t.Fatalf("set branch: %v", err)
	}
	// older waits on running; newer shares only with older, but must not overtake it.
	f.requireNoClaim(other, "a newer build sharing files with an older held build")

	f.move(owner, running, domain.LaneMerged, nil)
	if work := f.mustClaim(other, app.ClaimInput{}); work.Ticket.Ticket.ID != older {
		t.Fatalf("claimed #%d after the running build merged, want the older #%d", work.Ticket.Ticket.ID, older)
	}
}

func TestABlockedOlderBuildDoesNotHoldNewerOnesBack(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	older, newer := f.ticket("older", api.ID), f.ticket("newer", api.ID)
	f.toReady(older)
	f.toReady(newer)
	f.toBuildWith(newer, "Edit src/core/Engine.cpp.")
	f.toBuildWith(older, "Edit src/core/Engine.cpp.")
	if err := f.store.Write(f.ctx, func(repos app.Repos) error {
		ticket, err := repos.Tickets().Get(f.ctx, older)
		if err != nil {
			return err
		}
		ticket.BlockedReason = "waiting on the owner"
		return repos.Tickets().Update(f.ctx, ticket)
	}); err != nil {
		t.Fatal(err)
	}
	if work := f.mustClaim(agent, app.ClaimInput{}); work.Ticket.Ticket.ID != newer {
		t.Fatalf("claimed #%d, want #%d past the blocked one", work.Ticket.Ticket.ID, newer)
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

func TestRepoOverlapIgnoreClaims(t *testing.T) {
	for _, tc := range []struct {
		name, pattern, extra string
		held                 bool
	}{
		{"exact", "src/messages/Catalog.cpp", "", false},
		{"glob", "src/*/Catalog.cpp", "", false},
		{"unmatched", "src/*/Other.cpp", "", true},
		{"other shared file", "src/*/Catalog.cpp", "  src/core/Engine.cpp", true},
		{"empty", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			repo := f.repo("example/app")
			patterns := []string{}
			if tc.pattern != "" {
				patterns = append(patterns, tc.pattern)
			}
			if _, err := f.repos.Restyle(f.ctx, owner, repo.ID, domain.RepoStyle{OverlapIgnore: &patterns}); err != nil {
				t.Fatal(err)
			}
			a, b := f.ticket("first", repo.ID), f.ticket("second", repo.ID)
			f.toReady(a)
			f.toReady(b)
			f.toBuildWith(b, "Edit src/messages/Catalog.cpp"+tc.extra)
			f.toBuildWith(a, "Edit src/messages/Catalog.cpp"+tc.extra)
			if w := f.mustClaim(agent, app.ClaimInput{}); w.Ticket.Ticket.ID != a {
				t.Fatal(w)
			}
			branch := "ob/first"
			if _, err := f.board.Edit(f.ctx, agent, a, domain.TicketEdit{Branch: &branch}); err != nil {
				t.Fatal(err)
			}
			if tc.held {
				f.requireNoClaim(other, "shared source")
			} else if w := f.mustClaim(other, app.ClaimInput{}); w.Ticket.Ticket.ID != b {
				t.Fatal(w)
			}
		})
	}
}

func TestHealthCheckBypassesOverlap(t *testing.T) {
	f := newFixture(t)
	repo := f.repo("example/app")
	a, b := f.ticket("first", repo.ID), f.ticket("review", repo.ID)
	f.toReady(a)
	f.toReady(b)
	f.toBuildWith(b, "Edit src/core/Engine.cpp")
	f.toBuildWith(a, "Edit src/core/Engine.cpp")
	if err := f.store.Write(f.ctx, func(repos app.Repos) error {
		ticket, err := repos.Tickets().Get(f.ctx, b)
		if err != nil {
			return err
		}
		ticket.IsHealthCheck = true
		return repos.Tickets().Update(f.ctx, ticket)
	}); err != nil {
		t.Fatal(err)
	}
	f.mustClaim(agent, app.ClaimInput{})
	branch := "ob/first"
	if _, err := f.board.Edit(f.ctx, agent, a, domain.TicketEdit{Branch: &branch}); err != nil {
		t.Fatal(err)
	}
	if w := f.mustClaim(other, app.ClaimInput{}); w.Ticket.Ticket.ID != b {
		t.Fatal(w)
	}
}

func TestOverlapIgnoreIsRepositoryLocal(t *testing.T) {
	f := newFixture(t)
	ignored := f.repo("example/ignored")
	ordinary := f.repo("example/ordinary")
	patterns := []string{"src/*/Catalog.cpp"}
	if _, err := f.repos.Restyle(f.ctx, owner, ignored.ID, domain.RepoStyle{OverlapIgnore: &patterns}); err != nil {
		t.Fatal(err)
	}
	for i, repo := range []domain.Repo{ignored, ordinary} {
		a, b := f.ticket("first", repo.ID), f.ticket("second", repo.ID)
		f.toReady(a)
		f.toReady(b)
		f.toBuildWith(b, "Edit src/messages/Catalog.cpp")
		f.toBuildWith(a, "Edit src/messages/Catalog.cpp")
		actor := domain.Actor{Kind: domain.ActorAgent, ID: int64(20 + i), Name: "worker"}
		if w := f.mustClaim(actor, app.ClaimInput{Repo: repo.FullName}); w.Ticket.Ticket.ID != a {
			t.Fatal(w)
		}
		branch := "ob/first"
		if _, err := f.board.Edit(f.ctx, actor, a, domain.TicketEdit{Branch: &branch}); err != nil {
			t.Fatal(err)
		}
		second := domain.Actor{Kind: domain.ActorAgent, ID: int64(30 + i), Name: "second"}
		if i == 0 {
			if w := f.mustClaim(second, app.ClaimInput{Repo: repo.FullName}); w.Ticket.Ticket.ID != b {
				t.Fatal(w)
			}
		} else {
			if _, ok := f.claim(second, app.ClaimInput{Repo: repo.FullName}); ok {
				t.Fatal("another repository's exclusions applied")
			}
		}
	}
}
