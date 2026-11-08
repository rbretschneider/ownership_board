package app

import (
	"context"
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
	Host domain.BuildHost
	Jobs []RunnerAssignment
}
type BuildHostRunners struct {
	ObservedAt time.Time
	Hosts      []BuildHostRunner
	QueuedByOS map[domain.BuildHostOS]int
}

func (s *BuildHostService) Runners(ctx context.Context, actor domain.Actor) (BuildHostRunners, error) {
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
		for _, h := range hosts {
			if h.Status != domain.BuildHostApproved {
				continue
			}
			jobs := byHost[h.ID]
			if jobs == nil {
				jobs = []RunnerAssignment{}
			}
			out.Hosts = append(out.Hosts, BuildHostRunner{Host: h, Jobs: jobs})
		}
		return nil
	})
	return out, err
}
