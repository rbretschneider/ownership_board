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

func TestTeamRosterShowsOnlyTicketsBeingWorked(t *testing.T) {
	// The board shows every ticket; the Team view shows what is being worked.
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	tickets := []app.TicketSummary{
		{Ticket: domain.Ticket{ID: 1, Lane: domain.LaneInProgress, Stage: domain.StagePR}},
		{Ticket: domain.Ticket{ID: 2, Lane: domain.LaneInProgress, Stage: domain.StageBuild, BlockedReason: "Needs the owner"}},
		{Ticket: domain.Ticket{ID: 3, Lane: domain.LaneTodo, Stage: domain.StageReady}, WaitingOn: []int64{1}},
		{Ticket: domain.Ticket{ID: 4, Lane: domain.LaneInProgress, Stage: domain.StageBuild}, Build: &domain.BuildJob{Status: "running"}},
	}
	_, inFlight := app.TeamRoster(tickets, nil, now)
	if len(inFlight) != 1 || inFlight[0].Ticket.ID != 4 {
		t.Fatalf("in flight = %+v, want only #4", inFlight)
	}
}

func TestManagerActivityIsReportedByTheManagerOnlyAndExpires(t *testing.T) {
	f := newFixture(t)
	if _, err := f.board.ReportManagerActivity(f.ctx, owner, app.ManagerActivityInput{State: "idle"}); err == nil {
		t.Fatal("a person reported manager activity")
	}
	for _, bad := range []app.ManagerActivityInput{{State: "busy"}, {State: "working", Kind: "nap"},
		{State: "working", Kind: "flag", TicketIDs: []int64{-1}}, {State: "working", Kind: "flag", Host: "bad host"}} {
		if _, err := f.board.ReportManagerActivity(f.ctx, manager, bad); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
	got, err := f.board.ReportManagerActivity(f.ctx, manager, app.ManagerActivityInput{State: "working", Kind: "flag", TicketIDs: []int64{7, 7, 9}, Host: "builder-1"})
	if err != nil || len(got.TicketIDs) != 2 || got.Host != "builder-1" {
		t.Fatalf("working = %+v, %v", got, err)
	}
	team, err := f.board.Team(f.ctx)
	if err != nil || team.Manager.Activity.State != domain.ManagerWorking || len(team.Manager.Activity.TicketIDs) != 2 {
		t.Fatalf("team activity = %+v, %v", team.Manager.Activity, err)
	}
	f.clock.Advance(4 * time.Hour)
	if team, _ = f.board.Team(f.ctx); team.Manager.Activity.State != domain.ManagerIdle {
		t.Fatalf("stale working report still shown: %+v", team.Manager.Activity)
	}
	if _, err := f.board.ReportManagerActivity(f.ctx, manager, app.ManagerActivityInput{State: "idle"}); err != nil {
		t.Fatal(err)
	}
}
