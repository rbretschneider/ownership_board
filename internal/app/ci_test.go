package app_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

const (
	ciHead  = "1111111111111111111111111111111111111111"
	ciOlder = "2222222222222222222222222222222222222222"
)

// fakeCI is a provider's CI held in memory: runs by id, branch heads, and
// what was re-run or cancelled.
type fakeCI struct {
	runs       map[int64]domain.CIRun
	heads      map[string]string
	runnersErr error
	headErr    error
	runners    []domain.CIRunner
	cancelled  []int64
	reran      []int64
	rerunWhole []int64
}

func newFakeCI(runs ...domain.CIRun) *fakeCI {
	ci := &fakeCI{runs: map[int64]domain.CIRun{}, heads: map[string]string{"feat/x": ciHead, "main": ciHead}}
	for _, run := range runs {
		ci.runs[run.ID] = run
	}
	return ci
}

func (c *fakeCI) CI(context.Context, domain.Provider) (app.CIProvider, error) { return c, nil }

func (c *fakeCI) Runs(_ context.Context, _ string, query domain.CIRunQuery) ([]domain.CIRun, error) {
	var runs []domain.CIRun
	for _, run := range c.runs {
		if query.Branch == "" || run.HeadBranch == query.Branch {
			run.Jobs = nil
			runs = append(runs, run)
		}
	}
	slices.SortFunc(runs, func(a, b domain.CIRun) int { return int(b.ID - a.ID) })
	return runs, nil
}

func (c *fakeCI) Run(_ context.Context, _ string, runID int64) (domain.CIRun, error) {
	run, isKnown := c.runs[runID]
	if !isKnown {
		return domain.CIRun{}, domain.NotFoundf("no run %d", runID)
	}
	return run, nil
}

func (c *fakeCI) Job(_ context.Context, _ string, jobID int64) (domain.CIJob, error) {
	for _, run := range c.runs {
		for _, job := range run.Jobs {
			if job.ID == jobID {
				return job, nil
			}
		}
	}
	return domain.CIJob{}, domain.NotFoundf("no job %d", jobID)
}

func (c *fakeCI) JobLogTail(context.Context, string, int64, int) (string, error) {
	return "error: the test failed", nil
}

func (c *fakeCI) BranchHead(_ context.Context, _, branch string) (string, error) {
	return c.heads[branch], c.headErr
}

func (c *fakeCI) PullHead(context.Context, string, int) (string, string, error) {
	return "feat/x", c.heads["feat/x"], nil
}

func (c *fakeCI) Runners(context.Context, string) ([]domain.CIRunner, error) {
	return c.runners, c.runnersErr
}

func (c *fakeCI) RerunFailedJobs(_ context.Context, _ string, runID int64) error {
	c.reran = append(c.reran, runID)
	return nil
}

func (c *fakeCI) RerunRun(_ context.Context, _ string, runID int64) error {
	c.rerunWhole = append(c.rerunWhole, runID)
	return nil
}

func (c *fakeCI) RerunJob(_ context.Context, _ string, jobID int64) error {
	c.reran = append(c.reran, jobID)
	return nil
}

func (c *fakeCI) CancelRun(_ context.Context, _ string, runID int64) error {
	c.cancelled = append(c.cancelled, runID)
	return nil
}

// ciBoard is a board with one synced repo, one autonomous ticket on branch
// feat/x, and the CI service over fake provider CI.
type ciBoard struct {
	*fixture
	ci      *fakeCI
	service *app.CIService
	repo    domain.Repo
	ticket  int64
}

func newCIBoard(t *testing.T, runs ...domain.CIRun) ciBoard {
	t.Helper()
	f := newFixture(t)
	source := &listingSource{listings: []domain.RepoListing{{Provider: domain.ProviderForgejo, FullName: "acme/api", URL: "https://git.example.com/acme/api", DefaultBranch: "main"}}}
	f.repos = app.NewRepoService(f.store, f.clock, staticCatalog{source}, discardLogger())
	if err := f.repos.Sync(f.ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}
	repo := f.repoNamed("acme/api")
	created, err := f.board.Create(f.ctx, owner, domain.TicketDraft{Kind: "story", Title: "autopilot", RepoID: repo.ID, IsAutonomous: true})
	if err != nil {
		t.Fatalf("create autonomous ticket: %v", err)
	}
	branch := "feat/x"
	if _, err := f.board.Edit(f.ctx, owner, created.Ticket.ID, domain.TicketEdit{Branch: &branch}); err != nil {
		t.Fatalf("set branch: %v", err)
	}
	ci := newFakeCI(runs...)
	return ciBoard{fixture: f, ci: ci, repo: repo, ticket: created.Ticket.ID,
		service: app.NewCIService(f.store, f.clock, app.FanoutSink{f.events}, ci)}
}

func ciRunAt(id int64, branch, sha string, status domain.CIStatus, conclusion domain.CIConclusion) domain.CIRun {
	return domain.CIRun{ID: id, Workflow: "CI", Event: "push", HeadBranch: branch, HeadSHA: sha, Status: status, Conclusion: conclusion}
}

func (b ciBoard) ciHistory() []domain.TimelineEntry {
	b.t.Helper()
	detail, err := b.board.Ticket(b.ctx, b.ticket)
	if err != nil {
		b.t.Fatalf("get ticket: %v", err)
	}
	var entries []domain.TimelineEntry
	for _, entry := range detail.Timeline {
		if entry.Type == domain.TimelineCI {
			entries = append(entries, entry)
		}
	}
	return entries
}

func TestTheManagerCannotCancelCIOnAPRsCurrentHead(t *testing.T) {
	b := newCIBoard(t, ciRunAt(5, "feat/x", ciHead, domain.CIInProgress, ""))
	_, err := b.service.Cancel(b.ctx, manager, app.CICancelInput{TicketID: b.ticket, RunID: 5, Reason: "stale head"})
	if domain.KindOf(err) != domain.KindConflict {
		t.Fatalf("err = %v, want a conflict", err)
	}
	if len(b.ci.cancelled) != 0 || len(b.ciHistory()) != 0 {
		t.Fatalf("cancelled %v, history %v: want nothing cancelled and nothing recorded", b.ci.cancelled, b.ciHistory())
	}
}

func TestTheManagerCancelsARunANewerPushReplacedAndItIsRecorded(t *testing.T) {
	b := newCIBoard(t, ciRunAt(4, "feat/x", ciOlder, domain.CIQueued, ""))
	if _, err := b.service.Cancel(b.ctx, manager, app.CICancelInput{TicketID: b.ticket, RunID: 4, Reason: "a newer push replaced it"}); err != nil {
		t.Fatalf("cancel an obsolete run: %v", err)
	}
	history := b.ciHistory()
	if !slices.Equal(b.ci.cancelled, []int64{4}) || len(history) != 1 || history[0].Data["action"] != string(domain.CIActionCancel) ||
		history[0].Body != "a newer push replaced it" || history[0].Actor.Name != manager.Name {
		t.Fatalf("cancelled %v, history %+v: want run 4 cancelled and recorded with the reason", b.ci.cancelled, history)
	}
}

func TestAReRunWaitsForTheRunToComplete(t *testing.T) {
	b := newCIBoard(t, ciRunAt(6, "feat/x", ciHead, domain.CIInProgress, ""))
	_, err := b.service.Rerun(b.ctx, manager, app.CIRerunInput{TicketID: b.ticket, RunID: 6, Reason: "flaky runner"})
	if domain.KindOf(err) != domain.KindConflict || len(b.ci.reran) != 0 {
		t.Fatalf("err = %v, reran %v: want a conflict and no re-run while the run is going", err, b.ci.reran)
	}
	b.ci.runs[6] = ciRunAt(6, "feat/x", ciHead, domain.CICompleted, domain.CICancelled)
	if _, err := b.service.Rerun(b.ctx, manager, app.CIRerunInput{TicketID: b.ticket, RunID: 6, Reason: "evicted by a concurrency group"}); err != nil {
		t.Fatalf("re-run a completed run: %v", err)
	}
	// An evicted run may have no failed job, so a cancelled run is re-run whole.
	if !slices.Equal(b.ci.rerunWhole, []int64{6}) || len(b.ci.reran) != 0 || len(b.ciHistory()) != 1 {
		t.Fatalf("whole %v, failed-only %v, history %v: want run 6 re-run whole and recorded", b.ci.rerunWhole, b.ci.reran, b.ciHistory())
	}
	b.ci.runs[6] = ciRunAt(6, "feat/x", ciHead, domain.CICompleted, domain.CIFailure)
	if _, err := b.service.Rerun(b.ctx, manager, app.CIRerunInput{TicketID: b.ticket, RunID: 6, Reason: "runner lost mid-job"}); err != nil {
		t.Fatalf("re-run a failed run: %v", err)
	}
	if !slices.Equal(b.ci.reran, []int64{6}) {
		t.Fatalf("failed-only %v, want a failed run's failed jobs re-run, not the green ones", b.ci.reran)
	}
}

func TestAJobReRunWaitsForItsRunToComplete(t *testing.T) {
	run := ciRunAt(7, "feat/x", ciHead, domain.CIInProgress, "")
	run.Jobs = []domain.CIJob{{ID: 70, RunID: 7, Name: "windows", Status: domain.CICompleted, Conclusion: domain.CIFailure}}
	b := newCIBoard(t, run)
	if _, err := b.service.Rerun(b.ctx, manager, app.CIRerunInput{TicketID: b.ticket, JobID: 70, Reason: "x"}); domain.KindOf(err) != domain.KindConflict {
		t.Fatalf("err = %v, want a conflict while the run is going", err)
	}
}

func TestOnlyTheManagerOrAPersonActsOnCIAndOnlyWithAReason(t *testing.T) {
	b := newCIBoard(t, ciRunAt(4, "feat/x", ciOlder, domain.CIQueued, ""))
	if _, err := b.service.Cancel(b.ctx, agent, app.CICancelInput{TicketID: b.ticket, RunID: 4, Reason: "x"}); domain.KindOf(err) != domain.KindForbidden {
		t.Fatalf("worker cancel err = %v, want forbidden", err)
	}
	if _, err := b.service.Cancel(b.ctx, manager, app.CICancelInput{TicketID: b.ticket, RunID: 4}); domain.KindOf(err) != domain.KindInvalid {
		t.Fatalf("no-reason cancel err = %v, want invalid", err)
	}
	plain := b.fixture.ticket("ordinary", b.repo.ID)
	if _, err := b.service.Cancel(b.ctx, manager, app.CICancelInput{TicketID: plain, RunID: 4, Reason: "x"}); domain.KindOf(err) != domain.KindForbidden {
		t.Fatalf("manager on an ordinary ticket err = %v, want forbidden", err)
	}
	if len(b.ci.cancelled) != 0 {
		t.Fatalf("cancelled %v, want nothing", b.ci.cancelled)
	}
}

func TestCIStatusShowsTheTicketsRunsWithTheCurrentHead(t *testing.T) {
	b := newCIBoard(t, ciRunAt(5, "feat/x", ciHead, domain.CICompleted, domain.CICancelled), ciRunAt(4, "feat/x", ciOlder, domain.CICompleted, domain.CISuccess))
	view, err := b.service.Status(b.ctx, manager, app.CITarget{TicketID: b.ticket})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if view.Branch != "feat/x" || view.HeadSHA != ciHead || len(view.Runs) != 2 || view.Runs[0].ID != 5 {
		t.Fatalf("view = %+v, want feat/x at the head with runs 5 then 4", view)
	}
}

func TestCISensorsFlagACancelledHeadButNotOneTheBoardCancelled(t *testing.T) {
	b := newCIBoard(t, ciRunAt(5, "feat/x", ciHead, domain.CICompleted, domain.CICancelled), ciRunAt(4, "feat/x", ciOlder, domain.CIQueued, ""))
	b.ci.runnersErr = errors.New("the token was rejected (HTTP 403)")
	result, err := b.service.Flags(b.ctx, manager, app.RepoScope{}, 0)
	if err != nil {
		t.Fatalf("flags: %v", err)
	}
	if len(result.Flags) != 1 || result.Flags[0].Kind != domain.FlagCICancelled || result.Flags[0].TicketID != b.ticket {
		t.Fatalf("flags = %+v, want one ci_cancelled flag on the ticket", result.Flags)
	}
	if len(result.Notes) != 1 || !strings.Contains(result.Notes[0], "runners not readable") {
		t.Fatalf("notes = %v, want the unreadable runners noted, not hidden", result.Notes)
	}
	// Once the board itself cancelled the current head's run (after a newer push), it is no surprise.
	b.ci.heads["feat/x"] = "3333333333333333333333333333333333333333"
	b.ci.runs[5] = ciRunAt(5, "feat/x", ciHead, domain.CIQueued, "")
	if _, err := b.service.Cancel(b.ctx, manager, app.CICancelInput{TicketID: b.ticket, RunID: 5, Reason: "replaced"}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	b.ci.runs[5] = ciRunAt(5, "feat/x", ciHead, domain.CICompleted, domain.CICancelled)
	b.ci.heads["feat/x"] = ciHead
	if result, _ := b.service.Flags(b.ctx, manager, app.RepoScope{}, 0); len(result.Flags) != 0 {
		t.Fatalf("flags = %+v, want none for a run the board cancelled", result.Flags)
	}
}

func TestAQueuedJobStaysOnItsTicketWhenTheHeadIsUnreadable(t *testing.T) {
	b := newCIBoard(t)
	run := ciRunAt(8, "feat/x", ciHead, domain.CIQueued, "")
	run.Jobs = []domain.CIJob{{ID: 80, RunID: 8, Name: "windows", Status: domain.CIQueued, CreatedAt: b.clock.Now().Add(-2 * time.Hour)}}
	b.ci.runs[run.ID] = run
	b.ci.headErr = errors.New("the token was rejected (HTTP 403)")
	result, err := b.service.Flags(b.ctx, manager, app.RepoScope{}, 0)
	if err != nil {
		t.Fatalf("flags: %v", err)
	}
	if len(result.Flags) != 1 || result.Flags[0].Kind != domain.FlagCIQueued || result.Flags[0].TicketID != b.ticket {
		t.Fatalf("flags = %+v, want the queued job flagged on the ticket that owns the branch", result.Flags)
	}
}

func TestCISensorsRefuseAnAbsurdQueueLimit(t *testing.T) {
	b := newCIBoard(t)
	if _, err := b.service.Flags(b.ctx, manager, app.RepoScope{}, time.Minute); domain.KindOf(err) != domain.KindInvalid {
		t.Fatalf("err = %v, want a one-minute queue limit refused", err)
	}
	if _, err := b.service.Flags(b.ctx, agent, app.RepoScope{}, 0); domain.KindOf(err) != domain.KindForbidden {
		t.Fatalf("err = %v, want a worker refused", err)
	}
}
