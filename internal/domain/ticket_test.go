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

func TestNewTicketStartsBeingRefinedWithoutRepo(t *testing.T) {
	ticket := mustTicket(t)
	if ticket.Lane != domain.LaneTodo || ticket.Stage != domain.StageRefine || ticket.RepoID != 0 || ticket.Title != "Dark mode" {
		t.Fatalf("got %s/%s repo %d title %q", ticket.Lane, ticket.Stage, ticket.RepoID, ticket.Title)
	}
	if ticket.Owner(0) != domain.OwnerAgent || ticket.Owner(1) != domain.OwnerHuman {
		t.Fatalf("owner without questions %q, with %q", ticket.Owner(0), ticket.Owner(1))
	}
}

func TestNewTicketRejectsBadInput(t *testing.T) {
	cases := map[string]func(*domain.TicketDraft){
		"blank title":    func(d *domain.TicketDraft) { d.Title = "   " },
		"long title":     func(d *domain.TicketDraft) { d.Title = strings.Repeat("x", domain.MaxTitleLength+1) },
		"unknown kind":   func(d *domain.TicketDraft) { d.Kind = "epic" },
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

func TestStageChangeDropsLeaseAndStampsLaneChange(t *testing.T) {
	ticket := mustTicket(t)
	if err := ticket.Claim("agent-a", time.Hour, now); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	later := now.Add(time.Minute)
	if !ticket.MoveToStage(domain.StageBuild, 5, later) {
		t.Fatal("MoveToStage reported no lane change")
	}
	if ticket.Lane != domain.LaneInProgress || ticket.Lease != nil || !ticket.LaneChangedAt.Equal(later) {
		t.Fatalf("lane %s lease %v lane changed at %v", ticket.Lane, ticket.Lease, ticket.LaneChangedAt)
	}
}

func TestStageChangeWithinLaneKeepsLaneChangedAt(t *testing.T) {
	ticket := mustTicket(t)
	if ticket.MoveToStage(domain.StageSpecReview, 9, now.Add(time.Hour)) {
		t.Fatal("same-lane stage change reported a lane change")
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
