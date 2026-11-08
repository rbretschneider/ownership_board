package app

import (
	"context"

	"github.com/rbretschneider/ownership_board/internal/domain"
	"github.com/rbretschneider/ownership_board/internal/stats"
)

// StatsQuery picks the period of a stats report.
type StatsQuery struct {
	Days int
	// OffsetMinutes is the owner's UTC offset, so days end at their midnight.
	OffsetMinutes int
}

// Stats reports delivery metrics over the queried window, read from the
// tickets and their recorded history. It only reads.
func (s *BoardService) Stats(ctx context.Context, query StatsQuery) (stats.Report, error) {
	window, err := stats.NewWindow(s.clock.Now(), query.Days, query.OffsetMinutes)
	if err != nil {
		return stats.Report{}, err
	}
	var report stats.Report
	err = s.store.Read(ctx, func(repos Repos) error {
		input, err := loadStatsInput(ctx, repos)
		if err != nil {
			return err
		}
		report = stats.Compute(input, window)
		tickets := make(map[int64]domain.Ticket, len(input.Tickets))
		for _, ticket := range input.Tickets {
			tickets[ticket.ID] = ticket
		}
		for _, repo := range report.Repos {
			health := repo.Health
			if health == nil || health.LastCheckTicket == nil {
				continue
			}
			relations, err := repos.Relations().ForTicket(ctx, *health.LastCheckTicket)
			if err != nil {
				return err
			}
			open := make(map[int64]bool)
			for _, relation := range relations {
				if relation.Kind != domain.RelationFollowUpOf || relation.ToID != *health.LastCheckTicket {
					continue
				}
				if ticket, exists := tickets[relation.FromID]; exists && ticket.Stage != domain.StageDone {
					open[ticket.ID] = true
				}
			}
			health.OpenFollowUps = len(open)
		}
		return nil
	})
	if err != nil {
		return stats.Report{}, err
	}
	return report, nil
}

func loadStatsInput(ctx context.Context, repos Repos) (stats.Input, error) {
	var input stats.Input
	var err error
	if input.Tickets, err = repos.Tickets().List(ctx, TicketFilter{}); err != nil {
		return input, err
	}
	if input.OpenQuestions, err = repos.Questions().OpenCounts(ctx); err != nil {
		return input, err
	}
	if input.Timeline, err = repos.Timeline().OfTypes(ctx, stats.TimelineTypes); err != nil {
		return input, err
	}
	input.Repos, err = repos.CodeRepos().List(ctx)
	return input, err
}
