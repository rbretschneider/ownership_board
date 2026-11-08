package app_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestStatsHealthCountsLatestDirectOpenFollowUpsWithoutMutation(t *testing.T) {
	f := newFixture(t)
	repo := f.repo("acme/app")
	idle := f.repo("acme/idle")
	f.healthInterval(repo.ID, 1)
	f.move(owner, f.ticket("delivery", repo.ID), domain.LaneMerged, nil)
	old := f.checks(repo.ID)[0]
	f.move(owner, old.ID, domain.LaneMerged, nil)
	f.clock.Advance(time.Second)
	f.move(owner, f.ticket("next delivery", repo.ID), domain.LaneMerged, nil)
	var latest domain.Ticket
	for _, c := range f.checks(repo.ID) {
		if c.ID != old.ID {
			latest = c
		}
	}
	link := func(from int64, kind domain.RelationKind, to int64) {
		t.Helper()
		if _, err := f.board.LinkTickets(f.ctx, owner, from, string(kind), to); err != nil {
			t.Fatal(err)
		}
	}
	open := f.ticket("open", repo.ID)
	link(open, domain.RelationFollowUpOf, latest.ID)
	if _, err := f.board.Block(f.ctx, owner, open, "waiting for a dependency"); err != nil {
		t.Fatal(err)
	}
	done := f.ticket("closed", repo.ID)
	link(done, domain.RelationFollowUpOf, latest.ID)
	f.move(owner, done, domain.LaneDeployed, nil)
	link(f.ticket("old follow up", repo.ID), domain.RelationFollowUpOf, old.ID)
	link(f.ticket("other kind", repo.ID), domain.RelationRelatesTo, latest.ID)
	link(latest.ID, domain.RelationFollowUpOf, f.ticket("reverse", repo.ID))
	link(f.ticket("indirect", repo.ID), domain.RelationFollowUpOf, open)
	cross := f.ticket("cross repo follow up", idle.ID)
	link(cross, domain.RelationFollowUpOf, latest.ID)
	link(cross, domain.RelationDependsOn, f.ticket("external prerequisite", idle.ID))
	if len(f.waitingOn(cross)) != 1 {
		t.Fatal("follow-up is not waiting")
	}
	before := f.detail(latest.ID)
	countBefore := f.ticketCount()
	eventsBefore := len(f.events.events)
	report, err := f.board.Stats(f.ctx, app.StatsQuery{Days: 7})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range report.Repos {
		if r.Health == nil {
			t.Fatal("missing health")
		}
		if r.RepoID == repo.ID {
			h := r.Health
			if h.Every != 1 || h.MergesSinceCheck != 1 || h.NextIn == nil || *h.NextIn != 0 || h.LastCheckTicket == nil || *h.LastCheckTicket != latest.ID || h.LastCheckedAt == nil || h.OpenFollowUps != 2 {
				t.Fatalf("health: %+v", h)
			}
		}
		if r.RepoID == idle.ID && (r.Health.LastCheckTicket != nil || r.Health.OpenFollowUps != 0) {
			t.Fatalf("idle: %+v", r.Health)
		}
	}
	if !reflect.DeepEqual(before, f.detail(latest.ID)) || countBefore != f.ticketCount() || eventsBefore != len(f.events.events) {
		t.Fatal("stats mutated ticket or checkpoint")
	}
	f.healthInterval(repo.ID, 0)
	report, err = f.board.Stats(f.ctx, app.StatsQuery{Days: 90, OffsetMinutes: 300})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range report.Repos {
		if r.RepoID == repo.ID && (r.Health.NextIn != nil || r.Health.OpenFollowUps != 2 || *r.Health.LastCheckTicket != latest.ID) {
			t.Fatalf("disabled: %+v", r.Health)
		}
	}
}
