package app

import (
	"context"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// fileHealthCheck runs inside the merge's write transaction. The store's writer
// lock serializes eligibility, creation and checkpoint advancement.
func fileHealthCheck(ctx context.Context, repos Repos, repoID int64, now time.Time) ([]Event, error) {
	if repoID == 0 {
		return nil, nil
	}
	repo, err := repos.CodeRepos().Get(ctx, repoID)
	if err != nil || repo.HealthCheckEvery == 0 {
		return nil, err
	}
	open, err := repos.Tickets().HasOpenHealthCheck(ctx, repoID)
	if err != nil || open {
		return nil, err
	}
	covered, err := repos.Tickets().MergedSinceHealthCheck(ctx, repoID, repo.HealthCheckedThroughMergeSequence)
	if err != nil || len(covered) < repo.HealthCheckEvery {
		return nil, err
	}
	checkpointCommit := ""
	if repo.HealthCheckedThroughTicket != nil {
		links, err := repos.WorkLinks().ForTicket(ctx, *repo.HealthCheckedThroughTicket)
		if err != nil {
			return nil, err
		}
		checkpointCommit = recordedCommit(links, repoID)
	}
	ticket, err := domain.NewHealthCheckTicket(repo, covered, checkpointCommit, now)
	if err != nil {
		return nil, err
	}
	ticket.Rank, err = rankFor(ctx, repos.Tickets(), domain.LaneTodo, 0, atTop)
	if err != nil {
		return nil, err
	}
	if err := repos.Tickets().Insert(ctx, &ticket); err != nil {
		return nil, err
	}
	entries := []domain.TimelineEntry{
		domain.NewEntry(ticket.ID, domain.TimelineCreated, domain.SystemActor, now).With("autonomous", true),
		domain.NewEntry(ticket.ID, domain.TimelineSpec, domain.SystemActor, now).With("revision", 1).With("submitted", true),
	}
	events, err := record(ctx, repos, ticket, entries)
	if err != nil {
		return nil, err
	}
	last := covered[len(covered)-1]
	if err := repos.CodeRepos().RecordHealthCheck(ctx, repoID, last.ID, last.HealthCheckMergeSequence, now); err != nil {
		return nil, err
	}
	return events, nil
}

// Prefer a full SHA, then the newest timestamp and insertion ID. Work links do
// not establish that a commit is a merge commit or the parent of the range.
func recordedCommit(links []domain.WorkLink, repoID int64) string {
	var best domain.WorkLink
	for _, link := range links {
		if link.Kind != domain.WorkLinkCommit || link.RepoID != repoID {
			continue
		}
		sha, err := domain.ParseSHA(link.SHA)
		if err != nil {
			continue
		}
		link.SHA = sha
		full, bestFull := len(link.SHA) == 40, len(best.SHA) == 40
		if best.SHA == "" || (full && !bestFull) || (full == bestFull && (link.AddedAt.After(best.AddedAt) || (link.AddedAt.Equal(best.AddedAt) && link.ID > best.ID))) {
			best = link
		}
	}
	return best.SHA
}
