package app

import (
	"context"

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
	var input stats.Input
	err = s.store.Read(ctx, func(repos Repos) error {
		var err error
		input, err = loadStatsInput(ctx, repos)
		return err
	})
	if err != nil {
		return stats.Report{}, err
	}
	return stats.Compute(input, window), nil
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
