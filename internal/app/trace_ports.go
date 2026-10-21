package app

import (
	"context"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// WorkLinkRepository persists PRs and commits linked to tickets.
type WorkLinkRepository interface {
	ForTicket(ctx context.Context, ticketID int64) ([]domain.WorkLink, error)
	Insert(ctx context.Context, link *domain.WorkLink) error
	// Delete removes a link of the ticket and returns it.
	Delete(ctx context.Context, ticketID, linkID int64) (domain.WorkLink, error)
	CountByAgents(ctx context.Context, ticketID int64) (int, error)
	// TicketsFor returns ids of tickets with a commit whose sha starts with
	// shaPrefix, or a PR with exactly prURL. Either may be empty.
	TicketsFor(ctx context.Context, shaPrefix, prURL string, limit int) ([]int64, error)
}

// DecisionRepository persists ticket decision logs.
type DecisionRepository interface {
	ForTicket(ctx context.Context, ticketID int64) ([]domain.Decision, error)
	Get(ctx context.Context, ticketID, id int64) (domain.Decision, error)
	Insert(ctx context.Context, decision *domain.Decision) error
	SetSupersededBy(ctx context.Context, id int64, by *int64) error
	SetSupersedes(ctx context.Context, id int64, previous *int64) error
	// Tombstone clears a decision's content and marks it removed.
	Tombstone(ctx context.Context, id int64) error
	CountByAgents(ctx context.Context, ticketID int64) (int, error)
}

// RelationRepository persists links between tickets.
type RelationRepository interface {
	// ForTicket returns relations in both directions.
	ForTicket(ctx context.Context, ticketID int64) ([]domain.Relation, error)
	Insert(ctx context.Context, relation domain.Relation) error
	Delete(ctx context.Context, relation domain.Relation) error
	CountOutgoing(ctx context.Context, ticketID int64) (int, error)
}
