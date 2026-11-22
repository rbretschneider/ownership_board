package app

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// Flaky-test quarantine (docs/amendments/flaky-quarantine.md). Every finished
// run records the tests it failed. A test that failed on two unrelated tickets
// and then passed on a commit it had failed is flaky: it goes on its repo's
// quarantine list, and the board files a ticket to fix it. A failure that
// another ticket also hit recently gets a flake check: a re-run of only the
// suspect tests on the same commit. The list travels with every build job; a
// repo script may skip the listed tests. A test leaves the list once its fix
// ticket has merged and it has passed QuarantineReleasePasses runs.

// runRecord is one finished verification run with a verdict.
type runRecord struct {
	ID      string
	Repo    string
	RepoID  int64
	Members []domain.BuildBatchMember
	Passed  bool
	Result  []byte
	// Only, on a flake check, are the only tests the run ran.
	Only []string
	// Job is the commit job when the run verified one commit alone.
	Job *domain.BuildJob
}

// quarantineOutcome collects what a transaction did, for after it commits.
type quarantineOutcome struct {
	events []Event
}

func repoIDOf(ctx context.Context, r Repos, ticketID int64) (int64, error) {
	t, err := r.Tickets().Get(ctx, ticketID)
	if err != nil {
		return 0, err
	}
	return t.RepoID, nil
}

// refreshProbation moves active entries whose fix ticket merged to probation.
func refreshProbation(ctx context.Context, r Repos, repoID int64, now time.Time) ([]domain.QuarantinedTest, error) {
	list, err := r.TestQuarantine().List(ctx, repoID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		q := &list[i]
		if q.Status != domain.QuarantineActive || q.FixTicketID == nil {
			continue
		}
		t, err := r.Tickets().Get(ctx, *q.FixTicketID)
		if domain.KindOf(err) == domain.KindNotFound {
			continue
		}
		if err != nil {
			return nil, err
		}
		if t.Stage.IsCompleted() {
			q.Status, q.Passes, q.UpdatedAt = domain.QuarantineProbation, 0, now
			if err = r.TestQuarantine().Save(ctx, *q); err != nil {
				return nil, err
			}
		}
	}
	return list, nil
}

// quarantinedNames returns the tests a job of the repo should skip.
func quarantinedNames(ctx context.Context, r Repos, repoID int64, now time.Time) ([]string, error) {
	if repoID <= 0 {
		return nil, nil
	}
	list, err := refreshProbation(ctx, r, repoID, now)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, q := range list {
		if q.Status == domain.QuarantineActive && len(out) < domain.MaxQuarantinedTests {
			out = append(out, q.Test)
		}
	}
	return out, nil
}

// unrelatedFailures reports the tickets of a test's failures when at least two
// failures are on different tickets in different runs.
func unrelatedFailures(failures []domain.TestFailure) ([]int64, bool) {
	var tickets []int64
	unrelated := false
	for i, a := range failures {
		if !slices.Contains(tickets, a.TicketID) {
			tickets = append(tickets, a.TicketID)
		}
		for _, b := range failures[:i] {
			if a.TicketID != b.TicketID && a.RunID != b.RunID {
				unrelated = true
			}
		}
	}
	slices.Sort(tickets)
	return tickets, unrelated
}

// recordRun applies a finished run to the failure record and the quarantine list.
func (s *BuildJobService) recordRun(ctx context.Context, r Repos, run runRecord, now time.Time, out *quarantineOutcome) error {
	if run.RepoID <= 0 || len(run.Members) == 0 {
		return nil
	}
	list, err := refreshProbation(ctx, r, run.RepoID, now)
	if err != nil {
		return err
	}
	entry := map[string]*domain.QuarantinedTest{}
	for i := range list {
		entry[list[i].Test] = &list[i]
	}
	if !run.Passed {
		return s.recordFailedRun(ctx, r, run, entry, now, out)
	}
	// A passing run ran every test that is not actively quarantined, or only
	// the flake check's tests.
	ran := func(test string) bool {
		if len(run.Only) > 0 {
			return slices.Contains(run.Only, test)
		}
		q := entry[test]
		return q == nil || q.Status != domain.QuarantineActive
	}
	for _, m := range run.Members {
		failed, err := r.TestQuarantine().FailuresAt(ctx, run.Repo, m.SHA)
		if err != nil {
			return err
		}
		for _, f := range failed {
			if f.RunID == run.ID || !ran(f.Test) {
				continue
			}
			if q := entry[f.Test]; q != nil && q.Status != domain.QuarantineReleased {
				continue
			}
			history, err := r.TestQuarantine().FailuresOf(ctx, run.Repo, f.Test, time.Time{})
			if err != nil {
				return err
			}
			tickets, unrelated := unrelatedFailures(history)
			if !unrelated {
				continue
			}
			evidence, _ := json.Marshal(domain.QuarantineEvidence{Tickets: tickets, Failures: len(history), PassedOn: m.SHA, PassRun: run.ID})
			q := domain.QuarantinedTest{RepoID: run.RepoID, Test: f.Test, Status: domain.QuarantineActive, Source: domain.QuarantineDetected,
				AddedBy: "board", Reason: fmt.Sprintf("failed on %d tickets, then passed on commit %s it had failed", len(tickets), short(m.SHA)),
				Evidence: evidence, CreatedAt: now, UpdatedAt: now}
			if err = s.fileFix(ctx, r, &q, out); err != nil {
				return err
			}
			if err = r.TestQuarantine().Save(ctx, q); err != nil {
				return err
			}
			entry[q.Test] = &q
			if err = noteBatch(ctx, r, m.TicketID, now, run.ID, fmt.Sprintf("Test %q failed on commit %s and then passed on it; it failed on tickets %s too, so the board quarantined it as flaky.", f.Test, short(m.SHA), ticketList(tickets))); err != nil {
				return err
			}
		}
	}
	// Probation: each full passing run counts for every test on probation.
	if len(run.Only) > 0 {
		return nil
	}
	for _, q := range entry {
		if q.Status != domain.QuarantineProbation {
			continue
		}
		q.Passes++
		q.UpdatedAt = now
		if q.Passes >= domain.QuarantineReleasePasses {
			q.Status, q.ReleasedAt = domain.QuarantineReleased, &now
		}
		if err := r.TestQuarantine().Save(ctx, *q); err != nil {
			return err
		}
	}
	return nil
}

func (s *BuildJobService) recordFailedRun(ctx context.Context, r Repos, run runRecord, entry map[string]*domain.QuarantinedTest, now time.Time, out *quarantineOutcome) error {
	names := domain.FailedTests(run.Result)
	if len(run.Only) > 0 {
		names = slices.DeleteFunc(names, func(n string) bool { return !slices.Contains(run.Only, n) })
	}
	if len(names) == 0 {
		return nil
	}
	var rows []domain.TestFailure
	for _, m := range run.Members {
		for _, n := range names {
			rows = append(rows, domain.TestFailure{RunID: run.ID, Repo: run.Repo, TicketID: m.TicketID, SHA: m.SHA, Test: n, RecordedAt: now})
		}
	}
	if err := r.TestQuarantine().RecordFailures(ctx, rows); err != nil {
		return err
	}
	// A test on probation that fails again goes back on the list: its fix did
	// not hold, so the board files a new one.
	for _, n := range names {
		if q := entry[n]; q != nil && q.Status == domain.QuarantineProbation {
			q.Status, q.Passes, q.UpdatedAt, q.FixTicketID = domain.QuarantineActive, 0, now, nil
			q.Reason = clip(q.Reason+"; failed again after its fix merged", 2000)
			if err := s.fileFix(ctx, r, q, out); err != nil {
				return err
			}
			if err := r.TestQuarantine().Save(ctx, *q); err != nil {
				return err
			}
		}
	}
	if run.Job == nil || run.Job.IsFlakeCheck() {
		return nil
	}
	// A failure another ticket also hit recently may be a flake: re-run only
	// those tests on the same commit.
	var suspects []string
	for _, n := range names {
		if q := entry[n]; q != nil && q.Status != domain.QuarantineReleased {
			continue
		}
		recent, err := r.TestQuarantine().FailuresOf(ctx, run.Repo, n, now.Add(-domain.FlakeCheckWindow))
		if err != nil {
			return err
		}
		if _, unrelated := unrelatedFailures(recent); unrelated {
			suspects = append(suspects, n)
		}
	}
	if len(suspects) == 0 {
		return nil
	}
	slices.Sort(suspects)
	return s.queueFlakeCheck(ctx, r, *run.Job, suspects, now)
}

// queueFlakeCheck queues a re-run of only the given tests on the job's commit,
// reusing its bundle. One check per commit and test set.
func (s *BuildJobService) queueFlakeCheck(ctx context.Context, r Repos, of domain.BuildJob, tests []string, now time.Time) error {
	id := domain.FlakeCheckID(of, tests)
	if _, err := r.BuildJobs().Get(ctx, id); err == nil {
		return nil
	} else if domain.KindOf(err) != domain.KindNotFound {
		return err
	}
	bundleOf := of.ID
	if of.BundleOf != "" {
		bundleOf = of.BundleOf
	}
	check := domain.BuildJob{ID: id, TicketID: of.TicketID, Repo: of.Repo, SHA: of.SHA, Preset: of.Preset, Target: of.Target,
		Status: "queued", Attempt: 1, CreatedAt: now, BundleBytes: of.BundleBytes, BundleSHA256: of.BundleSHA256,
		Kind: domain.JobKindFlakeCheck, OnlyTests: tests, BundleOf: bundleOf}
	if err := r.BuildJobs().Insert(ctx, check); err != nil {
		return err
	}
	return noteBatch(ctx, r, of.TicketID, now, id, fmt.Sprintf("Commit %s failed %s, which other tickets failed recently too. The board re-runs only those tests on the same commit to see whether they are flaky; this ticket's verdict stands.", short(of.SHA), quoteList(tests)))
}

// fileFix files the autonomous fix ticket for a newly quarantined test that
// has none. Without a board service (tests of the job service alone) the
// entry stays without one.
func (s *BuildJobService) fileFix(ctx context.Context, r Repos, q *domain.QuarantinedTest, out *quarantineOutcome) error {
	if q.FixTicketID != nil || s.Board == nil {
		return nil
	}
	// An open ticket that already names the test is its fix: a second ticket
	// would edit the same test in parallel (2026-11-20: makercad #253 was filed
	// for F-09 while #250, the F-09 fix, was in Windows verification).
	existing, isFound, err := openTicketNaming(ctx, r, q.RepoID, q.Test)
	if err != nil {
		return err
	}
	if isFound {
		q.FixTicketID = &existing
		note := domain.NewEntry(existing, domain.TimelineComment, domain.SystemActor, q.CreatedAt).WithBody(
			"The board quarantined `" + q.Test + "` as flaky: " + q.Reason + ". This ticket already names that test, so it is " +
				"the fix and no separate flaky-test ticket was filed. Verification skips the test until this ticket merges " +
				"and the test passes " + fmt.Sprint(domain.QuarantineReleasePasses) + " runs.")
		return r.Timeline().Append(ctx, &note)
	}
	evidence, err := failureEvidence(ctx, r, q.RepoID, q.Test)
	if err != nil {
		return err
	}
	draft := domain.AutonomousDraft{
		TicketDraft: domain.TicketDraft{RepoID: q.RepoID, Kind: string(domain.KindBug), Title: clip("Fix flaky test: "+q.Test, 200),
			Body: "The board quarantined this test as flaky: " + q.Reason + "."},
		Spec: fixSpec(*q, evidence),
	}
	ticket, err := domain.NewBoardFiledAutonomousTicket(draft, q.CreatedAt)
	if err != nil {
		return err
	}
	detail, events, err := s.Board.insertAutonomousInTx(ctx, r, domain.SystemActor, ticket, nil, func(id int64) domain.Decision {
		return domain.DecisionFromAgreedSpec(id, ticket.SpecRevision, domain.SystemActor, ticket.CreatedAt)
	}, atBottom)
	if err != nil {
		return err
	}
	id := detail.Ticket.ID
	q.FixTicketID = &id
	out.events = append(out.events, events...)
	return nil
}

// openTicketNaming finds the oldest unfinished ticket in the repo whose title,
// request or spec names the test.
func openTicketNaming(ctx context.Context, r Repos, repoID int64, test string) (int64, bool, error) {
	tickets, err := r.Tickets().List(ctx, TicketFilter{IsScoped: true, RepoIDs: []int64{repoID}})
	if err != nil {
		return 0, false, err
	}
	var found int64
	for _, t := range tickets {
		if t.Stage.IsCompleted() || (found != 0 && t.ID > found) {
			continue
		}
		if strings.Contains(t.Title, test) || strings.Contains(t.Body, test) || strings.Contains(t.Spec, test) {
			found = t.ID
		}
	}
	return found, found != 0, nil
}

// Failure evidence copied into a flaky-test fix ticket: the newest failures'
// log lines around the test, within a byte budget.
const (
	evidenceRuns    = 2
	evidenceBytes   = 4000
	evidenceContext = 6
)

// failureEvidence quotes the failing log lines of the test's newest recorded
// failures, so the fix ticket carries what the build host saw (2026-11-22:
// workers on makercad #262, #264 and #266 could not name a cause because the
// spec held only ticket ids, and shipped "cause unconfirmed" PRs). Missing or
// unreadable jobs are skipped; an empty result means no log survived.
func failureEvidence(ctx context.Context, r Repos, repoID int64, test string) (string, error) {
	repo, err := r.CodeRepos().Get(ctx, repoID)
	if err != nil {
		return "", err
	}
	failures, err := r.TestQuarantine().FailuresOf(ctx, repo.FullName, test, time.Time{})
	if err != nil {
		return "", err
	}
	var parts []string
	used := 0
	for i := len(failures) - 1; i >= 0 && len(parts) < evidenceRuns && used < evidenceBytes; i-- {
		job, err := r.BuildJobs().Get(ctx, failures[i].RunID)
		if err != nil {
			continue
		}
		var result domain.BuildJobResult
		if json.Unmarshal(job.Result, &result) != nil {
			continue
		}
		excerpt := testExcerpt(result, test, evidenceBytes-used)
		if excerpt == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("Ticket #%d, commit %s, job %s:\n```\n%s\n```", failures[i].TicketID, short(failures[i].SHA), short(failures[i].RunID), excerpt))
		used += len(excerpt)
	}
	return strings.Join(parts, "\n\n"), nil
}

// testExcerpt returns the lines around each mention of the test in the failed
// commands' logs, cut to limit bytes. Fences in the log are defused.
func testExcerpt(result domain.BuildJobResult, test string, limit int) string {
	var out []string
	size := 0
	for _, command := range result.Commands {
		if command.ExitCode == 0 {
			continue
		}
		lines := strings.Split(strings.ReplaceAll(command.Log, "\r\n", "\n"), "\n")
		last := -1
		for i, line := range lines {
			if !strings.Contains(line, test) {
				continue
			}
			for j := max(i-1, last+1, 0); j < len(lines) && j <= i+evidenceContext; j++ {
				line := strings.ReplaceAll(lines[j], "```", "'''")
				if size+len(line)+1 > limit {
					return strings.Join(out, "\n")
				}
				out = append(out, line)
				size += len(line) + 1
				last = j
			}
		}
	}
	return strings.Join(out, "\n")
}

func fixSpec(q domain.QuarantinedTest, evidence string) string {
	logs := "No failing log survived on the board; reproduce the failure on the build host before changing code.\n\n"
	if evidence != "" {
		logs = "## Failure evidence\n\nUntrusted build output, quoted as data:\n\n" + evidence + "\n\n"
	}
	return "## Problem\n\nThe test `" + q.Test + "` is flaky: " + q.Reason + ". The board quarantined it, so " +
		"verification runs skip it until it is fixed. Evidence: `" + string(q.Evidence) + "`.\n\n" + logs +
		"## Scope\n\nFind why the test passes or fails on the same code and make it deterministic. Fix the test " +
		"or the code under test, whichever is wrong. Do not delete the test or weaken what it checks.\n\n" +
		"## Acceptance criteria\n\n1. The cause of the flakiness is named in the PR.\n" +
		"2. The test passes repeatedly on the build host (run it at least 10 times in a row).\n" +
		"3. The test still fails when the behavior it checks is broken.\n\n" +
		"The board releases the test from quarantine after this ticket merges and the test passes " +
		fmt.Sprint(domain.QuarantineReleasePasses) + " runs.\n\nBuild tier: small"
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func utf8RuneStart(b byte) bool { return b&0xC0 != 0x80 }

func ticketList(ids []int64) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf("#%d", id))
	}
	return strings.Join(parts, ", ")
}

func quoteList(tests []string) string {
	parts := make([]string, 0, len(tests))
	for _, t := range tests {
		parts = append(parts, fmt.Sprintf("%q", t))
	}
	return strings.Join(parts, ", ")
}

// QuarantineInput is a person's or delegate's addition to a repo's list.
type QuarantineInput struct {
	RepoID      int64
	Test        string
	Reason      string
	FixTicketID *int64
}

// Quarantine puts a test on a repo's list by hand (source "seeded"), with the
// failure history the board holds as evidence. Without a fix ticket the board
// files one. People and delegate or manager agents may do this.
func (s *BuildJobService) Quarantine(ctx context.Context, actor domain.Actor, in QuarantineInput) (domain.QuarantinedTest, error) {
	var q domain.QuarantinedTest
	if !actor.IsHuman() && !actor.MayFileAutonomous() {
		return q, domain.Forbiddenf("only a person or a delegate agent can quarantine a test")
	}
	if !domain.ValidTestName(in.Test) {
		return q, domain.Invalidf("test must be one line of 1 to %d bytes", domain.MaxTestNameBytes)
	}
	if len(in.Reason) > 2000 {
		return q, domain.Invalidf("reason is at most 2000 bytes")
	}
	out := &quarantineOutcome{}
	err := s.store.Write(ctx, func(r Repos) error {
		repo, err := r.CodeRepos().Get(ctx, in.RepoID)
		if err != nil {
			return err
		}
		now := s.now()
		if existing, err := r.TestQuarantine().Get(ctx, repo.ID, in.Test); err == nil && existing.Status != domain.QuarantineReleased {
			q = existing
			return domain.Conflictf("test is already quarantined (%s)", existing.Status)
		} else if err != nil && domain.KindOf(err) != domain.KindNotFound {
			return err
		}
		if in.FixTicketID != nil {
			t, err := r.Tickets().Get(ctx, *in.FixTicketID)
			if err != nil {
				return err
			}
			if t.RepoID != repo.ID {
				return domain.Invalidf("the fix ticket is in another repo")
			}
		}
		history, err := r.TestQuarantine().FailuresOf(ctx, repo.FullName, in.Test, time.Time{})
		if err != nil {
			return err
		}
		tickets, _ := unrelatedFailures(history)
		evidence, _ := json.Marshal(domain.QuarantineEvidence{Tickets: tickets, Failures: len(history)})
		reason := in.Reason
		if reason == "" {
			reason = fmt.Sprintf("seeded by %s; failed on %d tickets", actor.Name, len(tickets))
		}
		q = domain.QuarantinedTest{RepoID: repo.ID, Test: in.Test, Status: domain.QuarantineActive, Source: domain.QuarantineSeeded,
			AddedBy: actor.Name, Reason: reason, Evidence: evidence, FixTicketID: in.FixTicketID, CreatedAt: now, UpdatedAt: now}
		if err = s.fileFix(ctx, r, &q, out); err != nil {
			return err
		}
		return r.TestQuarantine().Save(ctx, q)
	})
	if err == nil {
		s.publish(ctx, out)
	}
	return q, err
}

// ReleaseQuarantine takes a test off a repo's list by hand.
func (s *BuildJobService) ReleaseQuarantine(ctx context.Context, actor domain.Actor, repoID int64, test string) (domain.QuarantinedTest, error) {
	var q domain.QuarantinedTest
	if !actor.IsHuman() && !actor.MayFileAutonomous() {
		return q, domain.Forbiddenf("only a person or a delegate agent can release a test")
	}
	err := s.store.Write(ctx, func(r Repos) error {
		var err error
		if q, err = r.TestQuarantine().Get(ctx, repoID, test); err != nil {
			return err
		}
		now := s.now()
		q.Status, q.ReleasedAt, q.UpdatedAt = domain.QuarantineReleased, &now, now
		return r.TestQuarantine().Save(ctx, q)
	})
	return q, err
}

// QuarantineList returns a repo's quarantine entries, released ones last.
func (s *BuildJobService) QuarantineList(ctx context.Context, repoID int64) ([]domain.QuarantinedTest, error) {
	var list []domain.QuarantinedTest
	err := s.store.Write(ctx, func(r Repos) error {
		if _, err := r.CodeRepos().Get(ctx, repoID); err != nil {
			return err
		}
		var err error
		list, err = refreshProbation(ctx, r, repoID, s.now())
		return err
	})
	return list, err
}

// BackfillTestFailures records the failed tests of every stored failed run,
// once, when the record is empty: the detector then starts with the history.
func (s *BuildJobService) BackfillTestFailures(ctx context.Context) error {
	return s.store.Write(ctx, func(r Repos) error {
		if n, err := r.TestQuarantine().CountFailures(ctx); err != nil || n > 0 {
			return err
		}
		jobs, err := r.BuildJobs().FailedVerdicts(ctx)
		if err != nil {
			return err
		}
		for _, j := range jobs {
			if domain.JobResultBlamesHost(j.Result) {
				continue
			}
			at := j.CreatedAt
			if j.FinishedAt != nil {
				at = *j.FinishedAt
			}
			var rows []domain.TestFailure
			for _, n := range domain.FailedTests(j.Result) {
				rows = append(rows, domain.TestFailure{RunID: j.ID, Repo: j.Repo, TicketID: j.TicketID, SHA: j.SHA, Test: n, RecordedAt: at})
			}
			if err = r.TestQuarantine().RecordFailures(ctx, rows); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *BuildJobService) publish(ctx context.Context, out *quarantineOutcome) {
	if s.Board != nil && len(out.events) > 0 {
		s.Board.writer.sink.Publish(ctx, out.events)
	}
}
