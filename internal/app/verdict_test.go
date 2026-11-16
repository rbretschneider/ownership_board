package app_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

// verdictFixture is jobFixture with ticket 1 leased to agent, and a way to run
// a job to the end without a bundle (bundle storage fails on Windows in tests).
type verdictFixture struct {
	*fixture
	jobs     *app.BuildJobService
	verdicts *app.VerdictService
	host     domain.BuildHost
	base     domain.BuildJobSubmission
}

func newVerdictFixture(t *testing.T) verdictFixture {
	t.Helper()
	f, jobs, m, h := jobFixture(t)
	if _, err := f.store.SQL().Exec(`UPDATE tickets SET lease_holder=?, lease_expires_at='2099-01-01T00:00:00.000Z' WHERE id=1`, agent.Name); err != nil {
		t.Fatal(err)
	}
	return verdictFixture{fixture: f, jobs: jobs, verdicts: app.NewVerdictService(f.store, f.clock), host: h, base: m}
}

func sha(n int) string { return fmt.Sprintf("%040x", n) }

// finish queues a job for commit n, lets the host claim it and report the result.
func (v verdictFixture) finish(t *testing.T, n int, status, result string) domain.BuildJob {
	t.Helper()
	m := v.base
	m.SHA = sha(n)
	job := domain.BuildJob{ID: m.ID(), TicketID: m.TicketID, Repo: m.Repo, SHA: m.SHA, Preset: m.Preset, Target: m.Target,
		Status: "queued", Attempt: 1, CreatedAt: v.clock.Now(), BundleBytes: m.BundleBytes, BundleSHA256: m.BundleSHA256}
	if err := v.store.Write(v.ctx, func(r app.Repos) error { return r.BuildJobs().Insert(v.ctx, job) }); err != nil {
		t.Fatal(err)
	}
	who := app.AuthenticatedBuildHost{ID: v.host.ID}
	if got, err := v.jobs.Claim(v.ctx, who); err != nil || got == nil || got.ID != job.ID {
		t.Fatalf("claim: %v %v", got, err)
	}
	done, err := v.jobs.Complete(v.ctx, who, job.ID, domain.BuildJobCompletion{Attempt: 1, Status: status, Result: []byte(result)})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	return done
}

const (
	passedResult = `{"commands":[{"label":"build","exit_code":0,"log":"ok"}]}`
	failedResult = `{"commands":[{"label":"test","exit_code":1,"log":"1 failed"}]}`
	infraResult  = `{"commands":[],"failure_kind":"infrastructure"}`
)

func TestAFinishedJobWritesItsVerdict(t *testing.T) {
	v := newVerdictFixture(t)
	passed := v.finish(t, 1, "passed", passedResult)
	v.clock.Advance(time.Second)
	failed := v.finish(t, 2, "failed", failedResult)
	v.finish(t, 3, "failed", infraResult)

	got, err := v.verdicts.ForCommit(v.ctx, "example/tools", strings.ToUpper(sha(1)))
	if err != nil || len(got) != 1 {
		t.Fatalf("verdicts of the passed commit: %v %v", got, err)
	}
	want := domain.Verdict{Repo: "example/tools", Target: "linux:cmake", SHA: sha(1), Status: domain.VerdictPassed, JobID: passed.ID,
		TicketID: 1, Source: domain.VerdictFromBuildJob, ReportedBy: fmt.Sprintf("build-host/%d", v.host.ID), RecordedAt: *passed.FinishedAt}
	if got[0] != want {
		t.Fatalf("verdict\n got %+v\nwant %+v", got[0], want)
	}
	if got, _ := v.verdicts.ForCommit(v.ctx, "example/tools", sha(2)); len(got) != 1 || got[0].Status != domain.VerdictFailed || got[0].JobID != failed.ID {
		t.Fatalf("failed commit: %+v", got)
	}
	if got, _ := v.verdicts.ForCommit(v.ctx, "example/tools", sha(3)); len(got) != 0 {
		t.Fatalf("an infrastructure failure is no verdict on the commit: %+v", got)
	}

	byTicket, err := v.verdicts.ForTicket(v.ctx, 1)
	if err != nil || len(byTicket) != 2 || byTicket[0].SHA != sha(2) || byTicket[1].SHA != sha(1) {
		t.Fatalf("ticket verdicts, newest first: %+v %v", byTicket, err)
	}
	// The ticket's summary carries the newest verdict per target, for the
	// merge sweep and the manager to read without another call.
	detail := v.detail(1)
	if len(detail.Verdicts) != 1 || detail.Verdicts[0].SHA != sha(2) {
		t.Fatalf("detail verdicts: %+v", detail.Verdicts)
	}
	board, err := v.board.Board(v.ctx, app.RepoScope{})
	if err != nil || len(board.Tickets) != 1 || len(board.Tickets[0].Verdicts) != 1 || board.Tickets[0].Verdicts[0].SHA != sha(2) {
		t.Fatalf("board verdicts: %+v %v", board.Tickets, err)
	}
	brief, err := v.agents.Brief(v.ctx, 1)
	if err != nil || !strings.Contains(brief, "Verdict on linux:cmake: commit `"+sha(2)+"` failed") {
		t.Fatalf("brief lacks the verdict: %v\n%s", err, brief)
	}
}

func TestACleanMergeNeedsAPassedBase(t *testing.T) {
	v := newVerdictFixture(t)
	report := domain.CleanMergeReport{TicketID: 1, Repo: "example/tools", SHA: sha(10), MergeOf: sha(1)}

	_, err := v.verdicts.ReportCleanMerge(v.ctx, agent, report)
	requireErrorKind(t, err, domain.KindConflict)
	v.finish(t, 2, "failed", failedResult)
	report.MergeOf = sha(2)
	_, err = v.verdicts.ReportCleanMerge(v.ctx, agent, report)
	requireErrorKind(t, err, domain.KindConflict)
	if got, _ := v.verdicts.ForCommit(v.ctx, "example/tools", sha(10)); len(got) != 0 {
		t.Fatalf("a refused clean merge left a verdict: %+v", got)
	}

	base := v.finish(t, 1, "passed", passedResult)
	report.MergeOf = sha(1)
	_, err = v.verdicts.ReportCleanMerge(v.ctx, other, report)
	requireErrorKind(t, err, domain.KindConflict) // other does not hold the lease
	_, err = v.verdicts.ReportCleanMerge(v.ctx, owner, report)
	requireErrorKind(t, err, domain.KindForbidden)
	wrongTarget := report
	wrongTarget.Target = "windows"
	_, err = v.verdicts.ReportCleanMerge(v.ctx, agent, wrongTarget)
	requireErrorKind(t, err, domain.KindConflict)

	got, err := v.verdicts.ReportCleanMerge(v.ctx, agent, report)
	if err != nil || len(got) != 1 {
		t.Fatalf("clean merge: %+v %v", got, err)
	}
	want := domain.Verdict{Repo: "example/tools", Target: "linux:cmake", SHA: sha(10), Status: domain.VerdictCleanMerge, MergeOf: sha(1),
		JobID: base.ID, TicketID: 1, Source: domain.VerdictFromCleanMerge, ReportedBy: agent.Name, RecordedAt: v.clock.Now().UTC()}
	if got[0] != want {
		t.Fatalf("verdict\n got %+v\nwant %+v", got[0], want)
	}
	if !got[0].IsPassed() {
		t.Fatal("a clean merge of a passed commit counts as passed")
	}
	// A clean merge of a clean merge still rests on the original run.
	chained, err := v.verdicts.ReportCleanMerge(v.ctx, agent, domain.CleanMergeReport{TicketID: 1, Repo: "example/tools", Target: "linux:cmake", SHA: sha(11), MergeOf: sha(10)})
	if err != nil || len(chained) != 1 || chained[0].JobID != base.ID {
		t.Fatalf("chained clean merge: %+v %v", chained, err)
	}
	// Reporting a failed commit as a clean merge cannot overrule its run.
	_, err = v.verdicts.ReportCleanMerge(v.ctx, agent, domain.CleanMergeReport{TicketID: 1, Repo: "example/tools", SHA: sha(2), MergeOf: sha(1)})
	requireErrorKind(t, err, domain.KindConflict)
}

func TestVerdictTargetKeys(t *testing.T) {
	for in, want := range map[string]string{"windows": "windows", "windows:msvc,cuda": "windows:cuda,msvc", " linux:cmake ": "linux:cmake"} {
		if got, err := domain.NormalizeVerdictTarget(in); err != nil || got != want {
			t.Errorf("%q: got %q %v, want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "beos", "windows:", "windows:Bad Cap"} {
		if _, err := domain.NormalizeVerdictTarget(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
