package app_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

const flaky = "suite:sometimes red"

func failing(tests ...string) string {
	log := "The following tests FAILED:\n"
	for i, t := range tests {
		log += fmt.Sprintf("\t%d - %s (Failed)\n", i+1, t)
	}
	raw, _ := json.Marshal(domain.BuildJobResult{Commands: []domain.BuildJobCommand{{Label: "test", ExitCode: 8, Log: log}}})
	return string(raw)
}

// quarantineRig is a batch rig with batching off, an agent that runs flake
// checks, and the board service filing fix tickets.
func quarantineRig(t *testing.T, tickets int) *batchRig {
	t.Helper()
	r := batchFixture(t, tickets, 4)
	for _, q := range []string{`UPDATE repos SET batch_verify=0`, `UPDATE build_hosts SET agent_version='1.2', max_parallel=1`} {
		if _, err := r.f.store.SQL().Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	r.s.Board = r.f.board
	return r
}

// runAlone claims the next job and completes it with the result.
func (r *batchRig) runAlone(status, result string) domain.BuildJob {
	r.t.Helper()
	j := r.claim()
	if j == nil || j.Batch != nil {
		r.t.Fatalf("want a single job, got %+v", j)
	}
	r.complete(j.ID, j.Attempt, status, result)
	return *j
}

func (r *batchRig) quarantine() []domain.QuarantinedTest {
	r.t.Helper()
	list, err := r.s.QuarantineList(r.f.ctx, 1)
	if err != nil {
		r.t.Fatal(err)
	}
	return list
}

func TestAFailureAnotherTicketHadGetsAFlakeCheckOfOnlyThatTest(t *testing.T) {
	r := quarantineRig(t, 2)
	r.runAlone("failed", failing(flaky, "suite:own bug"))
	second := r.runAlone("failed", failing(flaky))
	// A newer commit on #2 does not throw the check away.
	r.submit(2, 2)
	check := r.claim()
	if check == nil || !check.IsFlakeCheck() || check.SHA != second.SHA || !slices.Equal(check.OnlyTests, []string{flaky}) {
		t.Fatalf("want a flake check of %q on #2's commit, got %+v", flaky, check)
	}
	// It reuses the failed job's bundle and never blocks the ticket's worker.
	rc, _, err := r.s.OpenBundle(r.f.ctx, r.host, check.ID)
	if err != nil {
		t.Fatal(err)
	}
	rc.Close()
	// The check passes: the test is flaky. #2's verdict stays failed.
	r.complete(check.ID, 1, "passed", passed)
	list := r.quarantine()
	if len(list) != 1 || list[0].Test != flaky || list[0].Status != domain.QuarantineActive || list[0].Source != domain.QuarantineDetected || list[0].FixTicketID == nil {
		t.Fatalf("quarantine = %+v", list)
	}
	if v, _ := r.verdict(second); v.Status != domain.VerdictFailed {
		t.Fatalf("a flake check must not change the verdict: %+v", v)
	}
	fix, err := r.f.board.Ticket(r.f.ctx, *list[0].FixTicketID)
	if err != nil || !fix.Ticket.IsAutonomous || !strings.Contains(fix.Ticket.Title, flaky) || fix.Ticket.RepoID != 1 {
		t.Fatalf("fix ticket = %+v, %v", fix.Ticket, err)
	}
	// Every later job carries the list.
	r.submit(1, 2)
	next := r.claim()
	if next == nil || !slices.Equal(next.Quarantine, []string{flaky}) {
		t.Fatalf("assignment quarantine = %+v", next)
	}
}

// 2026-11-20: makercad #253 was filed for the F-09 perf test while #250, which
// names that test in its spec, was already fixing it.
func TestAnOpenTicketNamingTheTestBecomesItsFixInsteadOfANewTicket(t *testing.T) {
	r := quarantineRig(t, 2)
	if _, err := r.f.store.SQL().Exec(`UPDATE tickets SET spec = spec || ? WHERE id=2`, "\nFails: `"+flaky+"`."); err != nil {
		t.Fatal(err)
	}
	var before int
	if err := r.f.store.SQL().QueryRow(`SELECT count(*) FROM tickets`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	r.runAlone("failed", failing(flaky))
	r.runAlone("failed", failing(flaky))
	check := r.claim()
	if check == nil || !check.IsFlakeCheck() {
		t.Fatalf("want a flake check, got %+v", check)
	}
	r.complete(check.ID, 1, "passed", passed)
	list := r.quarantine()
	if len(list) != 1 || list[0].FixTicketID == nil || *list[0].FixTicketID != 2 {
		t.Fatalf("quarantine = %+v, want #2 as the fix", list)
	}
	var after int
	if err := r.f.store.SQL().QueryRow(`SELECT count(*) FROM tickets`).Scan(&after); err != nil || after != before {
		t.Fatalf("tickets %d -> %d (%v), want no new ticket", before, after, err)
	}
	var notes int
	if err := r.f.store.SQL().QueryRow(`SELECT count(*) FROM timeline WHERE ticket_id=2 AND type='comment' AND body LIKE '%already names that test%'`).Scan(&notes); err != nil || notes != 1 {
		t.Fatalf("notes on #2 = %d (%v), want 1", notes, err)
	}
}

func TestAFailureOnlyOneTicketHasIsNotChecked(t *testing.T) {
	r := quarantineRig(t, 1)
	first := r.runAlone("failed", failing(flaky))
	r.submit(1, 2)
	r.runAlone("failed", failing(flaky))
	if next := r.claim(); next != nil {
		t.Fatalf("a test failing on one ticket is a real failure; got %+v", next)
	}
	_ = first
	if len(r.quarantine()) != 0 {
		t.Fatal("nothing should be quarantined")
	}
}

func TestAnOlderAgentGetsNoFlakeCheck(t *testing.T) {
	r := quarantineRig(t, 2)
	r.runAlone("failed", failing(flaky))
	r.runAlone("failed", failing(flaky))
	if _, err := r.f.store.SQL().Exec(`UPDATE build_hosts SET agent_version='1.1'`); err != nil {
		t.Fatal(err)
	}
	if next := r.claim(); next != nil {
		t.Fatalf("a 1.1 agent would run the whole suite for a flake check; got %+v", next)
	}
}

func TestAQuarantinedTestLeavesAfterItsFixMergesAndItPasses(t *testing.T) {
	r := quarantineRig(t, 3)
	fix := int64(3)
	if _, err := r.s.Quarantine(r.f.ctx, owner, app.QuarantineInput{RepoID: 1, Test: flaky, FixTicketID: &fix}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.f.store.SQL().Exec(`UPDATE tickets SET stage='done', lane='merged' WHERE id=3`); err != nil {
		t.Fatal(err)
	}
	// Probation: the test runs again (not in the job's list).
	j := r.claim()
	if len(j.Quarantine) != 0 {
		t.Fatalf("a test on probation must run; quarantine = %v", j.Quarantine)
	}
	r.complete(j.ID, 1, "passed", passed)
	if q := r.quarantine()[0]; q.Status != domain.QuarantineProbation || q.Passes != 1 {
		t.Fatalf("after one pass: %+v", q)
	}
	// A failure on probation puts it back.
	r.runAlone("failed", failing(flaky))
	if q := r.quarantine()[0]; q.Status != domain.QuarantineActive {
		t.Fatalf("after a failure on probation: %+v", q)
	}
	if _, err := r.f.store.SQL().Exec(`UPDATE test_quarantine SET status='probation', passes=0`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < domain.QuarantineReleasePasses; i++ {
		r.submit(1, 10+i)
		r.runAlone("passed", passed)
	}
	if q := r.quarantine()[0]; q.Status != domain.QuarantineReleased || q.ReleasedAt == nil {
		t.Fatalf("after %d passes: %+v", domain.QuarantineReleasePasses, q)
	}
}

func TestSeedingAQuarantineNeedsAPersonOrDelegate(t *testing.T) {
	r := quarantineRig(t, 2)
	worker := domain.Actor{Kind: domain.ActorAgent, ID: 9, Name: "worker"}
	if _, err := r.s.Quarantine(r.f.ctx, worker, app.QuarantineInput{RepoID: 1, Test: flaky}); domain.KindOf(err) != domain.KindForbidden {
		t.Fatalf("a plain agent token must not quarantine: %v", err)
	}
	delegate := domain.Actor{Kind: domain.ActorAgent, ID: 8, Name: "Clod", IsDelegate: true}
	q, err := r.s.Quarantine(r.f.ctx, delegate, app.QuarantineInput{RepoID: 1, Test: flaky, Reason: "fails on #1 and #2"})
	if err != nil || q.Source != domain.QuarantineSeeded || q.AddedBy != "Clod" || q.FixTicketID == nil {
		t.Fatalf("seeded = %+v, %v", q, err)
	}
	if _, err := r.s.Quarantine(r.f.ctx, delegate, app.QuarantineInput{RepoID: 1, Test: flaky}); domain.KindOf(err) != domain.KindConflict {
		t.Fatalf("a second quarantine of the same test: %v", err)
	}
	if _, err := r.s.Quarantine(r.f.ctx, delegate, app.QuarantineInput{RepoID: 1, Test: "bad\nname"}); domain.KindOf(err) != domain.KindInvalid {
		t.Fatalf("a multi-line name: %v", err)
	}
}

func TestBackfillReadsStoredFailures(t *testing.T) {
	r := quarantineRig(t, 2)
	r.s.Board = nil
	r.runAlone("failed", failing(flaky))
	if _, err := r.f.store.SQL().Exec(`DELETE FROM test_failures`); err != nil {
		t.Fatal(err)
	}
	if err := r.s.BackfillTestFailures(r.f.ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := r.f.store.SQL().QueryRow(`SELECT count(*) FROM test_failures WHERE test=?`, flaky).Scan(&n); err != nil || n != 1 {
		t.Fatalf("backfilled %d (%v), want 1", n, err)
	}
}
