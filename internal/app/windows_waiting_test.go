package app_test

import (
	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
	"testing"
)

func TestDisabledWindowsRunnerSkipsWaitingWorkUntilEnabled(t *testing.T) {
	f := settingsFixture(t)
	settingsHost(t, f, "windows", domain.BuildHostApproved, false, 3)
	repo := f.repo("acme/windows")
	id := f.ticket("Needs Windows", repo.ID)
	f.toBuild(id)
	f.mustClaim(agent, app.ClaimInput{})
	if _, err := f.agents.SetWorkModel(f.ctx, agent, id, "gpt-6.1-sol · awaiting Windows"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.agents.Release(f.ctx, agent, id); err != nil {
		t.Fatal(err)
	}
	if _, claimed := f.claim(other, app.ClaimInput{}); claimed {
		t.Fatal("disabled Windows work was reclaimed")
	}
	detail := f.detail(id)
	if detail.Ticket.NeedsOwner(detail.OpenQuestions) {
		t.Fatal("queued work became an owner blocker")
	}
	enabled := true
	if _, err := f.board.UpdateSettings(f.ctx, owner, domain.SettingsEdit{WindowsRunnerEnabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	if work, claimed := f.claim(other, app.ClaimInput{}); !claimed || work.Ticket.Ticket.ID != id {
		t.Fatal("enabling Windows did not resume work")
	}
}
