package app_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestClaimPrefersBuildingThenDesignThenRefinementThenReady(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	ready := f.ticket("ready", api.ID)
	f.toReady(ready)
	designing, refine, building := f.ticket("designing", api.ID), f.ticket("refine", 0), f.ticket("building", api.ID)
	f.move(owner, designing, domain.LaneInProgress, nil)
	f.toBuild(building)
	wantOrder := []struct {
		id     int64
		action domain.WorkAction
	}{{building, domain.ActionImplement}, {designing, domain.ActionDesign}, {refine, domain.ActionRefine}, {ready, domain.ActionDesign}}
	for _, want := range wantOrder {
		work := f.mustClaim(agent, app.ClaimInput{})
		if work.Ticket.Ticket.ID != want.id || work.Action != want.action {
			t.Fatalf("claimed #%d (%s), want #%d (%s)", work.Ticket.Ticket.ID, work.Action, want.id, want.action)
		}
	}
	if _, isClaimed := f.claim(agent, app.ClaimInput{}); isClaimed {
		t.Fatal("claimed a leased ticket")
	}
}

func TestClaimingReadyWorkStartsTheDesign(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	id := f.ticket("build me", api.ID)
	f.toReady(id)
	work := f.mustClaim(agent, app.ClaimInput{})
	ticket := work.Ticket.Ticket
	if ticket.Lane != domain.LaneInProgress || ticket.Stage != domain.StageDesign || ticket.Lease.Holder != agent.Name {
		t.Fatalf("%s/%s lease %+v", ticket.Lane, ticket.Stage, ticket.Lease)
	}
}

func TestOwnerReviewStagesAreNeverClaimed(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	id := f.ticket("spec me", api.ID)
	f.mustClaim(agent, app.ClaimInput{})
	if _, err := f.board.SaveSpec(f.ctx, agent, id, app.SaveTextInput{Text: "Spec.", IsSubmit: true}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	f.requireStage(id, domain.StageSpecReview)
	if _, isClaimed := f.claim(other, app.ClaimInput{}); isClaimed {
		t.Fatal("an agent claimed a spec waiting for the owner")
	}
}

func TestExpiredLeaseCanBeReclaimedByAnotherAgent(t *testing.T) {
	f := newFixture(t)
	id := f.ticket("idea", 0)
	f.mustClaim(agent, app.ClaimInput{LeaseMinutes: 10})
	if _, isClaimed := f.claim(other, app.ClaimInput{}); isClaimed {
		t.Fatal("claimed while the lease was live")
	}
	f.clock.Advance(11 * time.Minute)
	if work := f.mustClaim(other, app.ClaimInput{}); work.Ticket.Ticket.ID != id {
		t.Fatalf("reclaimed #%d", work.Ticket.Ticket.ID)
	}
}

func TestRepoScopedClaimTakesUnassignedTicketsOnlyForRefinement(t *testing.T) {
	f := newFixture(t)
	api, web := f.repo("acme/api"), f.repo("acme/web")
	webTicket := f.ticket("web work", web.ID)
	f.toReady(webTicket)
	loose := f.ticket("loose design", 0)
	f.move(owner, loose, domain.LaneInProgress, nil)
	if _, isClaimed := f.claim(agent, app.ClaimInput{Repo: api.FullName}); isClaimed {
		t.Fatal("an acme/api agent claimed acme/web work or unassigned design work")
	}
	unrefined := f.ticket("loose idea", 0)
	if work := f.mustClaim(agent, app.ClaimInput{Repo: api.FullName}); work.Ticket.Ticket.ID != unrefined {
		t.Fatalf("claimed #%d, want the unassigned ticket being refined #%d", work.Ticket.Ticket.ID, unrefined)
	}
}

func TestClaimForUnknownRepoFails(t *testing.T) {
	f := newFixture(t)
	_, _, err := f.agents.Claim(f.ctx, agent, app.ClaimInput{Repo: "nobody/nothing"})
	requireErrorKind(t, err, domain.KindNotFound)
}

func TestPeopleCannotClaim(t *testing.T) {
	f := newFixture(t)
	_, _, err := f.agents.Claim(f.ctx, owner, app.ClaimInput{})
	requireErrorKind(t, err, domain.KindForbidden)
}

func TestWaitingClaimWakesWhenWorkArrives(t *testing.T) {
	f := newFixture(t)
	type result struct {
		work      app.Work
		isClaimed bool
		err       error
	}
	done := make(chan result, 1)
	go func() {
		work, isClaimed, err := f.agents.Claim(context.Background(), agent, app.ClaimInput{WaitSeconds: 20})
		done <- result{work, isClaimed, err}
	}()
	time.Sleep(100 * time.Millisecond)
	id := f.ticket("fresh idea", 0)
	select {
	case got := <-done:
		if got.err != nil || !got.isClaimed || got.work.Ticket.Ticket.ID != id {
			t.Fatalf("claim result = %+v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waiting claim did not wake when a ticket was created")
	}
}

func TestHeartbeatRequiresTheLeaseHolder(t *testing.T) {
	f := newFixture(t)
	id := f.ticket("idea", 0)
	f.mustClaim(agent, app.ClaimInput{})
	_, err := f.agents.Heartbeat(f.ctx, other, id, 0)
	requireErrorKind(t, err, domain.KindForbidden)
	before := f.detail(id).Ticket.Lease.ExpiresAt
	f.clock.Advance(time.Minute)
	detail, err := f.agents.Heartbeat(f.ctx, agent, id, 0)
	if err != nil || !detail.Ticket.Lease.ExpiresAt.After(before) {
		t.Fatalf("heartbeat: %v, expiry %v -> %v", err, before, detail.Ticket.Lease.ExpiresAt)
	}
}

func TestReleaseFreesTheTicket(t *testing.T) {
	f := newFixture(t)
	id := f.ticket("idea", 0)
	f.mustClaim(agent, app.ClaimInput{})
	if _, err := f.agents.Release(f.ctx, agent, id); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if work := f.mustClaim(other, app.ClaimInput{}); work.Ticket.Ticket.ID != id {
		t.Fatalf("claimed #%d after release", work.Ticket.Ticket.ID)
	}
}

func TestBriefForUnassignedTicketListsRepos(t *testing.T) {
	f := newFixture(t)
	f.repo("acme/api")
	f.ticket("which repo?", 0)
	brief := f.mustClaim(agent, app.ClaimInput{}).Brief
	for _, want := range []string{"Repos on the board", "`acme/api`", "What to do now", "business analyst"} {
		if !strings.Contains(brief, want) {
			t.Errorf("brief is missing %q:\n%s", want, brief)
		}
	}
}

func TestBriefCarriesAnswers(t *testing.T) {
	f := newFixture(t)
	id := f.ticket("idea", 0)
	asked, _ := f.agentAsk(id, ask("Mobile too?"))
	choice := 0
	if _, err := f.board.Answer(f.ctx, owner, id, []domain.AnswerDraft{{QuestionID: asked.Questions[0].ID, Choice: &choice, Text: "phones first"}}); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	brief := f.mustClaim(agent, app.ClaimInput{}).Brief
	if !strings.Contains(brief, `option 0, "yes"; phones first`) {
		t.Fatalf("brief does not carry the answer:\n%s", brief)
	}
}

func TestBriefNeverCutsTheSpecOrDesign(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	id := f.ticket("big", api.ID)
	spec := strings.Repeat("spec line\n", domain.MaxSpecLength/10)
	design := strings.Repeat("design ln\n", domain.MaxDesignLength/10)
	if _, err := f.board.SaveSpec(f.ctx, owner, id, app.SaveTextInput{Text: spec}); err != nil {
		t.Fatalf("spec: %v", err)
	}
	if _, err := f.board.Edit(f.ctx, owner, id, domain.TicketEdit{Body: ptr(strings.Repeat("body text\n", domain.MaxBodyLength/10))}); err != nil {
		t.Fatalf("body: %v", err)
	}
	f.toBuild(id)
	if _, err := f.board.SaveDesign(f.ctx, owner, id, app.SaveTextInput{Text: design}); err != nil {
		t.Fatalf("design: %v", err)
	}
	brief := f.mustClaim(agent, app.ClaimInput{}).Brief
	if !strings.Contains(brief, strings.TrimSpace(spec)) || !strings.Contains(brief, strings.TrimSpace(design)) {
		t.Fatal("the brief cut the spec or the design")
	}
}

func ptr(value string) *string { return &value }
