package app_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

// threeStories is a full split into three stories in a straight build order.
func threeStories() domain.SplitDraft {
	return domain.SplitDraft{
		Reason: "17 criteria across the kernel and the UI",
		Children: []domain.SplitChildDraft{
			{Title: "Kernel", Spec: "1. The kernel cuts threads.\n\nBuild tier: normal"},
			{Title: "Port", Spec: "1. The port exposes it.\n\nBuild tier: small"},
			{Title: "UI", Spec: "1. The dialog drives it.\n\nBuild tier: normal"},
		},
	}
}

func (f *fixture) mustSplit(actor domain.Actor, id int64, draft domain.SplitDraft) app.SplitResult {
	f.t.Helper()
	result, err := f.board.Split(f.ctx, actor, id, draft)
	if err != nil {
		f.t.Fatalf("split #%d: %v", id, err)
	}
	return result
}

// autonomousInBuild files an autonomous ticket and has agent claim it into
// design and then build.
func (f *fixture) autonomousInBuild(repoID int64) int64 {
	f.t.Helper()
	id := f.mustCreateAutonomous(repoID)
	f.mustClaim(agent, app.ClaimInput{})
	if _, err := f.board.SaveDesign(f.ctx, agent, id, app.SaveTextInput{Text: "The plan.", IsSubmit: true}); err != nil {
		f.t.Fatalf("submit design: %v", err)
	}
	f.requireStage(id, domain.StageBuild)
	f.mustClaim(agent, app.ClaimInput{})
	return id
}

func TestRefinementSplitsAnOversizedRequestIntoAnEpic(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	id := f.ticket("Threading", api.ID)
	f.mustClaim(agent, app.ClaimInput{})

	result := f.mustSplit(agent, id, threeStories())

	if len(result.ChildIDs) != 3 {
		t.Fatalf("children = %v, want 3", result.ChildIDs)
	}
	parent := f.detail(id)
	if parent.Ticket.Stage != domain.StageEpic || parent.Ticket.Lease != nil {
		t.Fatalf("parent stage=%s lease=%v, want an unleased epic", parent.Ticket.Stage, parent.Ticket.Lease)
	}
	if got := len(parent.SplitInto); got != 3 {
		t.Fatalf("parent shows %d children, want 3", got)
	}
	if !slices.ContainsFunc(parent.Decisions, func(d domain.Decision) bool {
		return strings.HasPrefix(d.Summary, "Split into #") && strings.Contains(d.Rationale, "Kernel")
	}) {
		t.Fatalf("no split decision on the parent: %+v", parent.Decisions)
	}
	kernel, port, ui := result.ChildIDs[0], result.ChildIDs[1], result.ChildIDs[2]
	for _, childID := range result.ChildIDs {
		child := f.detail(childID)
		// The repo gates the spec, so each story goes to the owner.
		if child.Ticket.Stage != domain.StageSpecReview || child.Ticket.RepoID != api.ID || child.Ticket.Spec == "" {
			t.Fatalf("child #%d stage=%s repo=%d spec=%q", childID, child.Ticket.Stage, child.Ticket.RepoID, child.Ticket.Spec)
		}
		if child.SplitFrom != id {
			t.Fatalf("child #%d split_from = %d, want %d", childID, child.SplitFrom, id)
		}
	}
	if got := f.waitingOn(port); !slices.Equal(got, []int64{kernel}) {
		t.Fatalf("port waits on %v, want the kernel", got)
	}
	if got := f.waitingOn(ui); !slices.Equal(got, []int64{port}) {
		t.Fatalf("UI waits on %v, want the port", got)
	}
	f.requireNoClaim(other, "an epic and stories at the spec checkpoint")
}

func TestAnAutonomousSplitInheritsAutonomyAndTheParentsDependencies(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	id := f.autonomousInBuild(api.ID)
	before := f.ticket("Groundwork", api.ID)
	f.mustLink(agent, id, before)
	draft := threeStories()
	draft.Children[2].After, draft.Children[2].HasAfter = []int{1}, true

	result := f.mustSplit(agent, id, draft)

	f.requireStage(id, domain.StageEpic)
	for _, childID := range result.ChildIDs {
		child := f.detail(childID).Ticket
		if !child.IsAutonomous || child.Stage != domain.StageReady || child.ApprovedSpecRevision != child.SpecRevision {
			t.Fatalf("child #%d autonomous=%v stage=%s approved=%d/%d", childID, child.IsAutonomous, child.Stage, child.ApprovedSpecRevision, child.SpecRevision)
		}
	}
	kernel, port, ui := result.ChildIDs[0], result.ChildIDs[1], result.ChildIDs[2]
	if got := f.waitingOn(kernel); !slices.Equal(got, []int64{before}) {
		t.Fatalf("first story waits on %v, want what the epic waited on", got)
	}
	if got := f.waitingOn(port); !slices.Equal(got, []int64{kernel}) {
		t.Fatalf("port waits on %v", got)
	}
	if got := f.waitingOn(ui); !slices.Equal(got, []int64{kernel}) {
		t.Fatalf("UI waits on %v, want only the kernel it named", got)
	}
}

func TestANarrowingSplitKeepsTheCoreBuilding(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	id := f.autonomousInBuild(api.ID)
	revision := f.detail(id).Ticket.SpecRevision
	draft := domain.SplitDraft{
		Reason:   "the PR grew past one story",
		KeepSpec: "1. The kernel cuts threads, with its tests.",
		Children: []domain.SplitChildDraft{{Title: "UI", Spec: "1. The dialog drives it, with its tests."}},
	}

	result := f.mustSplit(agent, id, draft)

	parent := f.detail(id).Ticket
	if parent.Stage != domain.StageBuild || !parent.IsLeased(f.clock.Now()) {
		t.Fatalf("parent stage=%s leased=%v, want it still building", parent.Stage, parent.IsLeased(f.clock.Now()))
	}
	if parent.Spec != draft.KeepSpec || parent.SpecRevision != revision+1 || parent.ApprovedSpecRevision != parent.SpecRevision {
		t.Fatalf("parent spec=%q rev=%d approved=%d", parent.Spec, parent.SpecRevision, parent.ApprovedSpecRevision)
	}
	if got := f.waitingOn(result.ChildIDs[0]); !slices.Equal(got, []int64{id}) {
		t.Fatalf("the moved feature waits on %v, want the core", got)
	}
}

func TestNarrowingNeedsAnAutonomousTicketInFlight(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	id := f.ticket("Threading", api.ID)
	f.mustClaim(agent, app.ClaimInput{})
	draft := domain.SplitDraft{Reason: "too big", KeepSpec: "core", Children: []domain.SplitChildDraft{{Title: "rest", Spec: "1. rest"}}}
	_, err := f.board.Split(f.ctx, agent, id, draft)
	requireErrorKind(t, err, domain.KindConflict)
	f.requireStage(id, domain.StageRefine)
	if got := f.ticketCount(); got != 1 {
		t.Fatalf("a refused split left %d tickets, want 1", got)
	}
}

func TestWhoMaySplit(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	managed := f.autonomousInBuild(api.ID)
	held := f.ticket("held", api.ID)
	f.mustClaim(agent, app.ClaimInput{})
	_, err := f.board.Split(f.ctx, other, held, threeStories())
	requireErrorKind(t, err, domain.KindForbidden)

	// The delegate that filed an agreed ticket may break it down before anyone starts it.
	filed := f.mustCreateAutonomous(api.ID)
	f.mustSplit(delegate, filed, threeStories())
	f.requireStage(filed, domain.StageEpic)

	// The board manager may split any autonomous ticket; a person any ticket.
	f.mustSplit(manager, managed, threeStories())
	f.mustSplit(owner, held, threeStories())
	f.requireStage(held, domain.StageEpic)
}

func TestSplitRefusals(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	id := f.ticket("Threading", api.ID)
	f.mustClaim(agent, app.ClaimInput{})

	one := domain.SplitDraft{Reason: "too big", Children: threeStories().Children[:1]}
	_, err := f.board.Split(f.ctx, agent, id, one)
	requireErrorKind(t, err, domain.KindInvalid)

	noSpec := threeStories()
	noSpec.Children[1].Spec = " "
	_, err = f.board.Split(f.ctx, agent, id, noSpec)
	requireErrorKind(t, err, domain.KindInvalid)

	forward := threeStories()
	forward.Children[0].After, forward.Children[0].HasAfter = []int{2}, true
	_, err = f.board.Split(f.ctx, agent, id, forward)
	requireErrorKind(t, err, domain.KindInvalid)

	if got := f.ticketCount(); got != 1 {
		t.Fatalf("refused splits left %d tickets, want 1", got)
	}
	f.mustSplit(agent, id, threeStories())
	_, err = f.board.Split(f.ctx, owner, id, threeStories())
	requireErrorKind(t, err, domain.KindConflict)

	// Only a split makes split_from links.
	a, b := f.ticket("a", api.ID), f.ticket("b", api.ID)
	_, err = f.board.LinkTickets(f.ctx, owner, a, string(domain.RelationSplitFrom), b)
	requireErrorKind(t, err, domain.KindInvalid)
}

func TestTheEpicClosesWhenEveryChildIsDeployed(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	id := f.autonomousInBuild(api.ID)
	after := f.ticket("Docs", api.ID)
	f.mustLink(owner, after, id)
	draft := threeStories()
	draft.Children[1].After, draft.Children[1].HasAfter = []int{}, true
	draft.Children[2].After, draft.Children[2].HasAfter = []int{}, true
	result := f.mustSplit(agent, id, draft)

	// One child split again: a nested epic closes its parent in turn.
	nested := f.mustSplit(owner, result.ChildIDs[2], domain.SplitDraft{Reason: "still big", Children: []domain.SplitChildDraft{
		{Title: "UI a", Spec: "1. a"}, {Title: "UI b", Spec: "1. b"},
	}})

	deploy := append(append([]int64{}, result.ChildIDs[:2]...), nested.ChildIDs...)
	for i, childID := range deploy {
		f.move(owner, childID, domain.LaneDeployed, nil)
		isLast := i == len(deploy)-1
		if stage := f.detail(id).Ticket.Stage; (stage == domain.StageDeployed) != isLast {
			t.Fatalf("after deploying %d of %d children the epic is %s", i+1, len(deploy), stage)
		}
	}
	f.requireStage(result.ChildIDs[2], domain.StageDeployed)
	if got := f.waitingOn(after); len(got) != 0 {
		t.Fatalf("the ticket after the epic still waits on %v", got)
	}
}

func TestBriefsCarryTheStorySizeRule(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	f.ticket("Threading", api.ID)
	refine := f.mustClaim(agent, app.ClaimInput{}).Brief
	for _, want := range []string{"## Story size", "half a day", "more than about 5 acceptance criteria", "split_ticket", "never defers a test"} {
		if !strings.Contains(refine, want) {
			t.Errorf("refine brief is missing %q", want)
		}
	}
	if strings.Contains(refine, "keep_spec") {
		t.Error("refine brief offers narrowing")
	}

	id := f.mustCreateAutonomous(api.ID)
	design := f.mustClaim(other, app.ClaimInput{})
	if design.Ticket.Ticket.ID != id {
		t.Fatalf("claimed #%d, want #%d", design.Ticket.Ticket.ID, id)
	}
	for _, want := range []string{"## Story size", "Judge the size again", "keep_spec"} {
		if !strings.Contains(design.Brief, want) {
			t.Errorf("autonomous design brief is missing %q", want)
		}
	}
}

func TestTheBoardShowsSplitProgress(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	id := f.ticket("Threading", api.ID)
	f.mustClaim(agent, app.ClaimInput{})
	result := f.mustSplit(agent, id, threeStories())
	board, err := f.board.Board(f.ctx, app.RepoScope{})
	if err != nil {
		t.Fatalf("board: %v", err)
	}
	for _, summary := range board.Tickets {
		switch {
		case summary.Ticket.ID == id:
			if len(summary.SplitInto) != 3 || summary.SplitInto[0].ID != result.ChildIDs[0] || summary.SplitInto[0].Title != "Kernel" {
				t.Fatalf("epic split_into = %+v", summary.SplitInto)
			}
		case slices.Contains(result.ChildIDs, summary.Ticket.ID):
			if summary.SplitFrom != id {
				t.Fatalf("child #%d split_from = %d", summary.Ticket.ID, summary.SplitFrom)
			}
		}
	}
}
