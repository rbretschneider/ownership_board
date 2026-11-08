package app_test

import (
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func (f *fixture) mergedForDeploy() int64 {
	repo := f.repo("acme/release")
	mode := domain.DeployModeAgent
	if _, err := f.repos.Restyle(f.ctx, owner, repo.ID, domain.RepoStyle{DeployMode: &mode}); err != nil {
		f.t.Fatal(err)
	}
	id := f.ticket("Release", repo.ID)
	f.move(owner, id, domain.LaneMerged, nil)
	return id
}

func TestRequestDeploymentAuthorizationAndEligibility(t *testing.T) {
	for _, tc := range []string{"agent", "system", "refine", "deployed", "off", "unassigned", "blocked", "duplicate"} {
		t.Run(tc, func(t *testing.T) {
			f := newFixture(t)
			id := f.mergedForDeploy()
			actor := owner
			want := domain.KindConflict
			switch tc {
			case "agent":
				actor = agent
				want = domain.KindForbidden
			case "system":
				actor = domain.SystemActor
				want = domain.KindForbidden
			case "refine":
				f.move(owner, id, domain.LaneTodo, nil)
			case "deployed":
				f.move(owner, id, domain.LaneDeployed, nil)
			case "off":
				mode := domain.DeployModeOff
				_, err := f.repos.Restyle(f.ctx, owner, f.detail(id).Ticket.RepoID, domain.RepoStyle{DeployMode: &mode})
				if err != nil {
					t.Fatal(err)
				}
			case "unassigned":
				n := int64(0)
				if _, err := f.board.Edit(f.ctx, owner, id, domain.TicketEdit{RepoID: &n}); err != nil {
					t.Fatal(err)
				}
			case "blocked":
				if err := f.store.Write(f.ctx, func(r app.Repos) error {
					ticket, err := r.Tickets().Get(f.ctx, id)
					if err != nil {
						return err
					}
					ticket.BlockedReason = "Needs attention"
					return r.Tickets().Update(f.ctx, ticket)
				}); err != nil {
					t.Fatal(err)
				}
			case "duplicate":
				if _, err := f.board.RequestDeploy(f.ctx, owner, id); err != nil {
					t.Fatal(err)
				}
			}
			before := f.detail(id)
			events := len(f.events.events)
			_, err := f.board.RequestDeploy(f.ctx, actor, id)
			requireErrorKind(t, err, want)
			if !reflect.DeepEqual(before, f.detail(id)) || len(f.events.events) != events {
				t.Fatal("rejected request mutated state")
			}
		})
	}
}

func TestDeploymentCycleAndCompletionAccounting(t *testing.T) {
	for _, block := range []bool{false, true} {
		t.Run(map[bool]string{false: "report", true: "block"}[block], func(t *testing.T) {
			f := newFixture(t)
			id := f.mergedForDeploy()
			initial := f.detail(id).Ticket
			dependent, err := f.create(owner, "Next", initial.RepoID, id)
			if err != nil {
				t.Fatal(err)
			}
			met := len(dependenciesMet(f.detail(dependent.Ticket.ID).Timeline))
			queueEvents := len(f.events.events)
			queued, err := f.board.RequestDeploy(f.ctx, owner, id)
			if err != nil {
				t.Fatal(err)
			}
			if queued.Ticket.Stage != domain.StageDeploying || queued.Ticket.Owner(0) != domain.OwnerAgent || queued.Ticket.Lease != nil || queued.Ticket.Rank != initial.Rank || !queued.Ticket.LaneChangedAt.Equal(initial.LaneChangedAt) {
				t.Fatalf("queued: %+v", queued.Ticket)
			}
			for _, e := range f.events.events[queueEvents:] {
				if _, ok := policy().Evaluate(e); ok {
					t.Fatal("queue request notified")
				}
			}
			d := queued.Decisions[len(queued.Decisions)-1]
			if d.Weight != domain.WeightBinding || d.Source != domain.DecisionFromHuman || d.Summary != "Deploy requested by "+owner.Name {
				t.Fatalf("decision: %+v", d)
			}
			if len(f.waitingOn(dependent.Ticket.ID)) != 0 {
				t.Fatal("deploying prerequisite reopened")
			}
			work := f.mustClaim(agent, app.ClaimInput{})
			if work.Ticket.Ticket.ID != id || work.Action != domain.ActionDeploy {
				t.Fatal("deployment not prioritized")
			}
			start := len(f.events.events)
			var failed app.TicketDetail
			if block {
				failed, err = f.board.Block(f.ctx, agent, id, "Runtime\nverification failed")
			} else {
				failed, err = f.board.ReportDeploy(f.ctx, agent, id, app.DeployFailed, "Runtime\nverification failed")
			}
			if err != nil {
				t.Fatal(err)
			}
			if failed.Ticket.Stage != domain.StageDone || failed.Ticket.Lane != domain.LaneMerged || failed.Ticket.BlockedReason != "Runtime verification failed" || failed.Ticket.Lease != nil {
				t.Fatalf("failure: %+v", failed.Ticket)
			}
			foundFailureComment := false
			for _, e := range failed.Timeline {
				if e.Type == domain.TimelineComment && e.Body == "Runtime\nverification failed" {
					foundFailureComment = true
				}
			}
			if !foundFailureComment {
				t.Fatal("failure summary not preserved as a comment")
			}
			assertDeployNotes(t, f.events.events[start:], app.PriorityHigh)
			if _, err = f.board.Unblock(f.ctx, owner, id, ""); err != nil {
				t.Fatal(err)
			}
			f.requireStage(id, domain.StageDone)
			if _, err = f.board.RequestDeploy(f.ctx, owner, id); err != nil {
				t.Fatal(err)
			}
			f.mustClaim(agent, app.ClaimInput{})
			// Changing settings after queueing cannot prevent reporting the result.
			mode := domain.DeployModeOff
			if _, err = f.repos.Restyle(f.ctx, owner, initial.RepoID, domain.RepoStyle{DeployMode: &mode}); err != nil {
				t.Fatal(err)
			}
			start = len(f.events.events)
			success, err := f.board.ReportDeploy(f.ctx, agent, id, app.DeployDeployed, "  Version verified: abc123\nHealth check passed.  ")
			if err != nil {
				t.Fatal(err)
			}
			if success.Ticket.Stage != domain.StageDeployed || success.Ticket.Lane != domain.LaneDeployed || success.Ticket.IsBlocked() || success.Ticket.Lease != nil {
				t.Fatalf("success: %+v", success.Ticket)
			}
			found := false
			for _, e := range success.Timeline {
				if e.Type == domain.TimelineComment && e.Body == "Version verified: abc123\nHealth check passed." {
					found = true
				}
			}
			if !found {
				t.Fatal("summary not saved")
			}
			assertDeployNotes(t, f.events.events[start:], app.PriorityLow)
			if success.Ticket.HealthCheckMergeSequence != initial.HealthCheckMergeSequence || len(dependenciesMet(f.detail(dependent.Ticket.ID).Timeline)) != met {
				t.Fatal("deployment repeated merge accounting")
			}
		})
	}
}

func assertDeployNotes(t *testing.T, events []app.Event, want app.Priority) {
	t.Helper()
	n := 0
	for _, e := range events {
		if note, ok := policy().Evaluate(e); ok {
			n++
			if note.Priority != want || strings.Contains(note.Message, "verification") {
				t.Fatalf("notification: %+v", note)
			}
		}
	}
	if n != 1 {
		t.Fatalf("notifications: %d; events %+v", n, events)
	}
}

func TestDeploymentReportValidation(t *testing.T) {
	for _, tc := range []string{"missing", "foreign", "expired", "human", "system", "blank", "long", "hidden", "invalid", "wrong-stage", "blocked"} {
		t.Run(tc, func(t *testing.T) {
			f := newFixture(t)
			id := f.mergedForDeploy()
			if _, err := f.board.RequestDeploy(f.ctx, owner, id); err != nil {
				t.Fatal(err)
			}
			actor := agent
			summary := "Verified runtime version"
			outcome := app.DeployDeployed
			want := domain.KindForbidden
			if tc != "missing" {
				f.mustClaim(agent, app.ClaimInput{})
			}
			switch tc {
			case "foreign":
				actor = other
			case "expired":
				f.clock.Advance(time.Hour)
			case "human":
				actor = owner
			case "system":
				actor = domain.SystemActor
			case "blank":
				summary = " "
				want = domain.KindInvalid
			case "long":
				summary = strings.Repeat("a", 1001)
				want = domain.KindInvalid
			case "hidden":
				summary = "text\u200bhidden"
				want = domain.KindInvalid
			case "invalid":
				outcome = "unknown"
				want = domain.KindInvalid
			case "wrong-stage":
				f.move(owner, id, domain.LaneTodo, nil)
				want = domain.KindForbidden
			case "blocked":
				if err := f.store.Write(f.ctx, func(r app.Repos) error {
					ticket, err := r.Tickets().Get(f.ctx, id)
					if err != nil {
						return err
					}
					ticket.BlockedReason = "stop"
					return r.Tickets().Update(f.ctx, ticket)
				}); err != nil {
					t.Fatal(err)
				}
				want = domain.KindConflict
			}
			before := f.detail(id)
			events := len(f.events.events)
			_, err := f.board.ReportDeploy(f.ctx, actor, id, outcome, summary)
			requireErrorKind(t, err, want)
			if !reflect.DeepEqual(before, f.detail(id)) || len(f.events.events) != events {
				t.Fatal("rejected report mutated state")
			}
		})
	}
}

func TestConcurrentDeployRequests(t *testing.T) {
	f := newFixture(t)
	id := f.mergedForDeploy()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := f.board.RequestDeploy(f.ctx, owner, id); errs <- err }()
	}
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for err := range errs {
		if err == nil {
			success++
		} else if domain.KindOf(err) == domain.KindConflict {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 || len(f.detail(id).Decisions) != 1 {
		t.Fatal("request was not serialized")
	}
}

func TestDeploymentClaimPriorityCapacityAndRecovery(t *testing.T) {
	f := newFixture(t)
	id := f.mergedForDeploy()
	repoID := f.detail(id).Ticket.RepoID
	id = f.mustCreateAutonomous(repoID)
	f.move(owner, id, domain.LaneMerged, nil)
	f.repo("acme/other")
	// Fill capacity with already active autonomous work.
	for range 3 {
		n := f.mustCreateAutonomous(repoID)
		f.move(owner, n, domain.LaneInProgress, nil)
	}
	var before int
	if err := f.store.Read(f.ctx, func(r app.Repos) error {
		var err error
		before, err = r.Tickets().CountInFlightAutonomous(f.ctx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.board.RequestDeploy(f.ctx, owner, id); err != nil {
		t.Fatal(err)
	}
	work := f.mustClaim(agent, app.ClaimInput{Repo: "acme/release"})
	if work.Ticket.Ticket.ID != id || work.Action != domain.ActionDeploy {
		t.Fatalf("priority/capacity: %+v", work)
	}
	if _, claimed := f.claim(other, app.ClaimInput{Repo: "acme/other"}); claimed {
		t.Fatal("foreign repository claimed")
	}
	if _, err := f.agents.Heartbeat(f.ctx, agent, id, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := f.agents.Release(f.ctx, agent, id); err != nil {
		t.Fatal(err)
	}
	f.requireStage(id, domain.StageDeploying)
	work = f.mustClaim(other, app.ClaimInput{})
	if work.Ticket.Ticket.ID != id {
		t.Fatal("release did not resume deployment")
	}
	f.clock.Advance(time.Hour)
	work = f.mustClaim(agent, app.ClaimInput{})
	if work.Ticket.Ticket.ID != id || !strings.Contains(work.Brief, "partly") {
		t.Fatal("expired deployment not resumed with warning")
	}
	if err := f.store.Read(f.ctx, func(r app.Repos) error {
		n, err := r.Tickets().CountInFlightAutonomous(f.ctx)
		if n != before {
			t.Fatalf("capacity changed: %d -> %d", before, n)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDeployWritesRollbackTogether(t *testing.T) {
	for _, report := range []bool{false, true} {
		t.Run(map[bool]string{false: "request", true: "report"}[report], func(t *testing.T) {
			f := newFixture(t)
			id := f.mergedForDeploy()
			if report {
				if _, err := f.board.RequestDeploy(f.ctx, owner, id); err != nil {
					t.Fatal(err)
				}
				f.mustClaim(agent, app.ClaimInput{})
			}
			before := f.detail(id)
			events := len(f.events.events)
			if _, err := f.store.SQL().ExecContext(f.ctx, `CREATE TRIGGER fail_deploy BEFORE INSERT ON timeline WHEN NEW.type = 'stage' BEGIN SELECT RAISE(ABORT, 'stage write failure'); END`); err != nil {
				t.Fatal(err)
			}
			var err error
			if report {
				_, err = f.board.ReportDeploy(f.ctx, agent, id, app.DeployFailed, "Failed health check")
			} else {
				_, err = f.board.RequestDeploy(f.ctx, owner, id)
			}
			if err == nil {
				t.Fatal("expected injected write failure")
			}
			if !reflect.DeepEqual(before, f.detail(id)) || len(f.events.events) != events {
				t.Fatal("partial deployment mutation escaped rollback")
			}
		})
	}
}

func TestDeploymentPreservesMergeEffectsAcrossAttempts(t *testing.T) {
	f := newFixture(t)
	repo := f.repo("acme/release")
	f.healthInterval(repo.ID, 1)
	mode := domain.DeployModeAgent
	if _, err := f.repos.Restyle(f.ctx, owner, repo.ID, domain.RepoStyle{DeployMode: &mode}); err != nil {
		t.Fatal(err)
	}
	id := f.ticket("Delivery", repo.ID)
	dependent, err := f.create(owner, "Following change", repo.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	f.move(owner, id, domain.LaneMerged, nil)
	original := f.detail(id).Ticket.HealthCheckMergeSequence
	if len(f.checks(repo.ID)) != 1 || len(dependenciesMet(f.detail(dependent.Ticket.ID).Timeline)) != 1 {
		t.Fatal("initial merge effects missing")
	}
	for _, outcome := range []app.DeployOutcome{app.DeployFailed, app.DeployDeployed} {
		if _, err := f.board.RequestDeploy(f.ctx, owner, id); err != nil {
			t.Fatal(err)
		}
		work := f.mustClaim(agent, app.ClaimInput{})
		if work.Ticket.Ticket.ID != id {
			t.Fatal("deploy did not precede ready work")
		}
		if _, err := f.board.ReportDeploy(f.ctx, agent, id, outcome, "Runtime check evidence"); err != nil {
			t.Fatal(err)
		}
		if outcome == app.DeployFailed {
			if _, err := f.board.Unblock(f.ctx, owner, id, ""); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(f.checks(repo.ID)) != 1 || len(dependenciesMet(f.detail(dependent.Ticket.ID).Timeline)) != 1 || f.detail(id).Ticket.HealthCheckMergeSequence != original {
		t.Fatal("deployment repeated completion effects")
	}
}

func TestHumanCanStopDeployAndAgentsCannotChooseDeployStage(t *testing.T) {
	f := newFixture(t)
	id := f.mergedForDeploy()
	if _, err := f.board.RequestDeploy(f.ctx, owner, id); err != nil {
		t.Fatal(err)
	}
	f.mustClaim(agent, app.ClaimInput{})
	for _, lane := range []string{"merged", "deployed", "deploying"} {
		if _, err := f.board.Move(f.ctx, agent, id, app.MoveInput{Lane: lane}); err == nil {
			t.Fatalf("agent chose %s", lane)
		}
	}
	detail, err := f.board.Block(f.ctx, owner, id, "Stop this attempt")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Ticket.Stage != domain.StageDone || !detail.Ticket.IsBlocked() || detail.Ticket.Lease != nil {
		t.Fatalf("human block: %+v", detail.Ticket)
	}
}

func TestMergeReportPinsRecordedIdentity(t *testing.T) {
	f := newFixture(t)
	id := f.inReview("Candidate delivery")
	mode := domain.DeployModeAgent
	if _, err := f.repos.Restyle(f.ctx, owner, f.detail(id).Ticket.RepoID, domain.RepoStyle{DeployMode: &mode}); err != nil {
		t.Fatal(err)
	}
	detail, err := f.board.ReportPullRequest(f.ctx, agent, id, app.PullRequestMerged, "merge commit: 0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range detail.Timeline {
		if e.Type == domain.TimelineComment && e.Data["pr_state"] == "merged" && e.Data["pr_url"] == detail.Ticket.PRURL {
			found = true
		}
	}
	if !found {
		t.Fatal("merge report metadata missing")
	}
	if _, err := f.board.RequestDeploy(f.ctx, owner, id); err != nil {
		t.Fatal(err)
	}
	work := f.mustClaim(agent, app.ClaimInput{})
	if !strings.Contains(work.Brief, "merge commit: 0123456789abcdef") || !strings.Contains(work.Brief, "Deployment context") {
		t.Fatal("merge identity not carried into deployment")
	}
}
