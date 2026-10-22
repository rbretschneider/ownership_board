package domain

// WorkAction tells an agent what to do with a claimed ticket.
type WorkAction string

// The agent actions.
const (
	ActionRefine    WorkAction = "refine"
	ActionDesign    WorkAction = "design"
	ActionImplement WorkAction = "implement"
)

// ClaimOrder lists the stages an agent claims from, most urgent first: finish
// started work, then refine (cheap, and it unblocks the owner soonest), then
// start designing what is ready.
var ClaimOrder = []Stage{StageBuild, StageDesign, StageRefine, StageReady}

// ActionFor returns the action for a ticket claimed in stage, and the stage it
// should be in while the agent works on it.
func ActionFor(stage Stage) (WorkAction, Stage, bool) {
	switch stage {
	case StageRefine:
		return ActionRefine, StageRefine, true
	case StageReady, StageDesign:
		return ActionDesign, StageDesign, true
	case StageBuild:
		return ActionImplement, StageBuild, true
	default:
		return "", stage, false
	}
}
