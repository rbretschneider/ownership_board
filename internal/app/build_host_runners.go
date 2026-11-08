package app

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// RunnerAssignment is the presentation subset of an unreleased assignment.
type RunnerAssignment struct {
	HostID            int64
	ID                string
	TicketID          int64
	Repo, SHA, Preset string
	Status            string
	Phase, Detail     string
	StartedAt         *time.Time
	Attempt           int
}
type BuildHostRunner struct {
	Host  domain.BuildHost
	Jobs  []RunnerAssignment
	Queue []domain.BuildJob
}
type BuildHostRunners struct {
	ObservedAt time.Time
	Hosts      []BuildHostRunner
	QueuedByOS map[domain.BuildHostOS]int
}

func (s *BuildHostService) Runners(ctx context.Context, actor domain.Actor) (BuildHostRunners, error) {
	return s.runners(ctx, actor, false)
}

// CompatibilityRunners adds bounded host-eligible queues for legacy readers.
func (s *BuildHostService) CompatibilityRunners(ctx context.Context, actor domain.Actor) (BuildHostRunners, error) {
	return s.runners(ctx, actor, true)
}

func (s *BuildHostService) runners(ctx context.Context, actor domain.Actor, withQueues bool) (BuildHostRunners, error) {
	out := BuildHostRunners{ObservedAt: s.clock.Now(), Hosts: []BuildHostRunner{}, QueuedByOS: map[domain.BuildHostOS]int{"windows": 0, "linux": 0, "macos": 0}}
	if err := requireBuildHostHuman(actor); err != nil {
		return out, err
	}
	err := s.store.Read(ctx, func(r Repos) error {
		hosts, err := r.BuildHosts().List(ctx)
		if err != nil {
			return err
		}
		assignments, err := r.BuildJobs().RunnerAssignments(ctx)
		if err != nil {
			return err
		}
		counts, err := r.BuildJobs().QueuedCountsByOS(ctx)
		if err != nil {
			return err
		}
		for os, n := range counts {
			out.QueuedByOS[os] = n
		}
		byHost := map[int64][]RunnerAssignment{}
		for _, a := range assignments {
			byHost[a.HostID] = append(byHost[a.HostID], a)
		}
		slices.SortFunc(hosts, func(a, b domain.BuildHost) int { return cmp.Compare(a.ID, b.ID) })
		for _, h := range hosts {
			if h.Status != domain.BuildHostApproved {
				continue
			}
			jobs := byHost[h.ID]
			if jobs == nil {
				jobs = []RunnerAssignment{}
			}
			row := BuildHostRunner{Host: h, Jobs: jobs}
			if withQueues {
				row.Queue, err = r.BuildJobs().QueuedForHost(ctx, h, 40)
				if err != nil {
					return err
				}
			}
			out.Hosts = append(out.Hosts, row)
		}
		return nil
	})
	return out, err
}
