package app

import (
	"context"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// recentJobsPerTicket bounds the build jobs read per ticket for cycle checks.
const recentJobsPerTicket = 4

// ManagerStatus is what the board manager needs before it spends anything:
// whether it is on, its limits, and what it already spent today.
type ManagerStatus struct {
	Settings domain.BoardSettings
	Today    domain.ManagerUsage
	// RemainingUSD is today's budget left; zero or less means no model sessions.
	RemainingUSD float64
}

func requireManagerOrPerson(actor domain.Actor) error {
	if actor.IsHuman() || actor.IsBoardManager() {
		return nil
	}
	return domain.Forbiddenf("only a person or the board manager token can read the manager's view")
}

// ManagerFlags lists what needs the board manager in scope: autonomous tickets
// that are escalated, cycling, over the build cap or stalled. It runs no
// model, so the manager can poll it cheaply and spend only when it is not empty.
func (s *BoardService) ManagerFlags(ctx context.Context, actor domain.Actor, scope RepoScope) ([]domain.ManagerFlag, error) {
	if err := requireManagerOrPerson(actor); err != nil {
		return nil, err
	}
	var flags []domain.ManagerFlag
	err := s.store.Read(ctx, func(repos Repos) error {
		settings, err := loadSettings(ctx, repos)
		if err != nil {
			return err
		}
		board, err := loadBoard(ctx, repos, scope.filter())
		if err != nil {
			return err
		}
		var facts []domain.ManagerTicketFacts
		for _, summary := range board.Tickets {
			if !summary.Ticket.IsAutonomous || summary.Ticket.Stage.IsCompleted() {
				continue
			}
			jobs, err := repos.BuildJobs().RecentForTicket(ctx, summary.Ticket.ID, recentJobsPerTicket)
			if err != nil {
				return err
			}
			facts = append(facts, domain.ManagerTicketFacts{Ticket: summary.Ticket, OpenQuestions: summary.OpenQuestions,
				WaitingOn: summary.WaitingOn, Attempts: summary.Attempts, RecentJobs: jobs})
		}
		flags = domain.ManagerFlags(facts, settings, s.clock.Now())
		return nil
	})
	return flags, err
}

// ManagerStatus returns the manager's settings and today's spend.
func (s *BoardService) ManagerStatus(ctx context.Context, actor domain.Actor) (ManagerStatus, error) {
	if err := requireManagerOrPerson(actor); err != nil {
		return ManagerStatus{}, err
	}
	var status ManagerStatus
	err := s.store.Read(ctx, func(repos Repos) error {
		settings, err := loadSettings(ctx, repos)
		if err != nil {
			return err
		}
		today, err := repos.ManagerUsage().Day(ctx, s.clock.Now().UTC().Format("2006-01-02"))
		if err != nil {
			return err
		}
		status = ManagerStatus{Settings: settings, Today: today, RemainingUSD: float64(settings.ManagerDailyBudgetUSD) - today.CostUSD}
		return nil
	})
	return status, err
}

// RecordManagerUsage adds one manager session's spend to today's total.
func (s *BoardService) RecordManagerUsage(ctx context.Context, actor domain.Actor, costUSD float64) (ManagerStatus, error) {
	if !actor.IsBoardManager() {
		return ManagerStatus{}, domain.Forbiddenf("only the board manager token records manager usage")
	}
	if costUSD < 0 || costUSD > 1000 {
		return ManagerStatus{}, domain.Invalidf("cost_usd must be between 0 and 1000")
	}
	now := s.clock.Now()
	if err := s.store.Write(ctx, func(repos Repos) error {
		return repos.ManagerUsage().Add(ctx, now.UTC().Format("2006-01-02"), 1, costUSD, now)
	}); err != nil {
		return ManagerStatus{}, err
	}
	return s.ManagerStatus(ctx, actor)
}
