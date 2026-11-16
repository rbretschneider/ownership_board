package app

import (
	"context"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// VerdictRepository persists one verdict per (repo, target, commit).
type VerdictRepository interface {
	// Record stores a verdict, replacing any earlier one for its commit and target.
	Record(context.Context, domain.Verdict) error
	// Get returns the verdict for a commit on a target, or a not-found error.
	Get(ctx context.Context, repo, target, sha string) (domain.Verdict, error)
	// ForCommit returns a commit's verdicts on every target.
	ForCommit(ctx context.Context, repo, sha string) ([]domain.Verdict, error)
	// ForTicket returns a ticket's verdicts, newest first, at most limit.
	ForTicket(ctx context.Context, ticketID int64, limit int) ([]domain.Verdict, error)
	// Latest returns each ticket's newest verdict per target, for the tickets given.
	Latest(ctx context.Context, ticketIDs []int64) (map[int64][]domain.Verdict, error)
}
