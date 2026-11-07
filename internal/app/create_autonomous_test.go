package app_test

import (
	"strings"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestOwnerStartsATicketOnAutopilot(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	detail, err := f.board.Create(f.ctx, owner, domain.TicketDraft{Kind: "story", Title: "fast lane", RepoID: api.ID, IsAutonomous: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := detail.Ticket.ID
	if !detail.Ticket.IsAutonomous || detail.Ticket.Stage != domain.StageRefine {
		t.Fatalf("autonomous=%v stage=%s, want an autonomous ticket being refined", detail.Ticket.IsAutonomous, detail.Ticket.Stage)
	}
	brief := f.mustClaim(agent, app.ClaimInput{}).Brief
	for _, want := range []string{"Turned autonomous mode on", "## Autonomous ticket", "started the ticket on autopilot"} {
		if !strings.Contains(brief, want) {
			t.Errorf("refine brief is missing %q:\n%s", want, brief)
		}
	}
	if _, err := f.board.Ask(f.ctx, agent, id, ask("Mobile too?")); err == nil {
		t.Fatal("an autopilot ticket took a clarifying question")
	}
	// No spec checkpoint: the submitted spec goes straight to Ready.
	if _, err := f.board.SaveSpec(f.ctx, agent, id, app.SaveTextInput{Text: "Spec.", IsSubmit: true}); err != nil {
		t.Fatalf("submit spec: %v", err)
	}
	f.requireStage(id, domain.StageReady)
}

func TestAutopilotAtCreationNeedsAPersonAndARepo(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	_, err := f.board.Create(f.ctx, agent, domain.TicketDraft{Kind: "story", Title: "sneaky", RepoID: api.ID, IsAutonomous: true})
	requireErrorKind(t, err, domain.KindForbidden)
	_, err = f.board.Create(f.ctx, owner, domain.TicketDraft{Kind: "story", Title: "where?", IsAutonomous: true})
	requireErrorKind(t, err, domain.KindInvalid)
}
