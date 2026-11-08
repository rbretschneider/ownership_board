package app_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestRegisteredRunnerProjection(t *testing.T) {
	f, _, _, h := jobFixture(t)
	s := app.NewBuildHostService(f.store, f.clock, nil, nil)
	for _, actor := range []domain.Actor{agent, domain.SystemActor, {}, {Kind: domain.ActorHuman}} {
		_, e := s.Runners(f.ctx, actor)
		requireErrorKind(t, e, domain.KindForbidden)
		_, e = s.RecentEvents(f.ctx, actor, h.ID, 20)
		requireErrorKind(t, e, domain.KindForbidden)
	}
	// Queue counts exclude all terminal and running states. Unreleased terminal
	// assignments still occupy capacity and must remain visible to the owner.
	statuses := []string{"queued", "running", "passed", "failed", "superseded", "cancelled"}
	for i, status := range statuses {
		var host any
		if status != "queued" {
			host = h.ID
		}
		var released any
		if status == "passed" || status == "failed" {
			released = "2026-01-01T00:01:00.000Z"
		}
		_, e := f.store.SQL().Exec(`INSERT INTO build_jobs(id,ticket_id,repo,sha,preset,target,target_os,status,host_id,assignment_released_at,attempt,created_at,started_at,bundle_bytes,bundle_sha256) VALUES(?,1,'example/tools','abcdef','cmake','{}','linux',?,?,?,1,'2026-01-01T00:00:00.000Z','2026-01-01T00:00:01.000Z',3,?)`, fmt.Sprintf("%064x", i+1), status, host, released, fmt.Sprintf("%064x", 0))
		if e != nil {
			t.Fatal(e)
		}
	}
	snapshot, e := s.Runners(f.ctx, owner)
	if e != nil {
		t.Fatal(e)
	}
	if !snapshot.ObservedAt.Equal(f.clock.Now()) || len(snapshot.Hosts) != 1 || snapshot.QueuedByOS["linux"] != 1 || snapshot.QueuedByOS["windows"] != 0 || snapshot.QueuedByOS["macos"] != 0 {
		t.Fatal(snapshot)
	}
	jobs := snapshot.Hosts[0].Jobs
	if len(jobs) != 3 || jobs[0].Status != "running" || jobs[1].Status != "superseded" || jobs[2].Status != "cancelled" {
		t.Fatal(jobs)
	}
	disabled := false
	if _, e = s.Update(f.ctx, owner, h.ID, domain.BuildHostEdit{Enabled: &disabled}); e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.SQL().Exec(`UPDATE build_hosts SET last_seen_at=? WHERE id=?`, f.clock.Now().Format("2006-01-02T15:04:05.000Z"), h.ID); e != nil {
		t.Fatal(e)
	}
	snapshot, e = s.Runners(f.ctx, owner)
	if e != nil {
		t.Fatal(e)
	}
	row := snapshot.Hosts[0]
	if row.Host.Enabled || !row.Host.Online(snapshot.ObservedAt) {
		t.Fatal(row)
	}
	f.clock.Advance(3*time.Minute + time.Millisecond)
	snapshot, e = s.Runners(f.ctx, owner)
	if e != nil || snapshot.Hosts[0].Host.Online(snapshot.ObservedAt) {
		t.Fatal(snapshot, e)
	}
	if _, e = s.Revoke(f.ctx, owner, h.ID); e != nil {
		t.Fatal(e)
	}
	snapshot, e = s.Runners(f.ctx, owner)
	if e != nil || len(snapshot.Hosts) != 0 {
		t.Fatal(snapshot, e)
	}
}

func TestRecentBuildHostEvents(t *testing.T) {
	f, _, _, h := jobFixture(t)
	s := app.NewBuildHostService(f.store, f.clock, nil, nil)
	if e := f.store.Write(f.ctx, func(r app.Repos) error {
		for i := 0; i < 25; i++ {
			if e := r.BuildHostEvents().Append(f.ctx, &domain.BuildHostEvent{HostID: h.ID, Type: "toolchains_changed", Data: []byte(`{}`), CreatedAt: f.clock.Now()}); e != nil {
				return e
			}
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	recent, e := s.RecentEvents(f.ctx, owner, h.ID, 20)
	if e != nil || len(recent) != 20 || recent[0].ID != 25 || recent[19].ID != 6 {
		t.Fatal(recent, e)
	}
	forward, e := s.Events(f.ctx, owner, h.ID, 20, 20)
	if e != nil || len(forward) != 5 || forward[0].ID != 21 {
		t.Fatal(forward, e)
	}
	for _, limit := range []int{0, -1, 201} {
		_, e = s.RecentEvents(f.ctx, owner, h.ID, limit)
		requireErrorKind(t, e, domain.KindInvalid)
	}
	_, e = s.RecentEvents(f.ctx, owner, 999, 20)
	requireErrorKind(t, e, domain.KindNotFound)
}
