package domain

// Lane is one of the fixed swim lanes a ticket moves through. The set is part
// of the domain, not configuration: agents and integrations rely on it never
// changing. Finer progress lives in the ticket's Stage.
type Lane string

// The lanes, in board order.
const (
	LaneTodo       Lane = "todo"
	LaneInProgress Lane = "in_progress"
	LaneMerged     Lane = "merged"
	LaneDeployed   Lane = "deployed"
)

// LaneInfo describes a lane for display and for agents.
type LaneInfo struct {
	ID          Lane   `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

var laneCatalog = []LaneInfo{
	{LaneTodo, "Todo", "Capture an idea, agree the outcome, and queue the work."},
	{LaneInProgress, "In progress", "Work is being designed, built, repaired or reviewed."},
	{LaneMerged, "Merged", "Integrated into the project; deployment follows the project’s release process."},
	{LaneDeployed, "Deployed", "Released or complete without deployment."},
}

// Lanes returns every lane in board order.
func Lanes() []LaneInfo {
	out := make([]LaneInfo, len(laneCatalog))
	copy(out, laneCatalog)
	return out
}

// ParseLane validates a lane id.
func ParseLane(raw string) (Lane, error) {
	for _, info := range laneCatalog {
		if string(info.ID) == raw {
			return info.ID, nil
		}
	}
	return "", Invalidf("unknown lane %q", raw)
}

// Label returns the display name of the lane.
func (l Lane) Label() string {
	for _, info := range laneCatalog {
		if info.ID == l {
			return info.Label
		}
	}
	return string(l)
}
