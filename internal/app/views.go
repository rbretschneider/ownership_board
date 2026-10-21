package app

import (
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// TicketSummary is a ticket as shown on a card.
type TicketSummary struct {
	Ticket        domain.Ticket
	OpenQuestions int
}

// TicketDetail is a ticket with everything needed to work on it.
type TicketDetail struct {
	TicketSummary
	Questions []domain.Question
	Timeline  []domain.TimelineEntry
}

// Board is everything the board screen shows.
type Board struct {
	Lanes   []domain.LaneInfo
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
