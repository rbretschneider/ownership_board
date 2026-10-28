package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// DecisionSource says how a decision was recorded.
type DecisionSource string

// The decision sources.
const (
	DecisionFromAnswer DecisionSource = "answer"
	DecisionFromAgent  DecisionSource = "agent"
	DecisionFromHuman  DecisionSource = "human"
	// DecisionFromApproval is the owner approving a spec or design revision.
	DecisionFromApproval DecisionSource = "approval"
	// DecisionFromChat is the owner agreeing a spec in a chat with a delegate
	// agent, which filed it as an autonomous ticket.
	DecisionFromChat DecisionSource = "chat"
)

// DecisionWeight says how much a decision binds later work.
type DecisionWeight string

// The decision weights.
const (
	// WeightBinding is the owner's own explicit choice. Agents must follow it
	// and may not replace it.
	WeightBinding DecisionWeight = "binding"
	// WeightDelegated is the owner letting the agent decide ("your call", or
	// accepting every recommendation at once). It is context, not a ruling.
	WeightDelegated DecisionWeight = "delegated"
	// WeightAgent is a decision an agent recorded. It is context.
	WeightAgent DecisionWeight = "agent"
)

// Decision limits.
const (
	MaxDecisionSummaryLength   = 300
	MaxDecisionRationaleLength = 2_000
	MaxDecisionAlternatives    = 5
	MaxAlternativeLength       = 200
	MaxAgentDecisionsPerTicket = 200
)

// Decision is one entry in a ticket's decision log.
type Decision struct {
	ID             int64
	TicketID       int64
	Summary        string
	Rationale      string
	Alternatives   []string
	Source         DecisionSource
	Weight         DecisionWeight
	DecidedBy      Actor
	AskedBy        string
	QuestionID     *int64
	SupersedesID   *int64
	SupersededByID *int64
	IsRemoved      bool
	At             time.Time
}

// DecisionDraft is a person's or agent's input for a decision.
type DecisionDraft struct {
	Summary      string
	Rationale    string
	Alternatives []string
	SupersedesID *int64
}

// NewDecision validates a recorded decision. Source, weight and author come
// from who is recording it, never from the input.
func NewDecision(draft DecisionDraft, ticketID int64, actor Actor, now time.Time) (Decision, error) {
	summary, err := RequireText("summary", singleLine(draft.Summary), MaxDecisionSummaryLength)
	if err != nil {
		return Decision{}, err
	}
	rationale, err := LimitText("rationale", strings.TrimSpace(draft.Rationale), MaxDecisionRationaleLength)
	if err != nil {
		return Decision{}, err
	}
	alternatives, err := cleanAlternatives(draft.Alternatives)
	if err != nil {
		return Decision{}, err
	}
	source, weight := DecisionFromHuman, WeightBinding
	if !actor.IsHuman() {
		source, weight = DecisionFromAgent, WeightAgent
	}
	return Decision{
		TicketID: ticketID, Summary: summary, Rationale: rationale, Alternatives: alternatives,
		Source: source, Weight: weight, DecidedBy: actor, SupersedesID: draft.SupersedesID, At: now,
	}, nil
}

// DecisionFromQuestion records a person's answer as a decision. It never fails
// on length: text is cut to fit, because an answer must always save.
func DecisionFromQuestion(question Question, actor Actor, now time.Time) Decision {
	answer := question.Answer
	weight := WeightBinding
	if answer.IsDeferred || answer.IsBulk {
		weight = WeightDelegated
	}
	questionID := question.ID
	summary := truncate(singleLine(fmt.Sprintf("%s → %s", question.Prompt, describeAnswerText(question))), MaxDecisionSummaryLength)
	return Decision{
		TicketID: question.TicketID, Summary: summary, Rationale: truncate(answer.Text, MaxDecisionRationaleLength),
		Source: DecisionFromAnswer, Weight: weight, DecidedBy: actor, AskedBy: question.AskedBy,
		QuestionID: &questionID, At: now,
	}
}

// DecisionFromCheckpoint records the owner's approval of one revision. It is
// always binding: the owner read it and said yes.
func DecisionFromCheckpoint(ticketID int64, checkpoint Checkpoint, revision int, actor Actor, now time.Time) Decision {
	summary := fmt.Sprintf("Approved the %s (revision %d)", checkpoint, revision)
	return Decision{
		TicketID: ticketID, Summary: summary, Source: DecisionFromApproval, Weight: WeightBinding,
		DecidedBy: actor, At: now,
	}
}

// DecisionFromAgreedSpec records that the owner agreed an autonomous ticket's
// spec in a chat. It binds like an approval at the spec checkpoint.
func DecisionFromAgreedSpec(ticketID int64, revision int, actor Actor, now time.Time) Decision {
	summary := fmt.Sprintf("Agreed the spec (revision %d) in a chat; run to a pull request without stopping", revision)
	return Decision{
		TicketID: ticketID, Summary: summary, Source: DecisionFromChat, Weight: WeightBinding,
		DecidedBy: actor, At: now,
	}
}

// DecisionFromAutonomySwitch records a person turning a ticket's autonomous
// mode on or off, so the decision log says who let it run without stopping.
func DecisionFromAutonomySwitch(ticketID int64, isOn bool, actor Actor, now time.Time) Decision {
	summary := "Turned autonomous mode off: the repo's checkpoints apply again"
	if isOn {
		summary = "Turned autonomous mode on: run to a pull request without stopping"
	}
	return Decision{
		TicketID: ticketID, Summary: summary, Source: DecisionFromHuman, Weight: WeightBinding,
		DecidedBy: actor, At: now,
	}
}

// IsActive reports whether the decision still stands.
func (d Decision) IsActive() bool {
	return d.SupersededByID == nil && !d.IsRemoved
}

// CanBeSupersededBy reports whether actor may replace this decision. Only a
// person may replace a binding decision; an agent that disagrees must ask.
func (d Decision) CanBeSupersededBy(actor Actor) error {
	if !d.IsActive() {
		return Conflictf("decision %d has already been replaced or removed", d.ID)
	}
	if d.Weight == WeightBinding && !actor.IsHuman() {
		return Forbiddenf("decision %d is the owner's; ask a question instead of replacing it", d.ID)
	}
	return nil
}

func describeAnswerText(question Question) string {
	answer := question.Answer
	switch {
	case answer.IsDeferred:
		return "left to the agent's recommendation"
	case answer.Choice != nil:
		return question.Options[*answer.Choice]
	default:
		return answer.Text
	}
}

func cleanAlternatives(raw []string) ([]string, error) {
	if len(raw) > MaxDecisionAlternatives {
		return nil, Invalidf("at most %d alternatives", MaxDecisionAlternatives)
	}
	alternatives := make([]string, 0, len(raw))
	for _, alternative := range raw {
		value, err := RequireText("alternative", singleLine(alternative), MaxAlternativeLength)
		if err != nil {
			return nil, err
		}
		alternatives = append(alternatives, value)
	}
	return alternatives, nil
}

// singleLine folds a short field onto one line, so it can never start a new
// markdown block when rendered into a brief.
func singleLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func truncate(value string, max int) string {
	if utf8.RuneCountInString(value) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max-1]) + "…"
}
