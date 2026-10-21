package domain

import (
	"strings"
	"time"
)

// Question is one item in a round of clarifying questions an agent asks.
type Question struct {
	ID          int64
	TicketID    int64
	Round       int
	Position    int
	Prompt      string
	Options     []string
	Recommended *int
	AskedBy     string
	AskedAt     time.Time
	Answer      *Answer
}

// QuestionDraft is the agent's input for one question.
type QuestionDraft struct {
	Prompt      string
	Options     []string
	Recommended *int
}

// AnswerDraft is the human's input for one question.
type AnswerDraft struct {
	QuestionID int64
	Choice     *int
	Text       string
	IsDeferred bool
}

// NewQuestion validates a draft for a ticket's round.
func NewQuestion(draft QuestionDraft, round, position int, actor Actor, now time.Time) (Question, error) {
	prompt, err := RequireText("question prompt", draft.Prompt, MaxPromptLength)
	if err != nil {
		return Question{}, err
	}
	options, err := cleanOptions(draft.Options)
	if err != nil {
		return Question{}, err
	}
	if draft.Recommended != nil && !isValidChoice(*draft.Recommended, options) {
		return Question{}, Invalidf("recommended must index one of the %d options", len(options))
	}
	return Question{
		Round: round, Position: position, Prompt: prompt, Options: options,
		Recommended: draft.Recommended, AskedBy: actor.Name, AskedAt: now,
	}, nil
}

// IsOpen reports whether the question still waits for an answer.
func (q *Question) IsOpen() bool {
	return q.Answer == nil
}

// ApplyAnswer records a human's answer. An answer needs at least one of a
// choice, free text, or a deferral to the agent's judgement.
func (q *Question) ApplyAnswer(draft AnswerDraft, actor Actor, now time.Time) error {
	if !q.IsOpen() {
		return Conflictf("question %d is already answered", q.ID)
	}
	text, err := LimitText("answer", strings.TrimSpace(draft.Text), MaxAnswerLength)
	if err != nil {
		return err
	}
	if draft.Choice != nil && !isValidChoice(*draft.Choice, q.Options) {
		return Invalidf("choice for question %d must index one of its %d options", q.ID, len(q.Options))
	}
	if draft.Choice == nil && text == "" && !draft.IsDeferred {
		return Invalidf("question %d needs a choice, text, or your call", q.ID)
	}
	q.Answer = &Answer{Choice: draft.Choice, Text: text, IsDeferred: draft.IsDeferred, By: actor.Name, At: now}
	return nil
}

func cleanOptions(raw []string) ([]string, error) {
	if len(raw) > MaxOptionsPerPrompt {
		return nil, Invalidf("a question may have at most %d options", MaxOptionsPerPrompt)
	}
	options := make([]string, 0, len(raw))
	for _, option := range raw {
		value, err := RequireText("option", option, MaxOptionLength)
		if err != nil {
			return nil, err
		}
		options = append(options, value)
	}
	return options, nil
}

func isValidChoice(choice int, options []string) bool {
	return choice >= 0 && choice < len(options)
}
