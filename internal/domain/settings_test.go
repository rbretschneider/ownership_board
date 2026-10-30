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
