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
	return holdsInFlightSlot(dependant) && waitsForInFlightSlot(prerequisite)
}

// InFlightStall names the two ends of a stall a new depends_on link would
// make: Holder is an autonomous ticket in design or build, Waiter an
// autonomous ticket that has not reached design, and Holder would wait on
// Waiter through the link.
type InFlightStall struct {
	Holder, Waiter int64
}

// DependencyStall reports whether adding "from depends_on to" would make an
// in-flight autonomous ticket wait, directly or through other tickets, on an
// autonomous ticket the in-flight cap holds back. edges are the existing
// depends_on edges (dependant id to the ids it depends on) and tickets holds
// every ticket the walk may reach; an id missing from it ends that branch.
// Merged tickets end a branch too, since nothing waits on them. Each walk
// visits a ticket at most once. When several pairs would stall, the lowest
// ids are named, so the result is the same on every run.
func DependencyStall(edges map[int64][]int64, tickets map[int64]Ticket, from, to int64) (InFlightStall, bool) {
	dependants := map[int64][]int64{}
	for dependant, prerequisites := range edges {
		for _, prerequisite := range prerequisites {
			dependants[prerequisite] = append(dependants[prerequisite], dependant)
		}
	}
	holder, hasHolder := lowestReachable(dependants, tickets, from, holdsInFlightSlot)
	if !hasHolder {
		return InFlightStall{}, false
	}
	waiter, hasWaiter := lowestReachable(edges, tickets, to, waitsForInFlightSlot)
	if !hasWaiter {
		return InFlightStall{}, false
	}
	return InFlightStall{Holder: holder, Waiter: waiter}, true
}

// lowestReachable walks next from start, start included, through tickets
// that are not merged, and returns the lowest id that matches.
func lowestReachable(next map[int64][]int64, tickets map[int64]Ticket, start int64, matches func(Ticket) bool) (int64, bool) {
	var lowest int64
	isFound := false
	seen := map[int64]bool{start: true}
	queue := []int64{start}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		ticket, isKnown := tickets[current]
		if !isKnown || ticket.Stage == StageDone {
			continue
		}
		if matches(ticket) && (!isFound || current < lowest) {
			lowest, isFound = current, true
		}
		for _, neighbour := range next[current] {
			if !seen[neighbour] {
				seen[neighbour] = true
				queue = append(queue, neighbour)
			}
		}
	}
	return lowest, isFound
}

// holdsInFlightSlot reports whether the ticket counts against the in-flight
// cap.
func holdsInFlightSlot(ticket Ticket) bool {
	return ticket.IsAutonomous && ticket.Stage.IsInFlight()
}

// waitsForInFlightSlot reports whether the ticket can only start through a
// claim from Ready, which the in-flight cap may hold back.
func waitsForInFlightSlot(ticket Ticket) bool {
	return ticket.IsAutonomous && ticket.Stage.Lane() == LaneTodo
}
