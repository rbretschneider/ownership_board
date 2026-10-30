package app_test

import (
	"testing"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestBoardSettingsStartAtTheDefaults(t *testing.T) {
	f := newFixture(t)
	settings, err := f.board.Settings(f.ctx)
	if err != nil || settings != domain.DefaultBoardSettings() {
		t.Fatalf("settings %+v, %v, want the defaults", settings, err)
	}
}

func TestOnlyPeopleChangeBoardSettings(t *testing.T) {
	f := newFixture(t)
	value := 3
	for _, actor := range []domain.Actor{agent, delegate} {
		_, err := f.board.UpdateSettings(f.ctx, actor, domain.SettingsEdit{AutonomousInFlightCap: &value})
		requireErrorKind(t, err, domain.KindForbidden)
	}
	f.setInFlightCap(3)
	settings, err := f.board.Settings(f.ctx)
	if err != nil || settings.AutonomousInFlightCap != 3 {
		t.Fatalf("cap %d, %v, want 3", settings.AutonomousInFlightCap, err)
	}
}

func TestOutOfRangeCapsAreRefusedAndChangeNothing(t *testing.T) {
	f := newFixture(t)
	for _, value := range []int{0, -1, 11} {
		_, err := f.board.UpdateSettings(f.ctx, owner, domain.SettingsEdit{AutonomousInFlightCap: &value})
		requireErrorKind(t, err, domain.KindInvalid)
	}
	settings, err := f.board.Settings(f.ctx)
	if err != nil || settings.AutonomousInFlightCap != domain.DefaultAutonomousInFlightCap {
		t.Fatalf("cap %d after refused changes, %v", settings.AutonomousInFlightCap, err)
	}
}
