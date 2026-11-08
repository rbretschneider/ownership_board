package stats_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestHealthProgressUsesEligibleTickets(t *testing.T) {
	for _, tc := range []struct {
		name       string
		every      int
		checkpoint int64
		want       int
		next       int
	}{
		{"first", 4, 0, 3, 1}, {"checkpoint", 4, 2, 2, 2}, {"threshold", 2, 2, 2, 0}, {"backlog", 1, 2, 2, 0}, {"off", 0, 2, 2, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newBoard(t)
			b.input.Repos = []domain.Repo{{ID: 1, FullName: "acme/app", HealthCheckEvery: tc.every, HealthCheckedThroughMergeSequence: tc.checkpoint, MergeSequence: 99, HealthCheckedAt: &now, HealthCheckedThroughTicket: ptr(int64(2))}, {ID: 2, FullName: "acme/idle", HealthCheckEvery: 8}}
			b.input.Tickets = []domain.Ticket{
				{ID: 1, RepoID: 1, Stage: domain.StageDone, HealthCheckMergeSequence: 2},
				{ID: 2, RepoID: 1, Stage: domain.StageDone, HealthCheckMergeSequence: 5},
				{ID: 3, RepoID: 1, Stage: domain.StageDone, HealthCheckMergeSequence: 9},
				{ID: 4, RepoID: 1, Stage: domain.StageReady, HealthCheckMergeSequence: 10},
				{ID: 5, RepoID: 1, Stage: domain.StageDone, CreatedAt: now.Add(time.Hour)},
				{ID: 60, RepoID: 1, Stage: domain.StageDone, IsHealthCheck: true, HealthCheckMergeSequence: 11, CreatedAt: now.Add(-time.Hour)},
				{ID: 7, RepoID: 1, Stage: domain.StageReady, IsHealthCheck: true, CreatedAt: now},
				{ID: 8, RepoID: 1, Stage: domain.StageDone, IsHealthCheck: true, CreatedAt: now},
				{ID: 9, RepoID: 2, Stage: domain.StageDone, HealthCheckMergeSequence: 1},
				{ID: 10, RepoID: 0, Stage: domain.StageReady},
				{ID: 11, RepoID: 77, Stage: domain.StageReady},
			}
			report := b.report(7)
			if len(report.Repos) != 4 {
				t.Fatalf("repositories: %+v", report.Repos)
			}
			for _, r := range report.Repos {
				if r.RepoID == 0 || r.RepoID == 77 {
					if r.Health != nil {
						t.Fatalf("unconfigured health: %+v", r)
					}
					continue
				}
				if r.Health == nil {
					t.Fatalf("missing health: %+v", r)
				}
				if r.RepoID != 1 {
					continue
				}
				h := r.Health
				if h.Every != tc.every || h.MergesSinceCheck != tc.want || h.LastCheckTicket == nil || *h.LastCheckTicket != 8 || h.LastCheckedAt == nil || !h.LastCheckedAt.Equal(now) || h.OpenFollowUps != 0 {
					t.Fatalf("health: %+v", h)
				}
				if tc.every == 0 {
					if h.NextIn != nil {
						t.Fatal("disabled next must be null")
					}
				} else if h.NextIn == nil || *h.NextIn != tc.next {
					t.Fatalf("next: %v", h.NextIn)
				}
			}
			other := b.reportIn(90, -300)
			for _, r := range report.Repos {
				for _, s := range other.Repos {
					if r.RepoID == s.RepoID && !reflect.DeepEqual(r.Health, s.Health) {
						t.Fatal("health depends on period")
					}
				}
			}
		})
	}
}

func TestHealthIdleAndPartialHistory(t *testing.T) {
	b := newBoard(t)
	b.input.Repos = []domain.Repo{{ID: 1, FullName: "idle", HealthCheckEvery: 8}, {ID: 2, FullName: "timestamp", HealthCheckedAt: &now}, {ID: 3, FullName: "ticket", HealthCheckEvery: 2}}
	b.input.Tickets = []domain.Ticket{{ID: 30, RepoID: 3, Stage: domain.StageDone, IsHealthCheck: true, CreatedAt: now.Add(-100 * day)}}
	r := b.report(7)
	if len(r.Repos) != 3 {
		t.Fatalf("idle repos omitted: %+v", r.Repos)
	}
	for _, repo := range r.Repos {
		h := repo.Health
		switch repo.RepoID {
		case 1:
			if h.LastCheckTicket != nil || h.LastCheckedAt != nil || h.NextIn == nil || *h.NextIn != 8 {
				t.Fatalf("idle: %+v", h)
			}
		case 2:
			if h.LastCheckTicket != nil || h.LastCheckedAt == nil {
				t.Fatalf("timestamp: %+v", h)
			}
		case 3:
			if h.LastCheckTicket == nil || *h.LastCheckTicket != 30 || h.LastCheckedAt != nil {
				t.Fatalf("ticket: %+v", h)
			}
		}
		if repo.Owner.Merged != 0 || repo.Autonomous.Merged != 0 {
			t.Fatal("delivery counters changed")
		}
	}
}

func ptr[T any](v T) *T { return &v }
