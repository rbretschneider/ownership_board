package domain

// WorkAction tells an agent what to do with a claimed ticket.
type WorkAction string

// The agent actions.
const (
	ActionDeploy    WorkAction = "deploy"
	ActionRefine    WorkAction = "refine"
	ActionDesign    WorkAction = "design"
	ActionImplement WorkAction = "implement"
)

// ClaimOrder prioritizes requested deployments, then started builds/designs,
// refinement and finally new design work.
var ClaimOrder = []Stage{StageDeploying, StageBuild, StageDesign, StageRefine, StageReady}

// ActionFor returns the action for a ticket claimed in stage, and the stage it
// should be in while the agent works on it.
func ActionFor(stage Stage) (WorkAction, Stage, bool) {
	switch stage {
	case StageDeploying:
		return ActionDeploy, StageDeploying, true
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
