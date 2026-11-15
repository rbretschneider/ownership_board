package app_test

import (
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

var manager = domain.Actor{Kind: domain.ActorAgent, ID: 77, Name: "board-manager", IsManager: true}

func TestTheManagerKeepsAutonomousTicketsMovingWithoutALease(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	auto, err := f.board.Create(f.ctx, owner, domain.TicketDraft{Kind: "story", Title: "autopilot", RepoID: api.ID, IsAutonomous: true})
	if err != nil {
		t.Fatalf("create autonomous: %v", err)
	}
	id := auto.Ticket.ID
	if _, err := f.board.Block(f.ctx, manager, id, "loops on the same CI failure"); err != nil {
		t.Fatalf("manager parks an autonomous ticket: %v", err)
	}
	flags, err := f.board.ManagerFlags(f.ctx, manager, app.RepoScope{})
	if err != nil {
		t.Fatalf("flags: %v", err)
	}
	if len(flags) != 1 || flags[0].TicketID != id || flags[0].Kind != domain.FlagEscalated {
		t.Fatalf("flags = %+v, want one escalated flag for #%d", flags, id)
	}
	if _, err := f.board.Comment(f.ctx, manager, id, "Direction: fix the parser, keep every test."); err != nil {
		t.Fatalf("manager comments without a lease: %v", err)
	}
	if _, err := f.board.Unblock(f.ctx, manager, id, "cause fixed on main"); err != nil {
		t.Fatalf("manager unblocks: %v", err)
	}
	if flags, _ := f.board.ManagerFlags(f.ctx, manager, app.RepoScope{}); len(flags) != 0 {
		t.Fatalf("flags after unblock = %+v, want none", flags)
	}
}

func TestTheManagerMakesAStoryWaitWithoutAskingTheOwner(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	fix, err := f.board.Create(f.ctx, owner, domain.TicketDraft{Kind: "bug", Title: "harness fix", RepoID: api.ID, IsAutonomous: true})
	if err != nil {
		t.Fatalf("create fix: %v", err)
	}
	story, err := f.board.Create(f.ctx, owner, domain.TicketDraft{Kind: "story", Title: "story", RepoID: api.ID, IsAutonomous: true})
	if err != nil {
		t.Fatalf("create story: %v", err)
	}
	detail, err := f.board.LinkTickets(f.ctx, manager, story.Ticket.ID, "depends_on", fix.Ticket.ID)
	if err != nil {
		t.Fatalf("manager links the story to its fix: %v", err)
	}
	if detail.Ticket.NeedsOwner(detail.OpenQuestions) || len(detail.WaitingOn) != 1 {
		t.Fatalf("needs owner=%v waiting on=%v, want the story waiting on the fix without the owner", detail.Ticket.NeedsOwner(detail.OpenQuestions), detail.WaitingOn)
	}
}

func TestTheManagerHasNoAuthorityOverOrdinaryTickets(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	plain, err := f.board.Create(f.ctx, owner, domain.TicketDraft{Kind: "story", Title: "owner reviews this", RepoID: api.ID})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = f.board.Block(f.ctx, manager, plain.Ticket.ID, "nope")
	requireErrorKind(t, err, domain.KindForbidden)
	_, err = f.board.Comment(f.ctx, manager, plain.Ticket.ID, "nope")
	requireErrorKind(t, err, domain.KindForbidden)
}

func TestOrdinaryAgentsStillCannotAnswerUnblockOrReadManagerFlags(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	auto, err := f.board.Create(f.ctx, owner, domain.TicketDraft{Kind: "story", Title: "autopilot", RepoID: api.ID, IsAutonomous: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = f.board.Unblock(f.ctx, agent, auto.Ticket.ID, "")
	requireErrorKind(t, err, domain.KindForbidden)
	choice := 0
	_, err = f.board.Answer(f.ctx, agent, auto.Ticket.ID, []domain.AnswerDraft{{QuestionID: 1, Choice: &choice}})
	requireErrorKind(t, err, domain.KindForbidden)
	_, err = f.board.ManagerFlags(f.ctx, agent, app.RepoScope{})
	requireErrorKind(t, err, domain.KindForbidden)
	_, err = f.board.RecordManagerUsage(f.ctx, agent, app.ManagerSessionInput{CostUSD: 1})
	requireErrorKind(t, err, domain.KindForbidden)
}

func TestTheManagerRecordsItsSpendAgainstTheDailyBudget(t *testing.T) {
	f := newFixture(t)
	status, err := f.board.RecordManagerUsage(f.ctx, manager, app.ManagerSessionInput{CostUSD: 1.25, Model: "sonnet", Summary: "MANAGER SUMMARY: #49 cycling, direction given"})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	status, err = f.board.RecordManagerUsage(f.ctx, manager, app.ManagerSessionInput{CostUSD: 0.75, Model: "opus"})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if status.Today.Sessions != 2 || status.Today.CostUSD != 2 || status.RemainingUSD != float64(domain.DefaultManagerDailyBudgetUSD)-2 {
		t.Fatalf("status = %+v, want 2 sessions, $2 spent, $%d-2 left", status, domain.DefaultManagerDailyBudgetUSD)
	}
}

func TestTheOwnerReadsTheManagersSessionsNewestFirst(t *testing.T) {
	f := newFixture(t)
	if _, err := f.board.RecordManagerUsage(f.ctx, manager, app.ManagerSessionInput{CostUSD: 0.25, Model: "sonnet", Summary: "first"}); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Minute)
	if _, err := f.board.RecordManagerUsage(f.ctx, manager, app.ManagerSessionInput{CostUSD: 0.5, Model: "opus", Summary: "second"}); err != nil {
		t.Fatal(err)
	}
	sessions, err := f.board.ManagerSessions(f.ctx, owner, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 || sessions[0].Summary != "second" || sessions[0].Model != "opus" || sessions[1].Summary != "first" {
		t.Fatalf("sessions = %+v, want second then first", sessions)
	}
	_, err = f.board.ManagerSessions(f.ctx, agent, 10)
	requireErrorKind(t, err, domain.KindForbidden)
}
