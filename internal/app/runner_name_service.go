package app

import (
	"context"
	"strings"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// swapPlaceholder parks a name during a swap. NormalizeRunnerName rejects a
// leading "@", so no real name can collide with it.
const swapPlaceholder = "@swapping"

// RunnerNames returns every runner name assigned so far. People and agent
// tokens may read it, like the board.
func (s *BoardService) RunnerNames(ctx context.Context) ([]domain.RunnerName, error) {
	var names []domain.RunnerName
	err := s.store.Read(ctx, func(repos Repos) error {
		var err error
		names, err = repos.RunnerNames().Assigned(ctx)
		return err
	})
	return names, err
}

// RunnerDirectory indexes the assigned runner names by worker id.
func (s *BoardService) RunnerDirectory(ctx context.Context) (domain.RunnerDirectory, error) {
	names, err := s.RunnerNames(ctx)
	return domain.NewRunnerDirectory(names), err
}

// RenameRunner gives a named runner the display name a person chose. A name
// still free in the family's pool swaps places with the runner's old name, so
// the pool keeps every name; a name another runner holds is a conflict.
func (s *BoardService) RenameRunner(ctx context.Context, actor domain.Actor, workerID, name string) (domain.RunnerName, error) {
	if !actor.IsHuman() {
		return domain.RunnerName{}, domain.Forbiddenf("only a person renames runners")
	}
	name, err := domain.NormalizeRunnerName(name)
	if err != nil {
		return domain.RunnerName{}, err
	}
	var renamed domain.RunnerName
	err = s.store.Write(ctx, func(repos Repos) error {
		current, err := repos.RunnerNames().ForWorker(ctx, workerID)
		if err != nil {
			return err
		}
		renamed, err = renameInPool(ctx, repos.RunnerNames(), current, name)
		return err
	})
	return renamed, err
}

// EnsureRunnerName gives a worker identity its runner name the first time it
// is seen and returns it. It is idempotent: a worker that already has a name
// keeps it. Agents without a worker id, and families without a pool, get
// none, and their raw identity stays their name.
func (s *AgentService) EnsureRunnerName(ctx context.Context, actor domain.Actor) (domain.RunnerName, bool, error) {
	workerID, isWorker := domain.WorkerIDOf(actor.Name)
	if !actor.IsAgent() || !isWorker {
		return domain.RunnerName{}, false, nil
	}
	var name domain.RunnerName
	var isNamed bool
	err := s.store.Read(ctx, func(repos Repos) error {
		var err error
		name, isNamed, err = existingRunnerName(ctx, repos.RunnerNames(), workerID)
		return err
	})
	if err != nil || isNamed {
		return name, isNamed, err
	}
	err = s.store.Write(ctx, func(repos Repos) error {
		var err error
		name, isNamed, err = assignRunnerName(ctx, repos.RunnerNames(), workerID, s.clock.Now())
		return err
	})
	return name, isNamed, err
}

// assignRunnerName picks and records a worker's name inside one transaction.
// Worker "<family>_<n>" takes the name at position n, so "claude_1" is always
// the family's first name; a worker with no free slot of its own takes the
// last free name, leaving the low positions for the numbered workers.
func assignRunnerName(ctx context.Context, names RunnerNameRepository, workerID string, now time.Time) (domain.RunnerName, bool, error) {
	if existing, isNamed, err := existingRunnerName(ctx, names, workerID); err != nil || isNamed {
		return existing, isNamed, err
	}
	family, slot := domain.WorkerFamily(workerID)
	pool, err := names.Pool(ctx, family)
	if err != nil {
		return domain.RunnerName{}, false, err
	}
	pick, isFree := pickRunnerName(pool, slot)
	if !isFree {
		return domain.RunnerName{}, false, nil
	}
	if err := names.Assign(ctx, family, pick.Position, workerID, now); err != nil {
		return domain.RunnerName{}, false, err
	}
	pick.Worker, pick.AssignedAt = workerID, now
	return pick, true, nil
}

// existingRunnerName reads a worker's name, treating not-found as unnamed.
func existingRunnerName(ctx context.Context, names RunnerNameRepository, workerID string) (domain.RunnerName, bool, error) {
	name, err := names.ForWorker(ctx, workerID)
	if domain.KindOf(err) == domain.KindNotFound {
		return domain.RunnerName{}, false, nil
	}
	return name, err == nil, err
}

// pickRunnerName prefers the worker's own slot, then the last free name.
func pickRunnerName(pool []domain.RunnerName, slot int) (domain.RunnerName, bool) {
	for _, candidate := range pool {
		if candidate.Position == slot && !candidate.IsAssigned() {
			return candidate, true
		}
	}
	for i := len(pool) - 1; i >= 0; i-- {
		if !pool[i].IsAssigned() {
			return pool[i], true
		}
	}
	return domain.RunnerName{}, false
}

// renameInPool renames current to name, swapping with a free pool entry that
// already holds name.
func renameInPool(ctx context.Context, names RunnerNameRepository, current domain.RunnerName, name string) (domain.RunnerName, error) {
	pool, err := names.Pool(ctx, current.Family)
	if err != nil {
		return domain.RunnerName{}, err
	}
	for _, other := range pool {
		if other.Position == current.Position || !strings.EqualFold(other.Name, name) {
			continue
		}
		if other.IsAssigned() {
			return domain.RunnerName{}, domain.Conflictf("runner %s is already named %s", other.Worker, other.Name)
		}
		if err := swapNames(ctx, names, current, other, name); err != nil {
			return domain.RunnerName{}, err
		}
		current.Name = name
		return current, nil
	}
	if err := names.SetName(ctx, current.Family, current.Position, name); err != nil {
		return domain.RunnerName{}, err
	}
	current.Name = name
	return current, nil
}

// swapNames gives current the name and the free entry current's old name.
func swapNames(ctx context.Context, names RunnerNameRepository, current, free domain.RunnerName, name string) error {
	if err := names.SetName(ctx, free.Family, free.Position, swapPlaceholder); err != nil {
		return err
	}
	if err := names.SetName(ctx, current.Family, current.Position, name); err != nil {
		return err
	}
	return names.SetName(ctx, free.Family, free.Position, current.Name)
}

// leaseHolderLabel names a ticket's lease holder for agent-facing text, or is
// empty when the ticket has no lease.
func leaseHolderLabel(ctx context.Context, repos Repos, ticket domain.Ticket) (string, error) {
	if ticket.Lease == nil {
		return "", nil
	}
	workerID, isWorker := domain.WorkerIDOf(ticket.Lease.Holder)
	if !isWorker {
		return ticket.Lease.Holder, nil
	}
	name, _, err := existingRunnerName(ctx, repos.RunnerNames(), workerID)
	return domain.RunnerLabel(ticket.Lease.Holder, name.Name), err
}
