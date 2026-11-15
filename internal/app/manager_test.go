package app_test

import (
	"testing"

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
	_, err = f.board.RecordManagerUsage(f.ctx, agent, 1)
	requireErrorKind(t, err, domain.KindForbidden)
}

func TestTheManagerRecordsItsSpendAgainstTheDailyBudget(t *testing.T) {
	f := newFixture(t)
	status, err := f.board.RecordManagerUsage(f.ctx, manager, 1.25)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	status, err = f.board.RecordManagerUsage(f.ctx, manager, 0.75)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if status.Today.Sessions != 2 || status.Today.CostUSD != 2 || status.RemainingUSD != float64(domain.DefaultManagerDailyBudgetUSD)-2 {
		t.Fatalf("status = %+v, want 2 sessions, $2 spent, $%d-2 left", status, domain.DefaultManagerDailyBudgetUSD)
	}
}
