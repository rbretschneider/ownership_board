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
	{StageRefine, LaneTodo, "Refining", false},
	{StageSpecReview, LaneTodo, "Spec ready", true},
	{StageReady, LaneTodo, "Ready to build", false},
	{StageDesign, LaneInProgress, "Designing", false},
	{StageDesignReview, LaneInProgress, "Design ready", true},
	{StageBuild, LaneInProgress, "Building", false},
	{StagePR, LaneReview, "PR open", true},
	{StageDone, LaneMerged, "Merged", false},
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

func stageInfo(s Stage) (StageInfo, bool) {
	for _, info := range stageCatalog {
		if info.ID == s {
			return info, true
		}
	}
	return StageInfo{}, false
}
