package domain

import (
	"strings"
	"time"
)

// Workflow limits.
const (
	MaxDesignLength        = 20_000
	MaxBlockedReasonLength = 1_000
	closedPRReason         = "The pull request was closed without merging. Decide what happens next: comment and unblock to rebuild, or drag the ticket elsewhere."
)

// Checkpoint is a point where the board waits for the owner's approval.
type Checkpoint string

// The checkpoints.
const (
	CheckpointSpec   Checkpoint = "spec"
	CheckpointDesign Checkpoint = "design"
)

// ParseCheckpoint validates a checkpoint name.
func ParseCheckpoint(raw string) (Checkpoint, error) {
	switch Checkpoint(raw) {
	case CheckpointSpec, CheckpointDesign:
		return Checkpoint(raw), nil
	default:
		return "", Invalidf("checkpoint must be spec or design")
	}
}

// Gates says which checkpoints apply to a ticket.
type Gates struct {
	Spec   bool
	Design bool
}

// GatesFor returns the repo's checkpoints. A ticket with no repo has both, so
// a missing repo can never be a way around them.
func GatesFor(repo *Repo) Gates {
	if repo == nil {
		return Gates{Spec: true, Design: true}
	}
	return Gates{Spec: repo.RequireSpecApproval, Design: repo.RequireDesignApproval}
}

// SaveSpec stores the agent's spec, in refine only. With submit it hands the
// ticket on: to the owner when the spec gate applies, otherwise to Ready.
func (t *Ticket) SaveSpec(spec string, isSubmit bool, gates Gates, openQuestions int, now time.Time) (Stage, error) {
	if t.Stage != StageRefine {
		return "", Conflictf("the spec can only be changed while ticket %d is being refined", t.ID)
	}
	if err := t.reviseSpec(spec, now); err != nil || !isSubmit {
		return "", err
	}
	if err := t.requireHandOff(openQuestions); err != nil {
		return "", err
	}
	if t.Spec == "" || t.RepoID == 0 {
		return "", Invalidf("ticket %d needs a spec and a repo before it can be submitted", t.ID)
	}
	if gates.Spec {
		return StageSpecReview, nil
	}
	return StageReady, nil
}

// EditSpecAsOwner lets a person rewrite the spec in any stage. It counts as a
// new revision, so an approval of the old text no longer covers it.
func (t *Ticket) EditSpecAsOwner(spec string, now time.Time) error {
	return t.reviseSpec(spec, now)
}

// SaveDesign stores the agent's design, in design only. With submit it hands
// the ticket on: to the owner when the design gate applies, otherwise to Build.
func (t *Ticket) SaveDesign(design string, isSubmit bool, gates Gates, openQuestions int, now time.Time) (Stage, error) {
	if t.Stage != StageDesign {
		return "", Conflictf("the design can only be changed while ticket %d is being designed", t.ID)
	}
	if err := t.reviseDesign(design, now); err != nil || !isSubmit {
		return "", err
	}
	if err := t.requireHandOff(openQuestions); err != nil {
		return "", err
	}
	if t.Design == "" {
		return "", Invalidf("ticket %d needs a design before it can be submitted", t.ID)
	}
	if gates.Design {
		return StageDesignReview, nil
	}
	return StageBuild, nil
}

// Approve records the owner's approval of exactly the revision they saw and
// returns the stage that follows. A different revision means the text changed
// after it was shown, so nothing is approved.
func (t *Ticket) Approve(checkpoint Checkpoint, revision int) (Stage, error) {
	switch checkpoint {
	case CheckpointSpec:
		if err := t.requireReview(StageSpecReview, revision, t.SpecRevision); err != nil {
			return "", err
		}
		t.ApprovedSpecRevision = revision
		return StageReady, nil
	default:
		if err := t.requireReview(StageDesignReview, revision, t.DesignRevision); err != nil {
			return "", err
		}
		t.ApprovedDesignRevision = revision
		return StageBuild, nil
	}
}

// RequestChanges sends a reviewed revision back to the agent.
func (t *Ticket) RequestChanges(checkpoint Checkpoint, revision int) (Stage, error) {
	if checkpoint == CheckpointSpec {
		return StageRefine, t.requireReview(StageSpecReview, revision, t.SpecRevision)
	}
	return StageDesign, t.requireReview(StageDesignReview, revision, t.DesignRevision)
}

// Block stops agent work until a person decides. It drops the lease.
func (t *Ticket) Block(reason string, now time.Time) error {
	if !t.Stage.IsAgentStage() {
		return Conflictf("ticket %d is not with an agent", t.ID)
	}
	text, err := RequireText("reason", singleLine(reason), MaxBlockedReasonLength)
	if err != nil {
		return err
	}
	t.BlockedReason, t.Lease, t.UpdatedAt = text, nil, now
	return nil
}

// Unblock lets agents pick the ticket up again.
func (t *Ticket) Unblock(now time.Time) error {
	if !t.IsBlocked() {
		return Conflictf("ticket %d is not blocked", t.ID)
	}
	t.BlockedReason, t.UpdatedAt = "", now
	return nil
}

// HandOffForQuestions drops the lease when an agent asks: the ticket now waits
// for the owner and must not stay held.
func (t *Ticket) HandOffForQuestions(now time.Time) error {
	if !t.Stage.IsAgentStage() {
		return Conflictf("questions can only be asked while an agent works on ticket %d", t.ID)
	}
	t.Lease, t.UpdatedAt = nil, now
	return nil
}

// SubmitForReview is the agent's only lane move: a finished build with an open
// pull request goes to Review.
func (t *Ticket) SubmitForReview(openQuestions int) (Stage, error) {
	if t.Stage != StageBuild {
		return "", Conflictf("only a ticket being built can go to review")
	}
	if err := t.requireHandOff(openQuestions); err != nil {
		return "", err
	}
	if t.PRURL == "" {
		return "", Invalidf("set pr_url before moving ticket %d to review", t.ID)
	}
	return StagePR, nil
}

// PullRequestClosed sends a reviewed ticket back blocked, so a person decides
// whether to rebuild, rethink or drop it.
func (t *Ticket) PullRequestClosed(rank float64, now time.Time) {
	t.setStage(StageBuild, rank, now)
	t.BlockedReason = closedPRReason
}

// DropStage is the stage a person's drop into lane lands in: the first stage of
// that lane whose checkpoints are met by approved revisions.
func (t *Ticket) DropStage(lane Lane, gates Gates) Stage {
	switch lane {
	case LaneTodo:
		if t.isSpecCleared(gates) {
			return StageReady
		}
		return StageRefine
	case LaneInProgress:
		if t.isDesignCleared(gates) {
			return StageBuild
		}
		return StageDesign
	case LaneReview:
		return StagePR
	default:
		return StageDone
	}
}

// MoveToStage applies a transition the caller has checked.
func (t *Ticket) MoveToStage(stage Stage, rank float64, now time.Time) bool {
	return t.setStage(stage, rank, now)
}

// ClearBlockForDrop clears a block when a person moves the ticket to another
// lane.
func (t *Ticket) ClearBlockForDrop() {
	t.BlockedReason = ""
}

// AgentMayEdit limits which fields an agent may change in each stage: what the
// ticket is (title, body, kind, repo) only while refining, and the delivery
// fields (branch, pr_url) only while building.
func (t *Ticket) AgentMayEdit(fields []string) error {
	for _, field := range fields {
		isRefineField := field == "title" || field == "body" || field == "kind" || field == "repo"
		isBuildField := field == "branch" || field == "pr_url"
		if (isRefineField && t.Stage != StageRefine) || (isBuildField && t.Stage != StageBuild) {
			return Forbiddenf("an agent cannot change %s while ticket %d is in %s", field, t.ID, t.Stage)
		}
	}
	return nil
}

// EditDesignAsOwner lets a person rewrite the design in any stage, as a new
// revision.
func (t *Ticket) EditDesignAsOwner(design string, now time.Time) error {
	return t.reviseDesign(design, now)
}

func (t *Ticket) reviseDesign(design string, now time.Time) error {
	value, err := limitDocument("design", design, MaxDesignLength)
	if err != nil {
		return err
	}
	if value != t.Design {
		t.Design, t.DesignRevision = value, t.DesignRevision+1
	}
	t.UpdatedAt = now
	return nil
}

func (t *Ticket) reviseSpec(spec string, now time.Time) error {
	value, err := limitDocument("spec", spec, MaxSpecLength)
	if err != nil {
		return err
	}
	if value != t.Spec {
		t.Spec, t.SpecRevision = value, t.SpecRevision+1
	}
	t.UpdatedAt = now
	return nil
}

func (t *Ticket) requireHandOff(openQuestions int) error {
	if openQuestions > 0 {
		return Conflictf("ticket %d still has %d open questions", t.ID, openQuestions)
	}
	if t.IsBlocked() {
		return Conflictf("ticket %d is blocked", t.ID)
	}
	return nil
}

func (t *Ticket) requireReview(stage Stage, revision, current int) error {
	if t.Stage != stage {
		return Conflictf("ticket %d is not waiting for this approval any more", t.ID)
	}
	if revision != current {
		return Conflictf("ticket %d changed since it was shown (revision %d, now %d); reload and review again", t.ID, revision, current)
	}
	return nil
}

func (t *Ticket) isSpecCleared(gates Gates) bool {
	if gates.Spec {
		return t.SpecRevision > 0 && t.ApprovedSpecRevision == t.SpecRevision
	}
	return t.Spec != ""
}

func (t *Ticket) isDesignCleared(gates Gates) bool {
	if gates.Design {
		return t.DesignRevision > 0 && t.ApprovedDesignRevision == t.DesignRevision
	}
	return t.Design != ""
}

// maxDocumentBytesPerRune bounds a spec or design in bytes as well as
// characters, so the two together always fit in an agent's brief whole.
const maxDocumentBytesPerRune = 2

func limitDocument(field, text string, maxRunes int) (string, error) {
	value, err := LimitText(field, strings.TrimSpace(text), maxRunes)
	if err == nil && len(value) > maxRunes*maxDocumentBytesPerRune {
		return "", Invalidf("%s must be at most %d bytes", field, maxRunes*maxDocumentBytesPerRune)
	}
	return value, err
}
