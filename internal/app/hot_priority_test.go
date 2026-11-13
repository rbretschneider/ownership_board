package app_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

// kickBack records n more returns to the code (entries into the build stage from PR).
func (f *fixture) kickBack(id int64, n int) {
	f.t.Helper()
	if err := f.store.Write(f.ctx, func(r app.Repos) error {
		for i := 0; i < n; i++ {
			entry := domain.NewEntry(id, domain.TimelineStage, agent, f.clock.Now())
			entry.Data = map[string]any{"from": string(domain.StagePR), "to": string(domain.StageBuild)}
			if err := r.Timeline().Append(f.ctx, &entry); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		f.t.Fatal(err)
	}
}

// 2026-11-13 (owner): a ticket at double-digit kickbacks gets priority to merge.
func TestAHotBuildIsClaimedBeforeHigherRankedBuilds(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	first, second, hot := f.ticket("first", api.ID), f.ticket("second", api.ID), f.ticket("looping", api.ID)
	for _, id := range []int64{first, second, hot} {
		f.toBuild(id)
	}
	f.kickBack(second, app.HotBuildAttempts-2) // one short, with its own entry into build
	f.kickBack(hot, app.HotBuildAttempts-1)
	// The hot ticket first, then the others in rank order, whatever the hot one's rank.
	want := []int64{hot}
	for _, id := range f.laneOrder(domain.LaneInProgress) {
		if id != hot {
			want = append(want, id)
		}
	}
	if order := f.laneOrder(domain.LaneInProgress); order[0] == hot {
		f.move(owner, hot, domain.LaneInProgress, &order[len(order)-1]) // rank it last
	}
	for _, id := range want {
		if got := f.mustClaim(agent, app.ClaimInput{}).Ticket.Ticket.ID; got != id {
			t.Fatalf("claimed #%d, want #%d (order %v)", got, id, want)
		}
	}
}

func TestAHotTicketsWindowsJobRunsFirst(t *testing.T) {
	f, s, m, h := jobFixture(t)
	if _, err := f.store.SQL().Exec(`INSERT INTO tickets(id,repo_id,kind,title,stage,lane,rank,created_by,created_at,updated_at,lane_changed_at) VALUES(2,1,'story','Looping','build','in_progress',2,'Owner','2026-01-01T00:00:00.000Z','2026-01-01T00:00:00.000Z','2026-01-01T00:00:00.000Z')`); err != nil {
		t.Fatal(err)
	}
	older := submitJob(t, f, s, m)
	m.TicketID, m.SHA = 2, fmt.Sprintf("%040x", 2)
	hot, _, err := s.Submit(f.ctx, m, bytes.NewBufferString("abc"), func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	f.kickBack(2, app.HotBuildAttempts)
	got, err := s.Claim(f.ctx, app.AuthenticatedBuildHost{ID: h.ID})
	if err != nil || got == nil || got.ID != hot.ID {
		t.Fatalf("claimed %+v (%v), want the hot ticket's job %s before %s", got, err, hot.ID, older.ID)
	}
}
