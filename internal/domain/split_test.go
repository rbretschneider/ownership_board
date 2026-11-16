package domain

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func splitDraft(children ...SplitChildDraft) SplitDraft {
	return SplitDraft{Reason: "too big for one story", Children: children}
}

func storyDraft(title string) SplitChildDraft {
	return SplitChildDraft{Title: title, Spec: "1. " + title}
}

func TestParseSplitDefaultsToAStraightBuildOrder(t *testing.T) {
	plan, err := ParseSplit(splitDraft(storyDraft("a"), storyDraft("b"), storyDraft("c")))
	if err != nil {
		t.Fatalf("ParseSplit: %v", err)
	}
	want := [][]int{nil, {0}, {1}}
	for i, child := range plan.Children {
		if !slices.Equal(child.After, want[i]) {
			t.Errorf("child %d waits for %v, want %v", i+1, child.After, want[i])
		}
	}
}

func TestParseSplitHonoursExplicitOrder(t *testing.T) {
	parallel := storyDraft("b")
	parallel.HasAfter = true
	joined := storyDraft("c")
	joined.After, joined.HasAfter = []int{1, 2, 1}, true
	plan, err := ParseSplit(splitDraft(storyDraft("a"), parallel, joined))
	if err != nil {
		t.Fatalf("ParseSplit: %v", err)
	}
	if len(plan.Children[1].After) != 0 || !slices.Equal(plan.Children[2].After, []int{0, 1}) {
		t.Fatalf("after = %v / %v", plan.Children[1].After, plan.Children[2].After)
	}
}

func TestParseSplitRefusals(t *testing.T) {
	forward := storyDraft("a")
	forward.After, forward.HasAfter = []int{2}, true
	itself := storyDraft("b")
	itself.After, itself.HasAfter = []int{2}, true
	tooMany := make([]SplitChildDraft, MaxSplitChildren+1)
	for i := range tooMany {
		tooMany[i] = storyDraft("x")
	}
	cases := map[string]SplitDraft{
		"no reason":             {Children: []SplitChildDraft{storyDraft("a"), storyDraft("b")}},
		"one child":             splitDraft(storyDraft("a")),
		"too many":              splitDraft(tooMany...),
		"no spec":               splitDraft(storyDraft("a"), SplitChildDraft{Title: "b"}),
		"no criteria":           splitDraft(storyDraft("a"), SplitChildDraft{Title: "b", Spec: "Build the UI somehow."}),
		"core without criteria": {Reason: "r", KeepSpec: "the core", Children: []SplitChildDraft{storyDraft("a")}},
		"waits forward":         splitDraft(forward, storyDraft("b")),
		"waits for itself":      splitDraft(storyDraft("a"), itself),
	}
	for name, draft := range cases {
		if _, err := ParseSplit(draft); KindOf(err) != KindInvalid {
			t.Errorf("%s: err = %v, want invalid", name, err)
		}
	}
	narrow := splitDraft(storyDraft("a"))
	narrow.KeepSpec = "1. the core"
	if _, err := ParseSplit(narrow); err != nil {
		t.Errorf("a narrowing split with one child: %v", err)
	}
}

func TestRequireSplittable(t *testing.T) {
	full, _ := ParseSplit(splitDraft(storyDraft("a"), storyDraft("b")))
	narrow, _ := ParseSplit(SplitDraft{Reason: "r", KeepSpec: "1. core", Children: []SplitChildDraft{storyDraft("a")}})
	for _, stage := range []Stage{StageRefine, StageReady, StageDesign, StageBuild} {
		ticket := Ticket{ID: 1, RepoID: 1, Stage: stage}
		if err := ticket.RequireSplittable(full); err != nil {
			t.Errorf("%s: %v", stage, err)
		}
	}
	for _, stage := range []Stage{StageSpecReview, StageDesignReview, StagePR, StageEpic, StageDone, StageDeploying, StageDeployed} {
		ticket := Ticket{ID: 1, RepoID: 1, Stage: stage}
		if err := ticket.RequireSplittable(full); KindOf(err) != KindConflict {
			t.Errorf("%s: err = %v, want conflict", stage, err)
		}
	}
	ownerBlocked := Ticket{ID: 1, RepoID: 1, Stage: StageBuild, BlockedReason: OwnerBlockPrefix + " buy a disk"}
	if err := ownerBlocked.RequireSplittable(full); KindOf(err) != KindConflict {
		t.Errorf("owner block: err = %v", err)
	}
	noRepo := Ticket{ID: 1, Stage: StageRefine}
	if err := noRepo.RequireSplittable(full); KindOf(err) != KindInvalid {
		t.Errorf("no repo: err = %v", err)
	}
	if err := (&Ticket{ID: 1, RepoID: 1, Stage: StageBuild}).RequireSplittable(narrow); KindOf(err) != KindConflict {
		t.Errorf("narrowing a supervised ticket: err = %v", err)
	}
	if err := (&Ticket{ID: 1, RepoID: 1, Stage: StageReady, IsAutonomous: true}).RequireSplittable(narrow); KindOf(err) != KindConflict {
		t.Errorf("narrowing an unstarted ticket: err = %v", err)
	}
	if err := (&Ticket{ID: 1, RepoID: 1, Stage: StageBuild, IsAutonomous: true}).RequireSplittable(narrow); err != nil {
		t.Errorf("narrowing an autonomous build: %v", err)
	}
}

func TestNewSplitChildInheritsFromItsParent(t *testing.T) {
	now := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	actor := Actor{Kind: ActorAgent, ID: 1, Name: "loop"}
	parent := Ticket{ID: 9, RepoID: 4, Kind: KindBug, IsAutonomous: true}
	child, err := NewSplitChild(storyDraft("a"), parent, Gates{Spec: true}, actor, now)
	if err != nil {
		t.Fatalf("NewSplitChild: %v", err)
	}
	if child.RepoID != 4 || child.Kind != KindBug || !child.IsAutonomous || child.Stage != StageReady || child.Lane != LaneTodo ||
		child.ApprovedSpecRevision != child.SpecRevision || child.SpecRevision != 1 || child.CreatedBy != "loop" {
		t.Fatalf("autonomous child = %+v", child)
	}
	parent.IsAutonomous = false
	gated, _ := NewSplitChild(storyDraft("a"), parent, Gates{Spec: true}, actor, now)
	open, _ := NewSplitChild(storyDraft("a"), parent, Gates{}, actor, now)
	if gated.IsAutonomous || gated.Stage != StageSpecReview || gated.ApprovedSpecRevision != 0 || open.Stage != StageReady {
		t.Fatalf("gated=%s/%d open=%s", gated.Stage, gated.ApprovedSpecRevision, open.Stage)
	}
}

func TestEpicIsFinished(t *testing.T) {
	if EpicIsFinished(nil) {
		t.Error("an epic with no children finished")
	}
	children := []SplitChild{{ID: 1, Stage: StageDeployed}, {ID: 2, Stage: StageDone}}
	if EpicIsFinished(children) {
		t.Error("an epic with a merged but undeployed child finished")
	}
	children[1].Stage = StageDeployed
	if !EpicIsFinished(children) {
		t.Error("an epic with every child deployed did not finish")
	}
}

func TestAnEpicIsNobodysWorkAndRaisesNoManagerFlag(t *testing.T) {
	now := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	epic := Ticket{ID: 1, IsAutonomous: true, Stage: StageEpic, UpdatedAt: now.Add(-48 * time.Hour)}
	if epic.IsClaimable(0, now) || epic.Stage.IsInFlight() || epic.Owner(0) != OwnerNobody {
		t.Fatalf("epic claimable=%v in-flight=%v owner=%s", epic.IsClaimable(0, now), epic.Stage.IsInFlight(), epic.Owner(0))
	}
	if StageEpic.Lane() != LaneInProgress {
		t.Fatalf("epic lane = %s", StageEpic.Lane())
	}
	if flags := ManagerFlags([]ManagerTicketFacts{{Ticket: epic}}, DefaultBoardSettings(), now); len(flags) != 0 {
		t.Fatalf("epic flags = %+v", flags)
	}
}

func TestDecisionFromSplitNamesEveryChild(t *testing.T) {
	plan, _ := ParseSplit(splitDraft(storyDraft("Kernel"), storyDraft("UI")))
	decision := DecisionFromSplit(Ticket{ID: 5}, plan, []int64{6, 7}, Actor{Kind: ActorAgent, Name: "loop"}, time.Time{})
	if !strings.HasPrefix(decision.Summary, "Split into #6, #7: ") || !strings.Contains(decision.Rationale, "- #7 UI") || decision.Weight != WeightAgent {
		t.Fatalf("decision = %+v", decision)
	}
	byOwner := DecisionFromSplit(Ticket{ID: 5}, plan, []int64{6, 7}, Actor{Kind: ActorHuman, Name: "o"}, time.Time{})
	if byOwner.Weight != WeightBinding {
		t.Fatalf("owner split weight = %s", byOwner.Weight)
	}
}
