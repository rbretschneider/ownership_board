package domain

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

// A split turns one ticket that is too big for a single half-day story into
// smaller child stories, each with its own spec, acceptance criteria and
// tests. A full split parks the parent as an epic that closes on its own when
// every child is deployed. A narrowing split keeps the core of an autonomous
// ticket in the parent and moves whole features, with their criteria and
// tests, into children that wait for it.

// Split limits.
const (
	MaxSplitChildren       = 12
	MaxSplitReasonLength   = 1_000
	minFullSplitChildren   = 2
	minNarrowSplitChildren = 1
)

// SplitChildDraft is one story a split creates.
type SplitChildDraft struct {
	Title string
	// Kind defaults to the parent's kind.
	Kind string
	Body string
	Spec string
	// After lists earlier children (1-based) this one waits for. When
	// HasAfter is false the child waits for the one before it, so the
	// default is a straight build order.
	After    []int
	HasAfter bool
}

// SplitDraft is a request to split a ticket.
type SplitDraft struct {
	Reason string
	// KeepSpec, when set, narrows the parent to this spec instead of parking
	// it as an epic; the children then wait for the parent.
	KeepSpec string
	Children []SplitChildDraft
}

// IsNarrowing reports whether the parent keeps building a narrowed spec.
func (d SplitDraft) IsNarrowing() bool {
	return strings.TrimSpace(d.KeepSpec) != ""
}

// SplitChildPlan is a validated child: its draft and the earlier children
// (0-based) it waits for.
type SplitChildPlan struct {
	Draft SplitChildDraft
	After []int
}

// SplitPlan is a validated split.
type SplitPlan struct {
	Reason   string
	KeepSpec string
	Children []SplitChildPlan
}

// IsNarrowing reports whether the parent keeps building a narrowed spec.
func (p SplitPlan) IsNarrowing() bool {
	return p.KeepSpec != ""
}

// ParseSplit validates a split request. Children may only wait for earlier
// children, so the order given is always a valid build order and no cycle
// can form among them.
func ParseSplit(draft SplitDraft) (SplitPlan, error) {
	reason, err := RequireText("reason", singleLine(draft.Reason), MaxSplitReasonLength)
	if err != nil {
		return SplitPlan{}, err
	}
	keep, err := limitDocument("keep_spec", draft.KeepSpec, MaxSpecLength)
	if err != nil {
		return SplitPlan{}, err
	}
	minimum := minFullSplitChildren
	if keep != "" {
		minimum = minNarrowSplitChildren
		if !hasNumberedCriteria(keep) {
			return SplitPlan{}, Invalidf("keep_spec must keep the numbered acceptance criteria, with their tests, of the code that stays")
		}
	}
	if len(draft.Children) < minimum || len(draft.Children) > MaxSplitChildren {
		return SplitPlan{}, Invalidf("a split needs %d to %d children", minimum, MaxSplitChildren)
	}
	plan := SplitPlan{Reason: reason, KeepSpec: keep}
	for i, child := range draft.Children {
		parsed, err := parseSplitChild(i, child)
		if err != nil {
			return SplitPlan{}, err
		}
		plan.Children = append(plan.Children, parsed)
	}
	return plan, nil
}

// numberedItem matches a numbered list line, "1. " or "1) ".
var numberedItem = regexp.MustCompile(`(?m)^\s*\d+[.)]\s+\S`)

// hasNumberedCriteria reports whether a spec lists numbered acceptance
// criteria. A split moves features with their criteria; a spec without any
// would leave nothing for the story's tests to prove.
func hasNumberedCriteria(spec string) bool {
	return numberedItem.MatchString(spec)
}

func parseSplitChild(index int, child SplitChildDraft) (SplitChildPlan, error) {
	position := index + 1
	if !hasNumberedCriteria(child.Spec) {
		return SplitChildPlan{}, Invalidf("child %d needs its own spec with numbered acceptance criteria and their tests", position)
	}
	if !child.HasAfter {
		if index == 0 {
			return SplitChildPlan{Draft: child}, nil
		}
		return SplitChildPlan{Draft: child, After: []int{index - 1}}, nil
	}
	var after []int
	for _, earlier := range child.After {
		if earlier < 1 || earlier >= position {
			return SplitChildPlan{}, Invalidf("child %d can only wait for earlier children (1 to %d)", position, position-1)
		}
		if !slices.Contains(after, earlier-1) {
			after = append(after, earlier-1)
		}
	}
	return SplitChildPlan{Draft: child, After: after}, nil
}

// RequireSplittable checks that the ticket is in a stage a split can start
// from: still being refined, ready, designed or built. A ticket at a
// checkpoint, in review or merged has gone too far, and a block only the
// owner can lift is theirs to resolve first. Narrowing keeps the parent
// working, which needs an autonomous ticket in design or build: the owner
// never re-approves the narrowed spec, so only their yes to run without
// stopping covers it.
func (t *Ticket) RequireSplittable(plan SplitPlan) error {
	if !t.Stage.IsSplittable() {
		return Conflictf("ticket %d is in %s; only a ticket being refined, ready, designed or built can be split", t.ID, t.Stage)
	}
	if t.IsOwnerBlock() {
		return Conflictf("ticket %d waits on the owner; a split cannot clear that", t.ID)
	}
	if t.RepoID == 0 {
		return Invalidf("ticket %d needs a repo before it can be split", t.ID)
	}
	if plan.IsNarrowing() && (!t.IsAutonomous || !t.Stage.IsInFlight()) {
		return Conflictf("keep_spec narrows an autonomous ticket in design or build; split ticket %d fully instead", t.ID)
	}
	return nil
}

// IsSplittable reports whether a ticket in this stage may be split.
func (s Stage) IsSplittable() bool {
	return s == StageRefine || s == StageReady || s == StageDesign || s == StageBuild
}

// Narrow replaces an autonomous ticket's spec with the core it keeps after a
// split. The narrowed revision counts as approved: the owner's yes to run
// without stopping covers it, as it covers the children the split files.
func (t *Ticket) Narrow(spec string, now time.Time) error {
	if err := t.reviseSpec(spec, now); err != nil {
		return err
	}
	t.ApprovedSpecRevision = t.SpecRevision
	t.BlockedReason = ""
	return nil
}

// NewSplitChild returns an unsaved child story of parent. It inherits the
// parent's repo and autonomous mode. An autonomous child starts Ready with
// its spec approved; any other child goes to the owner's spec checkpoint, or
// straight to Ready when the repo has that checkpoint off.
func NewSplitChild(child SplitChildDraft, parent Ticket, gates Gates, actor Actor, now time.Time) (Ticket, error) {
	kind := child.Kind
	if strings.TrimSpace(kind) == "" {
		kind = string(parent.Kind)
	}
	draft := TicketDraft{RepoID: parent.RepoID, Kind: kind, Title: child.Title, Body: child.Body}
	ticket, err := NewTicket(draft, actor, now)
	if err != nil {
		return Ticket{}, err
	}
	if err := ticket.reviseSpec(child.Spec, now); err != nil {
		return Ticket{}, err
	}
	switch {
	case parent.IsAutonomous:
		ticket.IsAutonomous = true
		ticket.ApprovedSpecRevision = ticket.SpecRevision
		ticket.Stage = StageReady
	case gates.Spec:
		ticket.Stage = StageSpecReview
	default:
		ticket.Stage = StageReady
	}
	ticket.Lane = ticket.Stage.Lane()
	return ticket, nil
}

// SplitChild is one child of a split, as the board shows it.
type SplitChild struct {
	ParentID int64
	ID       int64
	Title    string
	Stage    Stage
}

// EpicIsFinished reports whether an epic can close: it has children and every
// one of them is deployed.
func EpicIsFinished(children []SplitChild) bool {
	if len(children) == 0 {
		return false
	}
	for _, child := range children {
		if child.Stage != StageDeployed {
			return false
		}
	}
	return true
}

// DecisionFromSplit records why a ticket was split and into what, so the
// decision log carries the audit trail for every criterion that moved.
func DecisionFromSplit(parent Ticket, plan SplitPlan, childIDs []int64, actor Actor, now time.Time) Decision {
	refs := make([]string, len(childIDs))
	for i, id := range childIDs {
		refs[i] = fmt.Sprintf("#%d", id)
	}
	verb := "Split into"
	if plan.IsNarrowing() {
		verb = "Narrowed to its core; moved the rest into"
	}
	summary := truncate(fmt.Sprintf("%s %s: %s", verb, strings.Join(refs, ", "), plan.Reason), MaxDecisionSummaryLength)
	var rationale strings.Builder
	rationale.WriteString(plan.Reason)
	rationale.WriteString("\n\n")
	for i, child := range plan.Children {
		fmt.Fprintf(&rationale, "- #%d %s\n", childIDs[i], singleLine(child.Draft.Title))
	}
	source, weight := DecisionFromAgent, WeightAgent
	if actor.IsHuman() {
		source, weight = DecisionFromHuman, WeightBinding
	}
	return Decision{
		TicketID: parent.ID, Summary: summary, Rationale: truncate(strings.TrimSpace(rationale.String()), MaxDecisionRationaleLength),
		Source: source, Weight: weight, DecidedBy: actor, At: now,
	}
}

// DecisionFromSplitAutonomy records that a child runs autonomously because it
// was split out of an autonomous ticket: the owner's yes to run the parent
// without stopping covers the stories it was broken into.
func DecisionFromSplitAutonomy(ticketID, parentID int64, actor Actor, now time.Time) Decision {
	summary := fmt.Sprintf("Split from autonomous #%d, so it runs to a pull request without stopping too", parentID)
	return Decision{
		TicketID: ticketID, Summary: summary, Source: DecisionFromAgent, Weight: WeightBinding,
		DecidedBy: actor, At: now,
	}
}
