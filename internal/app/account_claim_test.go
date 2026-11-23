package app_test

import (
	"testing"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

// 2026-11-23, Ryan: Claude only writes code, but art generation must be done
// by GPT. A ticket pinned to an account is claimed only by an agent on it.
func TestAPinnedTicketGoesOnlyToAnAgentOnItsAccount(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/game")
	art, code := f.ticket("Hand-painted textures", api.ID), f.ticket("Shader cleanup", api.ID)
	codex := "codex"
	if _, err := f.board.Edit(f.ctx, owner, art, domain.TicketEdit{Account: &codex}); err != nil {
		t.Fatalf("pin: %v", err)
	}
	for _, id := range []int64{art, code} {
		f.toReady(id)
	}
	// Each move lands on top of In Progress, so the art ticket is first in line.
	f.toBuildWith(code, "Change src/render/Shader.cpp.")
	f.toBuildWith(art, "Add assets/textures/bark.png.")

	if _, isClaimed := f.claim(agent, app.ClaimInput{Repo: "acme/game", Account: "claude"}); !isClaimed {
		t.Fatal("a Claude agent got nothing, want the unpinned ticket")
	} else if got := f.detail(code).Ticket.Lease; got == nil {
		t.Fatalf("a Claude agent must skip the Codex-only ticket and take #%d", code)
	}
	if _, isClaimed := f.claim(other, app.ClaimInput{Repo: "acme/game"}); isClaimed {
		t.Fatal("an agent that names no account must not take a pinned ticket")
	}
	work := f.mustClaim(other, app.ClaimInput{Repo: "acme/game", Account: "codex"})
	if work.Ticket.Ticket.ID != art {
		t.Fatalf("a Codex agent claimed #%d, want the Codex-only ticket #%d", work.Ticket.Ticket.ID, art)
	}
	if detail := f.detail(art); detail.Ticket.Account != domain.AccountCodex {
		t.Fatalf("account = %q, want codex", detail.Ticket.Account)
	}
}

func TestAnAccountPinMustBeKnown(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/game")
	id := f.ticket("Textures", api.ID)
	bad := "gemini"
	if _, err := f.board.Edit(f.ctx, owner, id, domain.TicketEdit{Account: &bad}); domain.KindOf(err) != domain.KindInvalid {
		t.Fatalf("unknown account: %v, want invalid", err)
	}
	if _, _, err := f.agents.Claim(f.ctx, agent, app.ClaimInput{Account: "gemini"}); domain.KindOf(err) != domain.KindInvalid {
		t.Fatalf("claim with unknown account: %v, want invalid", err)
	}
}

// The manager pins a queued art ticket; once work starts the pin is fixed.
func TestTheManagerMayPinOnlyUntilWorkStarts(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/game")
	started := f.autonomousInBuild(api.ID)
	queued := f.autonomous("Icons", api.ID)
	codex := "codex"
	if _, err := f.board.Edit(f.ctx, manager, queued, domain.TicketEdit{Account: &codex}); err != nil {
		t.Fatalf("pin a Ready ticket: %v", err)
	}
	if _, err := f.board.Edit(f.ctx, manager, started, domain.TicketEdit{Account: &codex}); domain.KindOf(err) != domain.KindForbidden {
		t.Fatalf("pin a ticket in build: %v, want forbidden", err)
	}
}
