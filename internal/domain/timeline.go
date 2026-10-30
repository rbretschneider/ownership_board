package domain

import "time"

// TimelineType names something that happened to a ticket.
type TimelineType string

// The timeline entry types.
const (
	TimelineCreated   TimelineType = "created"
	TimelineMoved     TimelineType = "moved"
	TimelineComment   TimelineType = "comment"
	TimelineSpec      TimelineType = "spec"
	TimelineQuestions TimelineType = "questions"
	TimelineAnswers   TimelineType = "answers"
	TimelineEdited    TimelineType = "edited"
	TimelineClaimed   TimelineType = "claimed"
	TimelineReleased  TimelineType = "released"
	// TimelineDeleted is only ever published as an event; a deleted ticket has
	// no timeline left to hold it.
	TimelineDeleted  TimelineType = "deleted"
	TimelineWorkLink TimelineType = "work_link"
	TimelineDecision TimelineType = "decision"
	TimelineRelation TimelineType = "relation"
	// TimelineStage records a stage change, data {from, to}.
	TimelineStage            TimelineType = "stage"
	TimelineApproved         TimelineType = "approved"
	TimelineChangesRequested TimelineType = "changes_requested"
	TimelineBlocked          TimelineType = "blocked"
	TimelineUnblocked        TimelineType = "unblocked"
	TimelineDesign           TimelineType = "design"
	// TimelineDependenciesMet records that the last ticket this one depended
	// on was merged, so agents can claim it; data {ticket_id, merged_by_kind},
	// with the actor that merged that ticket.
	TimelineDependenciesMet TimelineType = "dependencies_met"
)

// TimelineEntry is one line of a ticket's history.
type TimelineEntry struct {
	ID       int64
	TicketID int64
	Type     TimelineType
	Actor    Actor
	Body     string
	Data     map[string]any
	At       time.Time
}

// NewEntry builds an unsaved timeline entry.
func NewEntry(ticketID int64, kind TimelineType, actor Actor, now time.Time) TimelineEntry {
	return TimelineEntry{TicketID: ticketID, Type: kind, Actor: actor, Data: map[string]any{}, At: now}
}

// WithBody returns a copy of the entry carrying text.
func (e TimelineEntry) WithBody(body string) TimelineEntry {
	e.Body = body
	return e
}

// With returns a copy of the entry with one more data field.
func (e TimelineEntry) With(key string, value any) TimelineEntry {
	data := make(map[string]any, len(e.Data)+1)
	for k, v := range e.Data {
		data[k] = v
	}
	data[key] = value
	e.Data = data
	return e
}
