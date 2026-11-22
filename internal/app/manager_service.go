package app

import (
	"context"
	"strings"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// recentJobsPerTicket bounds the build jobs read per ticket for cycle checks.
const recentJobsPerTicket = 4

// ManagerStatus is what the board manager needs before it spends anything:
// whether it is on, its limits, and what it already spent today.
type ManagerStatus struct {
	Settings domain.BoardSettings
	Today    domain.ManagerUsage
	// RemainingUSD is today's budget left; zero or less means no model sessions,
	// unless the owner switched the budget off (Settings.ManagerBudgetOff).
	RemainingUSD float64
}

// CanSpend reports whether the manager may start a model session now.
func (s ManagerStatus) CanSpend() bool {
	return s.Settings.ManagerBudgetOff || s.RemainingUSD > 0
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
		flags, err = managerFlagsOf(ctx, repos, board.Tickets, settings, s.clock.Now())
		return err
	})
	return flags, err
}

// managerFlagsOf derives the manager's flags from loaded ticket summaries.
func managerFlagsOf(ctx context.Context, repos Repos, tickets []TicketSummary, settings domain.BoardSettings, now time.Time) ([]domain.ManagerFlag, error) {
	moving, err := queuesMoving(ctx, repos, now)
	if err != nil {
		return nil, err
	}
	var facts []domain.ManagerTicketFacts
	for _, summary := range tickets {
		if !summary.Ticket.IsAutonomous || summary.Ticket.Stage.IsCompleted() || summary.Ticket.Stage == domain.StageEpic {
			continue
		}
		jobs, err := repos.BuildJobs().RecentForTicket(ctx, summary.Ticket.ID, recentJobsPerTicket)
		if err != nil {
			return nil, err
		}
		facts = append(facts, domain.ManagerTicketFacts{Ticket: summary.Ticket, OpenQuestions: summary.OpenQuestions,
			WaitingOn: summary.WaitingOn, Attempts: summary.Attempts, RecentJobs: jobs, QueueMoving: moving})
	}
	return domain.ManagerFlags(facts, settings, now), nil
}

// queueStartWindow is how recently a host must have started work for its
// queue to count as moving.
const queueStartWindow = time.Hour

// queuesMoving reports, per target OS, whether a build host started a job or
// batch within queueStartWindow. A long queue on a host that keeps starting
// work is capacity, not a fault (2026-11-22: with a six-hour Windows queue the
// manager spent about a dollar an hour confirming each "queued 2h" flag).
func queuesMoving(ctx context.Context, repos Repos, now time.Time) (map[domain.BuildHostOS]bool, error) {
	moving := map[domain.BuildHostOS]bool{}
	jobs, err := repos.BuildJobs().ActiveAssignments(ctx)
	if err != nil {
		return nil, err
	}
	for _, job := range jobs {
		if job.StartedAt != nil && now.Sub(*job.StartedAt) < queueStartWindow {
			moving[job.Target.OS] = true
		}
	}
	batches, err := repos.BuildBatches().ActiveAssignments(ctx)
	if err != nil {
		return nil, err
	}
	for _, batch := range batches {
		if batch.StartedAt != nil && now.Sub(*batch.StartedAt) < queueStartWindow {
			moving[batch.Target.OS] = true
		}
	}
	return moving, nil
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

// ManagerSessionInput is what the manager loop reports after one session.
type ManagerSessionInput struct {
	CostUSD float64
	Model   string
	// Summary is the session's own account of why it acted and what it did,
	// shown to the owner in Settings > Board manager.
	Summary string
	// Headline is the session's one plain sentence for the Team view. It is
	// stored as the summary's first "HEADLINE:" line unless the summary has one.
	Headline string
}

// RecordManagerUsage adds one manager session's spend to today's total and
// keeps its summary, so the owner can read the manager's reasoning in the app.
func (s *BoardService) RecordManagerUsage(ctx context.Context, actor domain.Actor, input ManagerSessionInput) (ManagerStatus, error) {
	if !actor.IsBoardManager() {
		return ManagerStatus{}, domain.Forbiddenf("only the board manager token records manager usage")
	}
	if input.CostUSD < 0 || input.CostUSD > 1000 {
		return ManagerStatus{}, domain.Invalidf("cost_usd must be between 0 and 1000")
	}
	if err := rejectHiddenFromAgent(actor, map[string]string{"summary": input.Summary, "model": input.Model, "headline": input.Headline}); err != nil {
		return ManagerStatus{}, err
	}
	summary := strings.TrimSpace(domain.WithManagerHeadline(input.Summary, input.Headline))
	if len(summary) > domain.MaxManagerSummaryLength {
		summary = summary[:domain.MaxManagerSummaryLength]
	}
	model := strings.TrimSpace(input.Model)
	if len(model) > 80 {
		model = model[:80]
	}
	now := s.clock.Now()
	if err := s.store.Write(ctx, func(repos Repos) error {
		if err := repos.ManagerUsage().Add(ctx, now.UTC().Format("2006-01-02"), 1, input.CostUSD, now); err != nil {
			return err
		}
		return repos.ManagerUsage().AddSession(ctx, domain.ManagerSession{At: now, Model: model, CostUSD: input.CostUSD, Summary: summary})
	}); err != nil {
		return ManagerStatus{}, err
	}
	return s.ManagerStatus(ctx, actor)
}

// ManagerSessions lists the manager's most recent sessions, newest first.
func (s *BoardService) ManagerSessions(ctx context.Context, actor domain.Actor, limit int) ([]domain.ManagerSession, error) {
	if err := requireManagerOrPerson(actor); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var sessions []domain.ManagerSession
	err := s.store.Read(ctx, func(repos Repos) error {
		var err error
		sessions, err = repos.ManagerUsage().RecentSessions(ctx, limit)
		return err
	})
	return sessions, err
}
