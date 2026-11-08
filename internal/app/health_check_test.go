package app_test

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func (f *fixture) healthInterval(repoID int64, n int) {
	f.t.Helper()
	if _, err := f.repos.Restyle(f.ctx, owner, repoID, domain.RepoStyle{HealthCheckEvery: &n}); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) checks(repoID int64) []domain.Ticket {
	f.t.Helper()
	board, err := f.board.Board(f.ctx, app.RepoScope{RepoID: repoID})
	if err != nil {
		f.t.Fatal(err)
	}
	var checks []domain.Ticket
	for _, s := range board.Tickets {
		if s.Ticket.IsHealthCheck {
			checks = append(checks, s.Ticket)
		}
	}
	return checks
}

func TestHealthCheckThresholdAndGeneratedContent(t *testing.T) {
	f := newFixture(t)
	repo := f.repo("acme/api")
	f.healthInterval(repo.ID, 3)
	ids := []int64{f.ticket("first change", repo.ID), f.ticket("second change", repo.ID), f.ticket("third change", repo.ID)}
	for i, id := range ids {
		f.move(owner, id, domain.LaneMerged, nil)
		want := 0
		if i == 2 {
			want = 1
		}
		if got := len(f.checks(repo.ID)); got != want {
			t.Fatalf("after %d merges got %d checks, want %d", i+1, got, want)
		}
	}
	check := f.checks(repo.ID)[0]
	if !check.IsHealthCheck || !check.IsAutonomous || check.Kind != domain.KindStory || check.Stage != domain.StageReady || check.ApprovedSpecRevision != 1 || check.SpecRevision != 1 || check.CreatedBy != "board" {
		t.Fatalf("generated ticket: %+v", check)
	}
	if check.Title != fmt.Sprintf("Architecture health check: acme/api after #%d..#%d", ids[0], ids[2]) {
		t.Fatal(check.Title)
	}
	for _, text := range []string{"first change", "second change", "third change", "duplication", "DRY", "SOLID", "layering", "dependency direction", "dead code", "test gaps", "Build tier: normal", "unknown"} {
		if !strings.Contains(check.Spec, text) {
			t.Errorf("spec lacks %q: %s", text, check.Spec)
		}
	}
	if f.laneOrder(domain.LaneTodo)[0] != check.ID {
		t.Fatal("check is not first")
	}
	detail := f.detail(check.ID)
	if len(detail.Decisions) != 0 {
		t.Fatal("generated a human agreement")
	}
	for _, e := range detail.Timeline {
		if e.Actor.Kind != domain.ActorSystem || e.Actor.Name != "board" {
			t.Fatalf("actor: %+v", e.Actor)
		}
	}
	notifications := 0
	for _, e := range f.events.events {
		if e.TicketID == check.ID {
			if _, ok := policy().Evaluate(e); ok {
				notifications++
			}
		}
	}
	if notifications != 1 {
		t.Fatalf("notifications: %d", notifications)
	}
}

func TestHealthCheckOpenGuardBacklogAndCompletion(t *testing.T) {
	f := newFixture(t)
	repo := f.repo("acme/api")
	f.healthInterval(repo.ID, 1)
	first := f.ticket("initial", repo.ID)
	f.move(owner, first, domain.LaneMerged, nil)
	checks := f.checks(repo.ID)
	if len(checks) != 1 {
		t.Fatalf("checks: %d", len(checks))
	}
	check := checks[0]
	for _, title := range []string{"backlog one", "backlog two"} {
		f.move(owner, f.ticket(title, repo.ID), domain.LaneMerged, nil)
	}
	if len(f.checks(repo.ID)) != 1 {
		t.Fatal("filed while open")
	}
	f.move(owner, check.ID, domain.LaneMerged, nil)
	checks = f.checks(repo.ID)
	if len(checks) != 2 {
		t.Fatalf("checks after completion: %d", len(checks))
	}
	var next domain.Ticket
	for _, c := range checks {
		if c.ID != check.ID {
			next = c
		}
	}
	if strings.Contains(next.Spec, "initial") || !strings.Contains(next.Spec, "backlog one") || !strings.Contains(next.Spec, "backlog two") {
		t.Fatal(next.Spec)
	}
	f.move(owner, next.ID, domain.LaneMerged, nil)
	if len(f.checks(repo.ID)) != 2 {
		t.Fatal("counted health check completion")
	}
}

func TestHealthCheckDisabledAndUnassigned(t *testing.T) {
	f := newFixture(t)
	repo := f.repo("acme/api")
	f.healthInterval(repo.ID, 0)
	for i := 0; i < 4; i++ {
		f.move(owner, f.ticket("disabled", repo.ID), domain.LaneMerged, nil)
	}
	f.move(owner, f.ticket("unassigned", 0), domain.LaneMerged, nil)
	if len(f.checks(repo.ID)) != 0 {
		t.Fatal("filed with disabled interval")
	}
	if f.detail(1).Repo.HealthCheckedThroughTicket != nil {
		t.Fatal("advanced disabled checkpoint")
	}
}

func TestHealthCheckReportedMergeAndRetry(t *testing.T) {
	f := newFixture(t)
	id := f.inReview("reported change")
	repoID := f.detail(id).Ticket.RepoID
	f.healthInterval(repoID, 1)
	if _, err := f.board.ReportPullRequest(f.ctx, agent, id, app.PullRequestMerged, ""); err != nil {
		t.Fatal(err)
	}
	if len(f.checks(repoID)) != 1 {
		t.Fatal("report did not file")
	}
	_, err := f.board.ReportPullRequest(f.ctx, agent, id, app.PullRequestMerged, "")
	requireErrorKind(t, err, domain.KindConflict)
	f.move(owner, id, domain.LaneMerged, nil)
	if len(f.checks(repoID)) != 1 {
		t.Fatal("retry filed twice")
	}
}

func TestHealthCheckRollsBackMergeAndFilingTogether(t *testing.T) {
	for name, trigger := range map[string]string{
		"checkpoint":         `CREATE TRIGGER fail_check BEFORE UPDATE OF health_checked_at ON repos BEGIN SELECT RAISE(ABORT, 'checkpoint failure'); END`,
		"generated timeline": `CREATE TRIGGER fail_check BEFORE INSERT ON timeline WHEN NEW.actor_kind = 'system' AND NEW.type = 'created' BEGIN SELECT RAISE(ABORT, 'timeline failure'); END`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			repo := f.repo("acme/api")
			f.healthInterval(repo.ID, 1)
			id := f.ticket("atomic change", repo.ID)
			if _, err := f.store.SQL().ExecContext(f.ctx, trigger); err != nil {
				t.Fatal(err)
			}
			eventsBefore := len(f.events.events)
			_, err := f.board.Move(f.ctx, owner, id, app.MoveInput{Lane: string(domain.LaneMerged)})
			if err == nil {
				t.Fatal("injected failure did not abort")
			}
			detail := f.detail(id)
			if detail.Ticket.Stage != domain.StageRefine || detail.Ticket.HealthCheckMergeSequence != 0 || detail.Repo.MergeSequence != 0 || detail.Repo.HealthCheckedThroughTicket != nil || len(f.checks(repo.ID)) != 0 || len(f.events.events) != eventsBefore {
				t.Fatal("partial transaction escaped rollback")
			}
			if _, err := f.store.SQL().ExecContext(f.ctx, `DROP TRIGGER fail_check`); err != nil {
				t.Fatal(err)
			}
			f.move(owner, id, domain.LaneMerged, nil)
			if len(f.checks(repo.ID)) != 1 || *f.detail(id).Repo.HealthCheckedThroughTicket != id || f.detail(id).Repo.HealthCheckedThroughMergeSequence != 1 {
				t.Fatal("retry was inconsistent")
			}
		})
	}
}

func TestHealthCheckConcurrentThresholdMerges(t *testing.T) {
	f := newFixture(t)
	repo := f.repo("acme/api")
	f.healthInterval(repo.ID, 2)
	f.move(owner, f.ticket("earlier", repo.ID), domain.LaneMerged, nil)
	ids := []int64{f.ticket("concurrent one", repo.ID), f.ticket("concurrent two", repo.ID)}
	start := make(chan struct{})
	results := make(chan error, len(ids))
	for _, id := range ids {
		go func(id int64) {
			<-start
			_, err := f.board.Move(f.ctx, owner, id, app.MoveInput{Lane: string(domain.LaneMerged)})
			results <- err
		}(id)
	}
	close(start)
	for range ids {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if len(f.checks(repo.ID)) != 1 {
		t.Fatal("concurrent merges duplicated filing")
	}
	got := f.detail(ids[0]).Repo
	if got.MergeSequence != 3 || got.HealthCheckedThroughMergeSequence != 2 {
		t.Fatalf("sequences: %+v", got)
	}
}

func TestHealthCheckMergeOrderRecompletionAndRepositoryIsolation(t *testing.T) {
	f := newFixture(t)
	repo := f.repo("acme/api")
	otherRepo := f.repo("acme/web")
	f.healthInterval(repo.ID, 3)
	f.healthInterval(otherRepo.ID, 1)
	oldest := f.ticket("created first merged last", repo.ID)
	second := f.ticket("merged first", repo.ID)
	third := f.ticket("merged second", repo.ID)
	f.move(owner, second, domain.LaneMerged, nil)
	f.move(owner, second, domain.LaneTodo, nil)
	f.move(owner, second, domain.LaneMerged, nil)
	if f.detail(second).Ticket.HealthCheckMergeSequence != 1 {
		t.Fatal("recompletion allocated a second merge")
	}
	f.move(owner, third, domain.LaneMerged, nil)
	if len(f.checks(repo.ID)) != 0 {
		t.Fatal("counted recompletion")
	}
	f.move(owner, f.ticket("separate", otherRepo.ID), domain.LaneMerged, nil)
	if len(f.checks(otherRepo.ID)) != 1 || len(f.checks(repo.ID)) != 0 {
		t.Fatal("cross-repository count")
	}
	f.move(owner, oldest, domain.LaneMerged, nil)
	check := f.checks(repo.ID)[0]
	if check.Title != fmt.Sprintf("Architecture health check: acme/api after #%d..#%d", second, oldest) {
		t.Fatal(check.Title)
	}
	if strings.Index(check.Spec, "merged first") >= strings.Index(check.Spec, "merged second") || strings.Index(check.Spec, "merged second") >= strings.Index(check.Spec, "created first merged last") {
		t.Fatal(check.Spec)
	}
}

func TestHealthCheckRepositoryReassignmentClearsSequence(t *testing.T) {
	f := newFixture(t)
	a := f.repo("acme/api")
	b := f.repo("acme/web")
	f.healthInterval(a.ID, 0)
	f.healthInterval(b.ID, 1)
	id := f.ticket("moved repo", a.ID)
	f.move(owner, id, domain.LaneMerged, nil)
	if f.detail(id).Ticket.HealthCheckMergeSequence == 0 {
		t.Fatal("disabled interval lost merge history")
	}
	detail, err := f.board.Edit(f.ctx, owner, id, domain.TicketEdit{RepoID: &b.ID})
	if err != nil {
		t.Fatal(err)
	}
	if detail.Ticket.HealthCheckMergeSequence != 0 || len(f.checks(b.ID)) != 0 {
		t.Fatal("edit carried merge state across repositories")
	}
	f.move(owner, id, domain.LaneTodo, nil)
	f.move(owner, id, domain.LaneMerged, nil)
	if len(f.checks(b.ID)) != 1 || len(f.checks(a.ID)) != 0 {
		t.Fatal("new repository merge not counted")
	}
}

func TestHealthCheckFilingIgnoresInFlightCapButClaimHonorsIt(t *testing.T) {
	f := newFixture(t)
	repo := f.repo("acme/api")
	f.healthInterval(repo.ID, 1)
	f.setInFlightCap(1)
	active := f.mustCreateAutonomous(repo.ID)
	f.mustClaim(agent, app.ClaimInput{})
	f.move(owner, f.ticket("ordinary delivery", repo.ID), domain.LaneMerged, nil)
	checks := f.checks(repo.ID)
	if len(checks) != 1 || checks[0].Stage != domain.StageReady {
		t.Fatal("cap blocked filing")
	}
	f.requireNoClaim(other, "capacity is occupied")
	f.move(owner, active, domain.LaneMerged, nil)
	if work := f.mustClaim(other, app.ClaimInput{}); work.Ticket.Ticket.ID != checks[0].ID {
		t.Fatal("queued check did not start after capacity freed")
	}
}

func TestHealthCheckBlockedGuardAndDeletedCheckpoint(t *testing.T) {
	f := newFixture(t)
	repo := f.repo("acme/api")
	f.healthInterval(repo.ID, 1)
	id := f.ticket("first delivery", repo.ID)
	f.move(owner, id, domain.LaneMerged, nil)
	check := f.checks(repo.ID)[0]
	f.mustClaim(agent, app.ClaimInput{})
	if _, err := f.board.Block(f.ctx, agent, check.ID, "review cannot continue"); err != nil {
		t.Fatal(err)
	}
	f.move(owner, f.ticket("during block", repo.ID), domain.LaneMerged, nil)
	if len(f.checks(repo.ID)) != 1 || *f.detail(check.ID).Repo.HealthCheckedThroughTicket != id {
		t.Fatal("blocked guard advanced checkpoint")
	}
	if err := f.board.Delete(f.ctx, owner, id); err != nil {
		t.Fatal(err)
	}
	f.move(owner, check.ID, domain.LaneMerged, nil)
	if len(f.checks(repo.ID)) != 2 {
		t.Fatal("deleted checkpoint prevented backlog filing")
	}
}

type healthCheckSink func(context.Context, []app.Event)

func (sink healthCheckSink) Publish(ctx context.Context, events []app.Event) { sink(ctx, events) }

func TestHealthCheckEventsSeeCommittedCheckpoint(t *testing.T) {
	f := newFixture(t)
	repo := f.repo("acme/api")
	f.healthInterval(repo.ID, 1)
	id := f.ticket("delivery", repo.ID)
	seen := 0
	board := app.NewBoardService(f.store, f.clock, healthCheckSink(func(ctx context.Context, events []app.Event) {
		for _, e := range events {
			if e.Type == domain.TimelineCreated {
				seen++
				detail, err := f.board.Ticket(ctx, e.TicketID)
				if err != nil {
					t.Fatal(err)
				}
				if !detail.Ticket.IsHealthCheck || detail.Repo.HealthCheckedThroughTicket == nil || *detail.Repo.HealthCheckedThroughTicket != id || detail.Repo.HealthCheckedThroughMergeSequence != 1 {
					t.Fatal("published before checkpoint committed")
				}
			}
		}
	}))
	if _, err := board.Move(f.ctx, owner, id, app.MoveInput{Lane: string(domain.LaneMerged)}); err != nil {
		t.Fatal(err)
	}
	if seen != 1 {
		t.Fatalf("creation events: %d", seen)
	}
}

func TestHealthCheckCheckpointCommitReferenceProvenance(t *testing.T) {
	f := newFixture(t)
	repo := f.repo("acme/api")
	f.healthInterval(repo.ID, 1)
	id := f.ticket("first delivery", repo.ID)
	for _, sha := range []string{"1234567", strings.Repeat("a", 40), strings.Repeat("b", 40), "cdef123"} {
		if _, err := f.board.AddWorkLink(f.ctx, owner, id, domain.WorkLinkDraft{Kind: "commit", SHA: sha}); err != nil {
			t.Fatal(err)
		}
		f.clock.Advance(time.Second)
	}
	f.move(owner, id, domain.LaneMerged, nil)
	first := f.checks(repo.ID)[0]
	if !strings.Contains(first.Spec, "Base commit: unknown") || strings.Contains(first.Spec, strings.Repeat("b", 40)) {
		t.Fatal("first covered commit was treated as the base")
	}
	f.move(owner, f.ticket("next delivery", repo.ID), domain.LaneMerged, nil)
	f.move(owner, first.ID, domain.LaneMerged, nil)
	for _, check := range f.checks(repo.ID) {
		if check.ID != first.ID {
			if !strings.Contains(check.Spec, strings.Repeat("b", 40)) || !strings.Contains(check.Spec, "not a verified merge base") || !strings.Contains(check.Spec, "commit work links") {
				t.Fatal(check.Spec)
			}
		}
	}
}

func TestHealthCheckLargeBacklogIsComplete(t *testing.T) {
	f := newFixture(t)
	repo := f.repo("a/" + strings.Repeat("b", 198))
	f.healthInterval(repo.ID, 1)
	firstID := f.ticket("start", repo.ID)
	f.move(owner, firstID, domain.LaneMerged, nil)
	first := f.checks(repo.ID)[0]
	var ids []int64
	for i := 0; i < 110; i++ {
		id := f.ticket(fmt.Sprintf("%03d %s", i, strings.Repeat("x", 190)), repo.ID)
		ids = append(ids, id)
		f.move(owner, id, domain.LaneMerged, nil)
	}
	f.move(owner, first.ID, domain.LaneMerged, nil)
	for _, check := range f.checks(repo.ID) {
		if check.ID != first.ID {
			if len(check.Title) <= domain.MaxTitleLength || len(check.Spec) <= domain.MaxSpecLength {
				t.Fatal("range did not exceed input limits")
			}
			for _, id := range ids {
				if !strings.Contains(check.Spec, fmt.Sprintf("#%d:", id)) {
					t.Fatalf("missing #%d", id)
				}
			}
			if *f.detail(check.ID).Repo.HealthCheckedThroughTicket != ids[len(ids)-1] {
				t.Fatal("incomplete checkpoint")
			}
		}
	}
}

func (f *fixture) leasedHealthCheck() domain.Ticket {
	f.t.Helper()
	repo := f.repo("acme/review")
	f.healthInterval(repo.ID, 1)
	f.move(owner, f.ticket("completed work", repo.ID), domain.LaneMerged, nil)
	check := f.checks(repo.ID)[0]
	if got := f.mustClaim(agent, app.ClaimInput{}).Ticket.Ticket.ID; got != check.ID {
		f.t.Fatalf("claimed %d, want %d", got, check.ID)
	}
	return f.detail(check.ID).Ticket
}

func (f *fixture) followUp(parent int64, spec string) (app.TicketDetail, error) {
	return f.board.CreateRelated(f.ctx, agent, domain.TicketDraft{Kind: "story", Title: "Simplify module"}, "follow_up_of", parent, spec)
}

func TestHealthCheckFollowUpCapIsAtomic(t *testing.T) {
	for _, spec := range []string{"Refactor specification.", ""} {
		t.Run(fmt.Sprintf("spec=%t", spec != ""), func(t *testing.T) {
			f := newFixture(t)
			parent := f.leasedHealthCheck()
			for i := 0; i < 3; i++ {
				if _, err := f.followUp(parent.ID, spec); err != nil {
					t.Fatal(err)
				}
			}
			before, err := f.board.Board(f.ctx, app.RepoScope{})
			if err != nil {
				t.Fatal(err)
			}
			parentBefore := f.detail(parent.ID)
			eventsBefore := len(f.events.events)
			_, err = f.followUp(parent.ID, spec)
			requireErrorKind(t, err, domain.KindInvalid)
			if !strings.Contains(err.Error(), fmt.Sprintf("health check %d allows at most 3", parent.ID)) {
				t.Fatal(err)
			}
			after, err := f.board.Board(f.ctx, app.RepoScope{})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(parentBefore, f.detail(parent.ID)) || len(f.events.events) != eventsBefore {
				t.Fatal("refusal changed board, history or events")
			}
		})
	}
}

func TestHealthCheckFollowUpPlacement(t *testing.T) {
	for _, health := range []bool{true, false} {
		t.Run(fmt.Sprint(health), func(t *testing.T) {
			f := newFixture(t)
			var parent domain.Ticket
			if health {
				parent = f.leasedHealthCheck()
			} else {
				repo := f.repo("acme/api")
				id := f.mustCreateAutonomous(repo.ID)
				f.mustClaim(agent, app.ClaimInput{})
				parent = f.detail(id).Ticket
			}
			existing := f.mustCreateAutonomous(parent.RepoID)
			child, err := f.followUp(parent.ID, "Simplify one subsystem.")
			if err != nil {
				t.Fatal(err)
			}
			c := child.Ticket
			if !c.IsAutonomous || c.IsHealthCheck || c.RepoID != parent.RepoID || c.Stage != domain.StageReady || c.SpecRevision != 1 || c.ApprovedSpecRevision != 1 || len(child.Decisions) != 1 || child.Decisions[0].Weight != domain.WeightBinding || len(child.Related) != 1 {
				t.Fatalf("inheritance: %+v", child)
			}
			order := f.laneOrder(domain.LaneTodo)
			want := existing
			if health {
				want = c.ID
			}
			if order[0] != want {
				t.Fatalf("order %v, want %d first", order, want)
			}
		})
	}
}

func TestHealthCheckFollowUpLinkPolicy(t *testing.T) {
	f := newFixture(t)
	parent := f.leasedHealthCheck()
	// Outgoing links and incoming links of other kinds do not consume slots.
	extra := f.ticket("other work", parent.RepoID)
	for _, link := range []struct {
		from, to int64
		kind     string
	}{{parent.ID, extra, "follow_up_of"}, {extra, parent.ID, "relates_to"}} {
		if _, err := f.board.LinkTickets(f.ctx, agent, link.from, link.kind, link.to); err != nil {
			t.Fatal(err)
		}
	}
	ids := []int64{}
	for i := 0; i < 3; i++ {
		id := f.ticket("human child", parent.RepoID)
		ids = append(ids, id)
		if _, err := f.board.LinkTickets(f.ctx, owner, id, "follow_up_of", parent.ID); err != nil {
			t.Fatal(err)
		}
	}
	f.healthInterval(parent.RepoID, 0)
	f.move(owner, ids[0], domain.LaneMerged, nil)
	_, err := f.board.LinkTickets(f.ctx, agent, extra, "follow_up_of", parent.ID)
	requireErrorKind(t, err, domain.KindInvalid)
	_, err = f.board.LinkTickets(f.ctx, agent, ids[0], "follow_up_of", parent.ID)
	requireErrorKind(t, err, domain.KindConflict)
	// A human can exceed the cap; removing two live links frees one slot.
	if _, err := f.board.LinkTickets(f.ctx, owner, extra, "follow_up_of", parent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.board.UnlinkTickets(f.ctx, owner, extra, "follow_up_of", parent.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.board.Delete(f.ctx, owner, ids[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := f.board.LinkTickets(f.ctx, agent, extra, "follow_up_of", parent.ID); err != nil {
		t.Fatal(err)
	}
}

func TestHealthCheckFollowUpConcurrentCap(t *testing.T) {
	f := newFixture(t)
	parent := f.leasedHealthCheck()
	for i := 0; i < 2; i++ {
		if _, err := f.followUp(parent.ID, "Small refactor."); err != nil {
			t.Fatal(err)
		}
	}
	before := f.ticketCount()
	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { <-start; _, err := f.followUp(parent.ID, "Small refactor."); results <- err }()
	}
	close(start)
	success, refused := 0, 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			success++
		} else if domain.KindOf(err) == domain.KindInvalid && strings.Contains(err.Error(), "at most 3") {
			refused++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || refused != 1 || f.ticketCount() != before+1 {
		t.Fatalf("success %d refused %d count %d", success, refused, f.ticketCount())
	}
	count := 0
	for _, related := range f.detail(parent.ID).Related {
		if related.Relation.Kind == domain.RelationFollowUpOf && related.Relation.ToID == parent.ID {
			count++
		}
	}
	if count != 3 {
		t.Fatalf("follow-ups %d", count)
	}
}

func TestRelatedCreationRollsBackOnLinkFailure(t *testing.T) {
	for _, autonomous := range []bool{true, false} {
		t.Run(fmt.Sprint(autonomous), func(t *testing.T) {
			f := newFixture(t)
			parent := f.leasedHealthCheck()
			if _, err := f.store.SQL().ExecContext(f.ctx, `CREATE TRIGGER fail_relation BEFORE INSERT ON relations WHEN NEW.kind = 'follow_up_of' BEGIN SELECT RAISE(ABORT, 'link failure'); END`); err != nil {
				t.Fatal(err)
			}
			before := f.ticketCount()
			eventsBefore := len(f.events.events)
			parentBefore := f.detail(parent.ID)
			spec := ""
			if autonomous {
				spec = "Small refactor."
			}
			if _, err := f.followUp(parent.ID, spec); err == nil {
				t.Fatal("expected link failure")
			}
			if f.ticketCount() != before || len(f.events.events) != eventsBefore || !reflect.DeepEqual(parentBefore, f.detail(parent.ID)) {
				t.Fatal("orphan or events survived failed link")
			}
			for _, table := range []string{"decisions", "timeline"} {
				var count int
				if err := f.store.SQL().QueryRowContext(f.ctx, "SELECT COUNT(*) FROM "+table+" WHERE ticket_id NOT IN (SELECT id FROM tickets)").Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Fatalf("orphan %s rows: %d", table, count)
				}
			}
		})
	}
}
