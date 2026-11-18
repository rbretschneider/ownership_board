package app_test

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

// batchFixture queues one job per ticket 1..n in a repo that batches up to
// size commits, on a host whose agent merges batches.
type batchRig struct {
	t    *testing.T
	f    *fixture
	s    *app.BuildJobService
	m    domain.BuildJobSubmission
	host app.AuthenticatedBuildHost
	jobs map[int64]domain.BuildJob
}

func batchFixture(t *testing.T, n, size int) *batchRig {
	t.Helper()
	f, s, m, h := jobFixture(t)
	for _, q := range []string{
		fmt.Sprintf(`UPDATE repos SET batch_verify=1, batch_max=%d WHERE id=1`, size),
		fmt.Sprintf(`UPDATE build_hosts SET agent_version='%s' WHERE id=%d`, domain.BatchAgentVersion, h.ID),
	} {
		if _, err := f.store.SQL().Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for id := 2; id <= n; id++ {
		if _, err := f.store.SQL().Exec(fmt.Sprintf(`INSERT INTO tickets(id,repo_id,kind,title,stage,lane,rank,created_by,created_at,updated_at,lane_changed_at) VALUES(%d,1,'story','Example %d','ready','todo',%d,'Owner','2026-01-01T00:00:00.000Z','2026-01-01T00:00:00.000Z','2026-01-01T00:00:00.000Z')`, id, id, id)); err != nil {
			t.Fatal(err)
		}
	}
	rig := &batchRig{t: t, f: f, s: s, m: m, host: app.AuthenticatedBuildHost{ID: h.ID}, jobs: map[int64]domain.BuildJob{}}
	for id := int64(1); id <= int64(n); id++ {
		rig.submit(id, 1)
	}
	return rig
}

func (r *batchRig) submit(ticket int64, version int) domain.BuildJob {
	r.t.Helper()
	m := r.m
	m.TicketID, m.SHA = ticket, fmt.Sprintf("%038x%02x", ticket, version)
	r.f.clock.Advance(time.Second)
	j := submitJob(r.t, r.f, r.s, m)
	r.jobs[ticket] = j
	return j
}

func (r *batchRig) claim() *domain.BuildJob {
	r.t.Helper()
	got, err := r.s.Claim(r.f.ctx, r.host)
	if err != nil {
		r.t.Fatal(err)
	}
	return got
}

func (r *batchRig) complete(id string, attempt int, status, result string) domain.BuildJob {
	r.t.Helper()
	c := domain.BuildJobCompletion{Attempt: attempt, Status: status, Result: []byte(result)}
	if err := c.Validate(); err != nil {
		r.t.Fatal(err)
	}
	j, err := r.s.Complete(r.f.ctx, r.host, id, c)
	if err != nil {
		r.t.Fatal(err)
	}
	return j
}

func (r *batchRig) job(ticket int64) domain.BuildJob {
	r.t.Helper()
	j, err := r.s.Get(r.f.ctx, r.jobs[ticket].ID)
	if err != nil {
		r.t.Fatal(err)
	}
	return j
}

func (r *batchRig) verdict(j domain.BuildJob) (domain.Verdict, bool) {
	r.t.Helper()
	var v domain.Verdict
	var found bool
	err := r.f.store.Read(r.f.ctx, func(repos app.Repos) error {
		var err error
		v, err = repos.Verdicts().Get(r.f.ctx, j.Repo, j.Target.Key(), j.SHA)
		if domain.KindOf(err) == domain.KindNotFound {
			return nil
		}
		found = err == nil
		return err
	})
	if err != nil {
		r.t.Fatal(err)
	}
	return v, found
}

func memberTickets(j *domain.BuildJob) []int64 {
	if j == nil || j.Batch == nil {
		return nil
	}
	var out []int64
	for _, m := range j.Batch.Members {
		out = append(out, m.TicketID)
	}
	return out
}

const passed = `{"commands":[{"label":"build","exit_code":0,"log":"ok"}]}`
const red = `{"commands":[{"label":"test","exit_code":1,"log":"1 test failed"}]}`

func TestABatchTakesQueuedCommitsInQueueOrderUpToTheRepoSize(t *testing.T) {
	r := batchFixture(t, 5, 4)
	got := r.claim()
	if want := []int64{1, 2, 3, 4}; !slices.Equal(memberTickets(got), want) {
		t.Fatalf("batch members = %v, want %v", memberTickets(got), want)
	}
	if got.SHA != r.jobs[1].SHA || got.BundleSHA256 != r.jobs[1].BundleSHA256 {
		t.Fatalf("the lead commit is the oldest: got %s", got.SHA)
	}
	for _, m := range got.Batch.Members {
		if m.SHA != r.jobs[m.TicketID].SHA || m.ID != r.jobs[m.TicketID].ID {
			t.Fatalf("member %d carries %s/%s", m.TicketID, m.ID, m.SHA)
		}
	}
	for id := int64(1); id <= 4; id++ {
		if j := r.job(id); j.Status != "queued" || j.BatchID != got.ID {
			t.Fatalf("job of #%d = %s held by %q, want queued and held by the batch", id, j.Status, j.BatchID)
		}
	}
	if j := r.job(5); j.BatchID != "" {
		t.Fatalf("#5 should wait for the next batch, held by %q", j.BatchID)
	}
	if next := r.claim(); next != nil {
		t.Fatalf("a batch takes one host slot; got another claim %v", next.ID)
	}
	// The host may fetch every member's bundle while the batch runs.
	for _, m := range got.Batch.Members {
		rc, _, err := r.s.OpenBundle(r.f.ctx, r.host, m.ID)
		if err != nil {
			t.Fatalf("bundle of member #%d: %v", m.TicketID, err)
		}
		rc.Close()
	}
	if _, _, err := r.s.OpenBundle(r.f.ctx, r.host, r.jobs[5].ID); domain.KindOf(err) != domain.KindForbidden {
		t.Fatalf("a job outside the batch must not be served: %v", err)
	}
}

func TestBatchingIsOffUnlessTheRepoOptsIn(t *testing.T) {
	r := batchFixture(t, 3, 4)
	if _, err := r.f.store.SQL().Exec(`UPDATE repos SET batch_verify=0`); err != nil {
		t.Fatal(err)
	}
	got := r.claim()
	if got == nil || got.Batch != nil || got.ID != r.jobs[1].ID {
		t.Fatalf("want the oldest job alone, got %+v", got)
	}
}

func TestAnAgentWithoutBatchSupportGetsSingleJobs(t *testing.T) {
	r := batchFixture(t, 3, 4)
	if _, err := r.f.store.SQL().Exec(`UPDATE build_hosts SET agent_version='1.0'`); err != nil {
		t.Fatal(err)
	}
	if got := r.claim(); got == nil || got.Batch != nil {
		t.Fatalf("an old agent would verify only the lead commit; got %+v", got)
	}
}

func TestABatchPassIsEveryMembersPassForItsOwnCommit(t *testing.T) {
	r := batchFixture(t, 3, 4)
	b := r.claim()
	r.complete(b.ID, 1, "passed", passed)
	for id := int64(1); id <= 3; id++ {
		j := r.job(id)
		if j.Status != "passed" || j.BatchID != "" {
			t.Fatalf("#%d job = %s held by %q, want passed and free", id, j.Status, j.BatchID)
		}
		if !strings.Contains(j.Detail, "verified in batch with") {
			t.Fatalf("#%d detail = %q", id, j.Detail)
		}
		v, ok := r.verdict(j)
		if !ok || v.Status != domain.VerdictPassed || v.SHA != r.jobs[id].SHA || v.JobID != j.ID || v.TicketID != id {
			t.Fatalf("#%d verdict = %+v (found %v), want a pass for its own commit and job", id, v, ok)
		}
	}
	detail, err := r.f.board.Ticket(r.f.ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(detail.Timeline, func(e domain.TimelineEntry) bool {
		return strings.Contains(e.Body, "passed for commit") && strings.Contains(e.Body, "#1, #3")
	}) {
		t.Fatal("#2's timeline should say it passed in a batch with #1 and #3")
	}
}

func TestACommitThatDoesNotMergeIsVerifiedAlone(t *testing.T) {
	r := batchFixture(t, 4, 3)
	b := r.claim()
	conflicted := r.jobs[2].ID
	r.complete(b.ID, 1, "passed", fmt.Sprintf(`{"commands":[{"label":"build","exit_code":0,"log":"ok"}],"excluded":[%q]}`, conflicted))
	if j := r.job(2); j.Status != "queued" || j.BatchID != "" || !j.BatchSolo {
		t.Fatalf("#2 = %s held by %q solo %v, want queued alone", j.Status, j.BatchID, j.BatchSolo)
	}
	if _, ok := r.verdict(r.jobs[2]); ok {
		t.Fatal("a commit left out of the batch must get no verdict from it")
	}
	if j := r.job(1); j.Status != "passed" {
		t.Fatalf("#1 = %s, want passed", j.Status)
	}
	// #2 runs alone even with batching on and #4 still queued.
	got := r.claim()
	if got == nil || got.Batch != nil || got.ID != r.jobs[2].ID {
		t.Fatalf("want #2's own job alone, got %+v", got)
	}
}

func TestOnlyABatchMayReportExcludedMembers(t *testing.T) {
	r := batchFixture(t, 2, 4)
	b := r.claim()
	c := domain.BuildJobCompletion{Attempt: 1, Status: "passed", Result: []byte(fmt.Sprintf(`{"commands":[{"label":"b","exit_code":0,"log":""}],"excluded":[%q]}`, r.jobs[1].ID))}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.Complete(r.f.ctx, r.host, b.ID, c); domain.KindOf(err) != domain.KindInvalid {
		t.Fatalf("the lead cannot be left out of its own batch: %v", err)
	}
}

func TestARedBatchBisectsUntilTheCulpritRunsAlone(t *testing.T) {
	r := batchFixture(t, 4, 4)
	b := r.claim()
	r.complete(b.ID, 1, "failed", red)
	for id := int64(1); id <= 4; id++ {
		if j := r.job(id); j.Status != "queued" {
			t.Fatalf("a red batch fails nobody: #%d = %s", id, j.Status)
		}
		if _, ok := r.verdict(r.jobs[id]); ok {
			t.Fatalf("#%d got a verdict from a mixed candidate", id)
		}
	}
	// First half: #1 and #2. Red again: each now runs alone.
	half := r.claim()
	if want := []int64{1, 2}; !slices.Equal(memberTickets(half), want) {
		t.Fatalf("first half = %v, want %v", memberTickets(half), want)
	}
	r.complete(half.ID, 1, "failed", red)
	// Second half: #3 and #4. Green: both pass without running alone.
	other := r.claim()
	if want := []int64{3, 4}; !slices.Equal(memberTickets(other), want) {
		t.Fatalf("second half = %v, want %v", memberTickets(other), want)
	}
	r.complete(other.ID, 1, "passed", passed)
	for _, id := range []int64{3, 4} {
		if v, ok := r.verdict(r.jobs[id]); !ok || v.Status != domain.VerdictPassed {
			t.Fatalf("#%d should pass from its half", id)
		}
	}
	// #1 alone is the culprit; #2 alone passes.
	solo := r.claim()
	if solo == nil || solo.Batch != nil || solo.ID != r.jobs[1].ID {
		t.Fatalf("want #1 alone, got %+v", solo)
	}
	r.complete(solo.ID, 1, "failed", red)
	solo = r.claim()
	if solo == nil || solo.Batch != nil || solo.ID != r.jobs[2].ID {
		t.Fatalf("want #2 alone, got %+v", solo)
	}
	r.complete(solo.ID, 1, "passed", passed)
	if v, _ := r.verdict(r.jobs[1]); v.Status != domain.VerdictFailed {
		t.Fatalf("#1 verdict = %+v, want failed", v)
	}
	if v, _ := r.verdict(r.jobs[2]); v.Status != domain.VerdictPassed {
		t.Fatalf("#2 verdict = %+v, want passed", v)
	}
}

func TestANewCommitDuringABatchDropsTheOldOneFromIt(t *testing.T) {
	r := batchFixture(t, 3, 4)
	b := r.claim()
	stale := r.jobs[2]
	newer := r.submit(2, 2)
	r.complete(b.ID, 1, "passed", passed)
	if j, _ := r.s.Get(r.f.ctx, stale.ID); j.Status != "superseded" {
		t.Fatalf("stale job = %s, want superseded", j.Status)
	}
	if _, ok := r.verdict(stale); ok {
		t.Fatal("a superseded commit's batch result must never count")
	}
	if j := r.job(2); j.ID != newer.ID || j.Status != "queued" || j.BatchID != "" || j.BatchSolo {
		t.Fatalf("new commit = %s held by %q solo %v, want free to batch", j.Status, j.BatchID, j.BatchSolo)
	}
	for _, id := range []int64{1, 3} {
		if v, ok := r.verdict(r.jobs[id]); !ok || v.Status != domain.VerdictPassed {
			t.Fatalf("#%d should still pass", id)
		}
	}
}

func TestABatchWhoseCommitsAreAllSupersededStops(t *testing.T) {
	r := batchFixture(t, 2, 4)
	b := r.claim()
	r.submit(1, 2)
	r.submit(2, 2)
	reply, err := r.s.Progress(r.f.ctx, r.host, b.ID, domain.BuildJobProgress{Attempt: 1, Phase: "building"})
	if err != nil || !reply.Stop {
		t.Fatalf("reply = %+v, %v; want stop", reply, err)
	}
}

func TestAHostFailureRunsTheBatchAgain(t *testing.T) {
	r := batchFixture(t, 2, 4)
	b := r.claim()
	r.complete(b.ID, 1, "failed", `{"commands":[],"failure_kind":"infrastructure"}`)
	again := r.claim()
	if again == nil || again.ID != b.ID || again.Attempt != 2 {
		t.Fatalf("want the same batch, attempt 2; got %+v", again)
	}
	if !slices.Equal(memberTickets(again), []int64{1, 2}) {
		t.Fatalf("members = %v", memberTickets(again))
	}
	raw, _ := json.Marshal(again)
	if !strings.Contains(string(raw), `"batch":{"members":[`) {
		t.Fatalf("assignment JSON lacks the batch: %s", raw)
	}
}

func TestTheRunnersStripShowsABatchsTickets(t *testing.T) {
	r := batchFixture(t, 3, 4)
	r.claim()
	svc := app.NewBuildHostService(r.f.store, r.f.clock, slog.Default(), nil)
	snap, err := svc.Runners(r.f.ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Hosts) != 1 || len(snap.Hosts[0].Jobs) != 1 || !slices.Equal(snap.Hosts[0].Jobs[0].Batch, []int64{1, 2, 3}) {
		t.Fatalf("runners = %+v", snap.Hosts)
	}
}
