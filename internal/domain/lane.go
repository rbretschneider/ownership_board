package domain

// Lane is one of the fixed swim lanes a ticket moves through. The set is part
// of the domain, not configuration: agents and integrations rely on it never
// changing.
type Lane string

// The lanes, in board order.
const (
	LaneNew        Lane = "new"
	LaneAnswers    Lane = "answers"
	LaneReady      Lane = "ready"
	LaneInProgress Lane = "in_progress"
	LaneReview     Lane = "review"
	LaneDone       Lane = "done"
)

// LaneInfo describes a lane for display and for agents.
type LaneInfo struct {
	ID          Lane   `json:"id"`
	Label       string `json:"label"`
	Owner       Owner  `json:"owner"`
	Description string `json:"description"`
}

var laneCatalog = []LaneInfo{
	{LaneNew, "New", OwnerAgent, "Created and waiting for the agent to refine it."},
	{LaneAnswers, "Pending Answers", OwnerHuman, "The agent asked questions. Nothing moves until they are answered."},
	{LaneReady, "Ready", OwnerAgent, "Refined and unambiguous. The agent's work queue."},
	{LaneInProgress, "In Progress", OwnerAgent, "Claimed by an agent and being built."},
	{LaneReview, "PR / Review", OwnerHuman, "Code complete with a PR open, waiting for human review."},
	{LaneDone, "Done", OwnerNobody, "Merged and deployed."},
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

// Owner returns who holds the ball while a ticket sits in this lane.
func (l Lane) Owner() Owner {
	for _, info := range laneCatalog {
		if info.ID == l {
			return info.Owner
		}
	}
	return OwnerNobody
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
