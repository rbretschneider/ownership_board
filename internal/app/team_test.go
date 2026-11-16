package app_test

import (
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func modelSignal(ticketID int64, worker, model string, at time.Time) domain.TimelineEntry {
	entry := domain.NewEntry(ticketID, domain.TimelineEdited, domain.Actor{Kind: domain.ActorAgent, Name: worker}, at)
	return entry.With("model", model)
}

func TestTeamRosterDerivesRunnersFromLeasesAndWorkModels(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	live := &domain.Lease{Holder: "@worker/7/codex_1", ExpiresAt: now.Add(10 * time.Minute)}
	held := &domain.Lease{Holder: "@worker/7/claude_1", ExpiresAt: now.Add(10 * time.Minute)}
	expired := &domain.Lease{Holder: "@worker/7/gone", ExpiresAt: now.Add(-time.Minute)}
	queued := &domain.BuildJob{Status: "queued", CreatedAt: now.Add(-time.Hour)}
	tickets := []app.TicketSummary{
		{Ticket: domain.Ticket{ID: 1, Stage: domain.StageBuild, Lease: live, WorkModel: "gpt-test · repairing the release workflow"}},
		{Ticket: domain.Ticket{ID: 2, Stage: domain.StageBuild, WorkModel: "opus-test · build-server:queued:Windows x64 · awaiting Windows"}, Build: queued},
		{Ticket: domain.Ticket{ID: 3, Stage: domain.StageBuild, Lease: held, WorkModel: "sonnet-test · awaiting verification"}},
		{Ticket: domain.Ticket{ID: 4, Stage: domain.StageDesign, Lease: expired}},
		{Ticket: domain.Ticket{ID: 5, Stage: domain.StageDone, Lease: live}},
	}
	signals := []domain.TimelineEntry{
		modelSignal(1, "@worker/7/codex_1", "gpt-test · repairing the release workflow", now.Add(-5*time.Minute)),
		modelSignal(2, "@worker/7/claude_2", "opus-test · build-server:queued:Windows x64 · awaiting Windows", now.Add(-30*time.Minute)),
		modelSignal(3, "@worker/7/claude_1", "sonnet-test · awaiting verification", now.Add(-2*time.Minute)),
		modelSignal(4, "@worker/7/codex_2", "gpt-test · designing", now.Add(-2*time.Hour)),
		modelSignal(4, "@worker/7/stale", "gpt-test · designing", now.Add(-48*time.Hour)),
		domain.NewEntry(4, domain.TimelineEdited, owner, now).With("model", "person"),
	}
	runners, inFlight := app.TeamRoster(tickets, signals, now)

	want := []struct {
		name   string
		state  app.RunnerState
		ticket int64
		model  string
	}{
		{"@worker/7/codex_1", app.RunnerWorking, 1, "gpt-test"},
		{"@worker/7/claude_1", app.RunnerWaiting, 3, "sonnet-test"},
		{"@worker/7/claude_2", app.RunnerWaiting, 2, "opus-test"},
		{"@worker/7/codex_2", app.RunnerIdle, 0, "gpt-test"},
	}
	if len(runners) != len(want) {
		t.Fatalf("runners = %+v, want %d", runners, len(want))
	}
	for i, w := range want {
		got := runners[i]
		if got.Name != w.name || got.State != w.state || got.TicketID != w.ticket || got.Model != w.model {
			t.Errorf("runner %d = %+v, want %+v", i, got, w)
		}
	}
	if runners[0].Signal != "repairing the release workflow" {
		t.Errorf("signal = %q", runners[0].Signal)
	}
	ids := map[int64]string{}
	for _, ticket := range inFlight {
		ids[ticket.Ticket.ID] = ticket.Holder
	}
	if len(ids) != 3 || ids[1] != "@worker/7/codex_1" || ids[2] != "@worker/7/claude_2" || ids[3] != "@worker/7/claude_1" {
		t.Fatalf("in flight = %v", ids)
	}
}

func TestTeamRosterIsEmptyOnAQuietBoard(t *testing.T) {
	runners, inFlight := app.TeamRoster(nil, nil, time.Now())
	if len(runners) != 0 || inFlight == nil || len(inFlight) != 0 {
		t.Fatalf("runners %v, in flight %v", runners, inFlight)
	}
}
