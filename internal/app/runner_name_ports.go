package app

import (
	"context"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// RunnerNameRepository persists each agent family's pool of runner names and
// which worker holds each one.
type RunnerNameRepository interface {
	// Assigned returns every assigned name, by family then position.
	Assigned(ctx context.Context) ([]domain.RunnerName, error)
	// ForWorker returns the name a worker holds, or a not-found error.
	ForWorker(ctx context.Context, workerID string) (domain.RunnerName, error)
	// Pool returns a family's names in display order, assigned or not.
	Pool(ctx context.Context, family string) ([]domain.RunnerName, error)
	// Assign gives the free name at a family's position to a worker. It fails
	// with a conflict when the name or the worker is already taken.
	Assign(ctx context.Context, family string, position int, workerID string, at time.Time) error
	// SetName renames the name at a family's position.
	SetName(ctx context.Context, family string, position int, name string) error
}
