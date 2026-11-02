package domain

// Stage is where a ticket is within its lane. Each stage belongs to exactly
// one lane, so a ticket's lane can always be derived from its stage.
type Stage string

// The stages, in workflow order.
const (
	StageRefine       Stage = "refine"
	StageSpecReview   Stage = "spec_review"
	StageReady        Stage = "ready"
	StageDesign       Stage = "design"
	StageDesignReview Stage = "design_review"
	StageBuild        Stage = "build"
	StagePR           Stage = "pr"
	StageDone         Stage = "done"
)

// StageInfo describes a stage for display and for agents.
type StageInfo struct {
	ID    Stage  `json:"id"`
	Lane  Lane   `json:"lane"`
	Label string `json:"label"`
	// IsOwnerReview marks stages where the board waits for a person.
	IsOwnerReview bool `json:"is_owner_review"`
}

var stageCatalog = []StageInfo{
	{StageRefine, LaneTodo, "Shaping the idea", false},
	{StageSpecReview, LaneTodo, "Approve the plan", true},
	{StageReady, LaneTodo, "Ready to start", false},
	{StageDesign, LaneInProgress, "Planning delivery", false},
	{StageDesignReview, LaneInProgress, "Approve the approach", true},
	{StageBuild, LaneInProgress, "Making it happen", false},
	{StagePR, LaneReview, "Delivery checks", true},
	{StageDone, LaneMerged, "Delivered", false},
}

// Stages returns every stage in workflow order.
func Stages() []StageInfo {
	out := make([]StageInfo, len(stageCatalog))
	copy(out, stageCatalog)
	return out
}

// ParseStage validates a stage id.
func ParseStage(raw string) (Stage, error) {
	if _, isKnown := stageInfo(Stage(raw)); isKnown {
		return Stage(raw), nil
	}
	return "", Invalidf("unknown stage %q", raw)
}

// Lane is the lane the stage belongs to.
func (s Stage) Lane() Lane {
	info, _ := stageInfo(s)
	return info.Lane
}

// Label is the stage's display name.
func (s Stage) Label() string {
	info, _ := stageInfo(s)
	return info.Label
}

// IsOwnerReview reports whether the board waits for a person at this stage.
func (s Stage) IsOwnerReview() bool {
	info, _ := stageInfo(s)
	return info.IsOwnerReview
}

// IsAgentStage reports whether agents work in this stage.
func (s Stage) IsAgentStage() bool {
	return s == StageRefine || s == StageReady || s == StageDesign || s == StageBuild
}

// IsInFlight reports whether an autonomous ticket in this stage takes one of
// the in-flight slots.
func (s Stage) IsInFlight() bool {
	return s == StageDesign || s == StageBuild
}

// MayTurnAutonomous reports whether a person may make a ticket in this stage
// autonomous: its spec is past review and it is not parked at a checkpoint,
// so turning the flag on never lets an unseen spec or design through.
func (s Stage) MayTurnAutonomous() bool {
	return s == StageReady || s == StageDesign || s == StageBuild
}

func stageInfo(s Stage) (StageInfo, bool) {
	for _, info := range stageCatalog {
		if info.ID == s {
			return info, true
		}
	}
	return StageInfo{}, false
}
