package domain_test

import (
	"testing"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestInFlightCapDefaultsToOne(t *testing.T) {
	if got := domain.DefaultBoardSettings().AutonomousInFlightCap; got != 1 {
		t.Fatalf("default cap %d, want 1", got)
	}
}

func TestInFlightCapMustBeBetweenOneAndTen(t *testing.T) {
	settings := domain.DefaultBoardSettings()
	for _, bad := range []int{0, -1, 11} {
		if _, err := settings.Apply(domain.SettingsEdit{AutonomousInFlightCap: &bad}); domain.KindOf(err) != domain.KindInvalid {
			t.Errorf("cap %d: %v, want invalid", bad, err)
		}
	}
	for _, good := range []int{1, 2, 10} {
		next, err := settings.Apply(domain.SettingsEdit{AutonomousInFlightCap: &good})
		if err != nil || next.AutonomousInFlightCap != good {
			t.Errorf("cap %d: got %d, %v", good, next.AutonomousInFlightCap, err)
		}
	}
	unchanged, err := settings.Apply(domain.SettingsEdit{})
	if err != nil || unchanged != settings {
		t.Fatalf("empty edit changed the settings: %+v, %v", unchanged, err)
	}
}

func TestStoredInFlightCapFallsBackToTheDefault(t *testing.T) {
	cases := map[string]struct {
		want    int
		isValid bool
	}{
		"3": {3, true}, "": {1, false}, "x": {1, false}, "0": {1, false}, "11": {1, false}, "99999999999999999999": {1, false},
	}
	for raw, tc := range cases {
		got, isValid := domain.ParseInFlightCap(raw)
		if got != tc.want || isValid != tc.isValid {
			t.Errorf("ParseInFlightCap(%q) = %d, %v, want %d, %v", raw, got, isValid, tc.want, tc.isValid)
		}
	}
}

func TestDependencyStallsInFlightSlot(t *testing.T) {
	ticket := func(stage domain.Stage, isAutonomous bool) domain.Ticket {
		return domain.Ticket{Stage: stage, Lane: stage.Lane(), IsAutonomous: isAutonomous}
	}
	cases := map[string]struct {
		dependant, prerequisite domain.Ticket
		want                    bool
	}{
		"in design on ready":           {ticket(domain.StageDesign, true), ticket(domain.StageReady, true), true},
		"in build on refine":           {ticket(domain.StageBuild, true), ticket(domain.StageRefine, true), true},
		"in design on spec review":     {ticket(domain.StageDesign, true), ticket(domain.StageSpecReview, true), true},
		"in design on in flight":       {ticket(domain.StageDesign, true), ticket(domain.StageBuild, true), false},
		"in design on open pr":         {ticket(domain.StageDesign, true), ticket(domain.StagePR, true), false},
		"in design on normal work":     {ticket(domain.StageDesign, true), ticket(domain.StageReady, false), false},
		"ready on ready":               {ticket(domain.StageReady, true), ticket(domain.StageReady, true), false},
		"normal in design on ready":    {ticket(domain.StageDesign, false), ticket(domain.StageReady, true), false},
		"design review is not counted": {ticket(domain.StageDesignReview, true), ticket(domain.StageReady, true), false},
	}
	for name, c := range cases {
		if got := domain.DependencyStallsInFlightSlot(c.dependant, c.prerequisite); got != c.want {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
}

func TestDependencyStallFollowsTheWholeChain(t *testing.T) {
	ticket := func(id int64, stage domain.Stage, isAutonomous bool) domain.Ticket {
		return domain.Ticket{ID: id, Stage: stage, Lane: stage.Lane(), IsAutonomous: isAutonomous}
	}
	tickets := map[int64]domain.Ticket{
		1: ticket(1, domain.StageBuild, true),   // holds a slot
		2: ticket(2, domain.StageRefine, false), // ordinary
		3: ticket(3, domain.StageReady, true),   // held back by the cap
		4: ticket(4, domain.StageDone, false),   // merged
		5: ticket(5, domain.StageReady, false),  // ordinary
		6: ticket(6, domain.StageDesign, true),  // holds a slot
	}
	cases := map[string]struct {
		edges    map[int64][]int64
		from, to int64
		want     domain.InFlightStall
		isStall  bool
	}{
		"direct":                        {nil, 1, 3, domain.InFlightStall{Holder: 1, Waiter: 3}, true},
		"holder link added last":        {map[int64][]int64{2: {3}}, 1, 2, domain.InFlightStall{Holder: 1, Waiter: 3}, true},
		"waiter link added last":        {map[int64][]int64{1: {2}}, 2, 3, domain.InFlightStall{Holder: 1, Waiter: 3}, true},
		"longer chain":                  {map[int64][]int64{1: {5}, 2: {3}}, 5, 2, domain.InFlightStall{Holder: 1, Waiter: 3}, true},
		"lowest holder is named":        {map[int64][]int64{6: {5}, 1: {5}}, 5, 3, domain.InFlightStall{Holder: 1, Waiter: 3}, true},
		"ordinary chain":                {map[int64][]int64{1: {2}}, 2, 5, domain.InFlightStall{}, false},
		"no holder upstream":            {map[int64][]int64{5: {2}}, 2, 3, domain.InFlightStall{}, false},
		"merged ticket ends the chain":  {map[int64][]int64{1: {4}}, 4, 3, domain.InFlightStall{}, false},
		"merged prerequisite":           {map[int64][]int64{4: {3}}, 1, 4, domain.InFlightStall{}, false},
		"existing cycle is walked once": {map[int64][]int64{2: {5}, 5: {2}}, 1, 2, domain.InFlightStall{}, false},
		"unknown ticket ends the chain": {map[int64][]int64{2: {99}, 99: {3}}, 1, 2, domain.InFlightStall{}, false},
	}
	for name, c := range cases {
		got, isStall := domain.DependencyStall(c.edges, tickets, c.from, c.to)
		if got != c.want || isStall != c.isStall {
			t.Errorf("%s: %+v, %v, want %+v, %v", name, got, isStall, c.want, c.isStall)
		}
	}
}
