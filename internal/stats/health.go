package stats

import "github.com/rbretschneider/ownership_board/internal/domain"

func repoHealth(tickets []history, repos []domain.Repo) map[int64]*RepoHealth {
	health := make(map[int64]*RepoHealth, len(repos))
	checkpoints := make(map[int64]int64, len(repos))
	latest := make(map[int64]domain.Ticket)
	for _, repo := range repos {
		health[repo.ID] = &RepoHealth{Every: repo.HealthCheckEvery, LastCheckedAt: repo.HealthCheckedAt}
		checkpoints[repo.ID] = repo.HealthCheckedThroughMergeSequence
	}
	for _, h := range tickets {
		ticket := h.Ticket
		status := health[ticket.RepoID]
		if status == nil {
			continue
		}
		if ticket.IsHealthCheck {
			previous, exists := latest[ticket.RepoID]
			if !exists || ticket.CreatedAt.After(previous.CreatedAt) || (ticket.CreatedAt.Equal(previous.CreatedAt) && ticket.ID > previous.ID) {
				latest[ticket.RepoID] = ticket
				id := ticket.ID
				status.LastCheckTicket = &id
			}
		} else if ticket.Stage.IsCompleted() && ticket.HealthCheckMergeSequence > checkpoints[ticket.RepoID] {
			status.MergesSinceCheck++
		}
	}
	for _, status := range health {
		if status.Every > 0 {
			next := max(0, status.Every-status.MergesSinceCheck)
			status.NextIn = &next
		}
	}
	return health
}
