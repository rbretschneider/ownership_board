package app

import (
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// TicketSummary is a ticket as shown on a card.
type TicketSummary struct {
	Ticket        domain.Ticket
	OpenQuestions int
	// WaitingOn lists the tickets this one depends on that are not merged
	// yet; agents cannot claim it while any are.
	WaitingOn []int64
}

// TicketDetail is a ticket with everything needed to work on it.
type TicketDetail struct {
	TicketSummary
	Questions []domain.Question
	Timeline  []domain.TimelineEntry
	WorkLinks []domain.WorkLink
	Decisions []domain.Decision
	Related   []RelatedTicket
	// Repo is the ticket's repo, or nil when it has none.
	Repo *domain.Repo
}

// RelatedTicket is a relation seen from one ticket, with the other end.
type RelatedTicket struct {
	Relation   domain.Relation
	IsOutgoing bool
	Other      TicketRef
}

// TicketRef is enough about another ticket to show and judge a link to it.
type TicketRef struct {
	ID    int64
	Title string
	Lane  domain.Lane
	// Repo is the other ticket's repo, or nil; it decides that ticket's
	// visibility.
	Repo *domain.Repo
}

// Board is everything the board screen shows.
type Board struct {
	Lanes   []domain.LaneInfo
	Stages  []domain.StageInfo
	Repos   []domain.Repo
	Tickets []TicketSummary
}

// Work is a claimed ticket plus the brief the agent should act on.
type Work struct {
	Action domain.WorkAction
	Ticket TicketDetail
	// Repo is nil while the ticket has no repo.
	Repo  *domain.Repo
	Brief string
}

// SyncOutcome is the result of a provider's most recent sync.
type SyncOutcome struct {
	At    time.Time
	Error string
}
