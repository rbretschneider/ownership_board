package domain

import (
	"strconv"
	"strings"
)

// SettingAutonomousInFlightCap is the stored key of
// BoardSettings.AutonomousInFlightCap.
const SettingAutonomousInFlightCap = "autonomous_in_flight_cap"

// Bounds of the in-flight cap. Zero is not allowed: a cap that stops every
// autonomous ticket would leave the queue waiting with no visible reason.
const (
	DefaultAutonomousInFlightCap = 1
	MinAutonomousInFlightCap     = 1
	MaxAutonomousInFlightCap     = 10
)

// BoardSettings are the board-wide settings people change in Settings.
type BoardSettings struct {
	// AutonomousInFlightCap is how many autonomous tickets may be in design
	// or build at once. While that many are, agents claim no further
	// autonomous ticket from Ready; other work still flows.
	AutonomousInFlightCap int
}

// SettingsEdit lists the settings to change; nil leaves a setting as it is.
type SettingsEdit struct {
	AutonomousInFlightCap *int
}

// DefaultBoardSettings are the settings of a new board.
func DefaultBoardSettings() BoardSettings {
	return BoardSettings{AutonomousInFlightCap: DefaultAutonomousInFlightCap}
}

// Apply validates edit and returns the settings with it applied.
func (s BoardSettings) Apply(edit SettingsEdit) (BoardSettings, error) {
	next := s
	if edit.AutonomousInFlightCap != nil {
		value := *edit.AutonomousInFlightCap
		if value < MinAutonomousInFlightCap || value > MaxAutonomousInFlightCap {
			return BoardSettings{}, Invalidf("%s must be between %d and %d", SettingAutonomousInFlightCap, MinAutonomousInFlightCap, MaxAutonomousInFlightCap)
		}
		next.AutonomousInFlightCap = value
	}
	return next, nil
}

// ParseInFlightCap reads a stored cap. A value that is missing, unreadable or
// out of range gives the default, which is the most cautious setting, and
// false.
func ParseInFlightCap(raw string) (int, bool) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < MinAutonomousInFlightCap || value > MaxAutonomousInFlightCap {
		return DefaultAutonomousInFlightCap, false
	}
	return value, true
}

// DependencyStallsInFlightSlot reports whether "dependant depends_on
// prerequisite" would park an in-flight autonomous ticket behind an
// autonomous ticket that has not reached design yet. The prerequisite can
// only start through a claim from Ready, which the in-flight cap holds back
// while the dependant keeps its slot, so neither ticket could move again.
func DependencyStallsInFlightSlot(dependant, prerequisite Ticket) bool {
	return dependant.IsAutonomous && dependant.Stage.IsInFlight() &&
		prerequisite.IsAutonomous && prerequisite.Stage.Lane() == LaneTodo
}
