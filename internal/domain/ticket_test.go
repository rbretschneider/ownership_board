package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

var (
	now   = time.Date(2026, 1, 2, 9, 0, 0, 0, time.UTC)
	human = domain.Actor{Kind: domain.ActorHuman, ID: 1, Name: "Owner"}
)

func validDraft() domain.TicketDraft {
	return domain.TicketDraft{Kind: "story", Title: "  Dark mode  ", Body: "please"}
}

func mustTicket(t *testing.T) domain.Ticket {
	t.Helper()
	ticket, err := domain.NewTicket(validDraft(), human, now)
	if err != nil {
		t.Fatalf("NewTicket: %v", err)
	}
	return ticket
}

func requireKind(t *testing.T, err error, want domain.ErrorKind) {
	t.Helper()
	if got := domain.KindOf(err); got != want {
		t.Fatalf("error kind = %v (%v), want %v", got, err, want)
	}
}

func TestNewTicketDefaultsToNewLaneWithoutRepo(t *testing.T) {
	ticket := mustTicket(t)
	if ticket.Lane != domain.LaneNew || ticket.RepoID != 0 || ticket.Title != "Dark mode" {
		t.Fatalf("got lane %q repo %d title %q", ticket.Lane, ticket.RepoID, ticket.Title)
	}
	if ticket.Owner() != domain.OwnerAgent {
		t.Fatalf("owner = %q, want agent", ticket.Owner())
	}
}

func TestNewTicketRejectsBadInput(t *testing.T) {
	cases := map[string]func(*domain.TicketDraft){
		"blank title":    func(d *domain.TicketDraft) { d.Title = "   " },
		"long title":     func(d *domain.TicketDraft) { d.Title = strings.Repeat("x", domain.MaxTitleLength+1) },
		"unknown kind":   func(d *domain.TicketDraft) { d.Kind = "epic" },
		"unknown lane":   func(d *domain.TicketDraft) { d.Lane = "backlog" },
		"negative repo":  func(d *domain.TicketDraft) { d.RepoID = -1 },
		"oversized body": func(d *domain.TicketDraft) { d.Body = strings.Repeat("x", domain.MaxBodyLength+1) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			draft := validDraft()
			mutate(&draft)
			_, err := domain.NewTicket(draft, human, now)
			requireKind(t, err, domain.KindInvalid)
		})
	}
}

func TestMoveToDropsLeaseAndStampsLaneChange(t *testing.T) {
	ticket := mustTicket(t)
	if err := ticket.Claim("agent-a", time.Hour, now); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	later := now.Add(time.Minute)
	if !ticket.MoveTo(domain.LaneReady, 5, later) {
		t.Fatal("MoveTo reported no lane change")
	}
	if ticket.Lease != nil || !ticket.LaneChangedAt.Equal(later) {
		t.Fatalf("lease %v lane changed at %v", ticket.Lease, ticket.LaneChangedAt)
	}
}

func TestReorderWithinLaneKeepsLaneChangedAt(t *testing.T) {
	ticket := mustTicket(t)
	if ticket.MoveTo(domain.LaneNew, 9, now.Add(time.Hour)) {
		t.Fatal("reorder reported a lane change")
	}
	if !ticket.LaneChangedAt.Equal(now) {
		t.Fatalf("lane changed at moved to %v", ticket.LaneChangedAt)
	}
}

func TestClaimConflictsWhileAnotherAgentHoldsTheLease(t *testing.T) {
	ticket := mustTicket(t)
	if err := ticket.Claim("agent-a", time.Hour, now); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	requireKind(t, ticket.Claim("agent-b", time.Hour, now.Add(time.Minute)), domain.KindConflict)
	if err := ticket.Claim("agent-b", time.Hour, now.Add(2*time.Hour)); err != nil {
		t.Fatalf("claim after expiry: %v", err)
	}
}

func TestApplyReportsOnlyChangedFields(t *testing.T) {
	ticket := mustTicket(t)
	sameTitle, prURL := "Dark mode", "https://example.com/pr/1"
	changed, err := ticket.Apply(domain.TicketEdit{Title: &sameTitle, PRURL: &prURL}, now)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(changed) != 1 || changed[0] != "pr_url" || ticket.PRURL != prURL {
		t.Fatalf("changed %v, pr %q", changed, ticket.PRURL)
	}
}

func TestApplyRejectsScriptURLAndLeavesTicketUntouched(t *testing.T) {
	ticket := mustTicket(t)
	title, bad := "New title", "javascript:alert(1)"
	_, err := ticket.Apply(domain.TicketEdit{Title: &title, PRURL: &bad}, now)
	requireKind(t, err, domain.KindInvalid)
	if ticket.Title != "Dark mode" {
		t.Fatalf("title changed to %q despite the failed edit", ticket.Title)
	}
}
