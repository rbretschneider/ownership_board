package domain

// WorkAction tells an agent what to do with a claimed ticket.
type WorkAction string

// The agent actions.
const (
	ActionRefine    WorkAction = "refine"
	ActionImplement WorkAction = "implement"
	ActionResume    WorkAction = "resume"
)

// ClaimOrder lists the lanes an agent claims from, most urgent first: finish
// started work, then refine (cheap, and it unblocks the human soonest), then
// build.
var ClaimOrder = []Lane{LaneInProgress, LaneNew, LaneReady}

// ActionFor returns the action for a ticket claimed from lane, and the lane it
// should sit in while the agent works on it.
func ActionFor(lane Lane) (WorkAction, Lane, bool) {
	switch lane {
	case LaneNew:
		return ActionRefine, LaneNew, true
	case LaneReady:
		return ActionImplement, LaneInProgress, true
	case LaneInProgress:
		return ActionResume, LaneInProgress, true
	default:
		return "", lane, false
	}
}
