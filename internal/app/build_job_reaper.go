package app

import (
	"context"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

func (s *BuildJobService) recover(ctx context.Context, r Repos, now time.Time) error {
	jobs, e := r.BuildJobs().ActiveAssignments(ctx)
	if e != nil {
		return e
	}
	byID := map[int64]domain.BuildHost{}
	for _, j := range jobs {
		if j.HostID == nil || j.AssignmentReleasedAt != nil || j.StartedAt == nil {
			continue
		}
		h, ok := byID[*j.HostID]
		if !ok {
			h, e = r.BuildHosts().Get(ctx, *j.HostID)
			if e != nil {
				return e
			}
			byID[h.ID] = h
		}
		seen := j.StartedAt
		if h.LastSeenAt != nil {
			seen = h.LastSeenAt
		}
		if h.Status != domain.BuildHostRevoked && now.Sub(*seen) < 3*time.Minute {
			continue
		}
		if j.Status == "superseded" || j.Status == "cancelled" {
			j.AssignmentReleasedAt = &now
		} else if j.Status == "running" {
			if j.Attempt >= BuildJobAttempts {
				j.Status = "failed"
				j.FinishedAt = &now
				j.AssignmentReleasedAt = &now
				j.Phase = "failed"
				j.Detail = "host connectivity lost after three attempts"
				j.Result = []byte(`{"commands":[],"failure_kind":"infrastructure"}`)
			} else {
				j.Status = "queued"
				j.Attempt++
				j.HostID = nil
				j.StartedAt = nil
				j.FinishedAt = nil
				j.Phase = ""
				j.Detail = ""
				j.Result = nil
			}
		} else {
			continue
		}
		if e = r.BuildJobs().Save(ctx, j); e != nil {
			return e
		}
	}
	return nil
}
func (s *BuildJobService) RequeueOffline(ctx context.Context) error {
	return s.store.Write(ctx, func(r Repos) error { return s.recover(ctx, r, s.now()) })
}
func (s *BuildJobService) RunReaper(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		if e := s.RequeueOffline(ctx); e != nil && ctx.Err() == nil {
			s.logger.Error("recover offline build jobs", "error", e)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
