package app_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/auth"
	"github.com/rbretschneider/ownership_board/internal/buildbundle"
	"github.com/rbretschneider/ownership_board/internal/domain"
	"github.com/rbretschneider/ownership_board/internal/store/sqlite"
)

func jobFixture(t *testing.T) (*fixture, *app.BuildJobService, domain.BuildJobSubmission, domain.BuildHost) {
	t.Helper()
	f := newFixture(t)
	for _, q := range []string{
		`INSERT INTO users(id,name,email,created_at) VALUES(1,'Owner','owner@example.com','2026-01-01T00:00:00.000Z')`,
		`INSERT INTO repos(id,provider,full_name,created_at) VALUES(1,'manual','example/tools','2026-01-01T00:00:00.000Z')`,
		`INSERT INTO tickets(id,repo_id,kind,title,stage,lane,rank,created_by,created_at,updated_at,lane_changed_at) VALUES(1,1,'story','Example','ready','todo',1,'Owner','2026-01-01T00:00:00.000Z','2026-01-01T00:00:00.000Z','2026-01-01T00:00:00.000Z')`,
	} {
		if _, err := f.store.SQL().Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	h, _ := domain.NewBuildHost(domain.BuildHostDraft{Name: "Builder", OS: "linux", Arch: "amd64", PublicKey: base64.StdEncoding.EncodeToString(make([]byte, 32)), Capabilities: []string{"cmake"}}, f.clock.Now())
	if err := f.store.Write(f.ctx, func(r app.Repos) error {
		if e := r.BuildHosts().Insert(f.ctx, &h); e != nil {
			return e
		}
		if e := h.Approve(1, f.clock.Now(), domain.BuildHostEdit{}); e != nil {
			return e
		}
		return r.BuildHosts().Approve(f.ctx, h)
	}); err != nil {
		t.Fatal(err)
	}
	bs, err := buildbundle.New(filepath.Join(t.TempDir(), "bundles"))
	if err != nil {
		t.Fatal(err)
	}
	s := app.NewBuildJobService(f.store, f.clock, bs, slog.Default())
	m := domain.BuildJobSubmission{TicketID: 1, Repo: "example/tools", SHA: fmt.Sprintf("%040x", 1), Preset: "cmake", Target: domain.BuildJobTarget{OS: "linux", RequiredCapabilities: []string{"cmake"}}, BundleBytes: 3, BundleSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("abc")))}
	return f, s, m, h
}
func submitJob(t *testing.T, f *fixture, s *app.BuildJobService, m domain.BuildJobSubmission) domain.BuildJob {
	t.Helper()
	j, _, err := s.Submit(f.ctx, m, bytes.NewBufferString("abc"), func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	return j
}
func TestBuildJobsLifecycle(t *testing.T) {
	f, s, m, h := jobFixture(t)
	j := submitJob(t, f, s, m)
	same, created, e := s.Submit(f.ctx, m, bytes.NewBufferString("different"), func() error { return nil })
	if e != nil || created || same.ID != j.ID {
		t.Fatal(same, created, e)
	}
	got, e := s.Claim(f.ctx, app.AuthenticatedBuildHost{ID: h.ID})
	if e != nil || got == nil || got.ID != j.ID {
		t.Fatal(got, e)
	}
	if next, e := s.Claim(f.ctx, app.AuthenticatedBuildHost{ID: h.ID}); e != nil || next != nil {
		t.Fatal("exceeded capacity", next, e)
	}
	m.SHA = fmt.Sprintf("%040x", 2)
	newer := submitJob(t, f, s, m)
	p, e := s.Progress(f.ctx, app.AuthenticatedBuildHost{ID: h.ID}, j.ID, domain.BuildJobProgress{Attempt: 1, Phase: "build"})
	if e != nil || !p.Stop {
		t.Fatal(p, e)
	}
	if next, e := s.Claim(f.ctx, app.AuthenticatedBuildHost{ID: h.ID}); e != nil || next != nil {
		t.Fatal("released stopping assignment", next, e)
	}
	_, e = s.Progress(f.ctx, app.AuthenticatedBuildHost{ID: h.ID}, j.ID, domain.BuildJobProgress{Attempt: 1, Phase: "stopped"})
	if e != nil {
		t.Fatal(e)
	}
	got, e = s.Claim(f.ctx, app.AuthenticatedBuildHost{ID: h.ID})
	if e != nil || got == nil || got.ID != newer.ID {
		t.Fatal(got, e)
	}
	report := domain.BuildJobCompletion{Attempt: 1, Status: "passed", Result: []byte(`{"commands":[{"label":"build","exit_code":0,"log":"ok"}]}`)}
	for i := 0; i < 2; i++ {
		end, e := s.Complete(f.ctx, app.AuthenticatedBuildHost{ID: h.ID}, newer.ID, report)
		if e != nil || end.Status != "passed" {
			t.Fatal(end, e)
		}
	}
	report.Status = "failed"
	report.Result = []byte(`{"commands":[],"failure_kind":"infrastructure"}`)
	if _, e = s.Complete(f.ctx, app.AuthenticatedBuildHost{ID: h.ID}, newer.ID, report); domain.KindOf(e) != domain.KindConflict {
		t.Fatal(e)
	}
}
func TestBuildJobsOfflineAttempts(t *testing.T) {
	f, s, m, h := jobFixture(t)
	j := submitJob(t, f, s, m)
	for attempt := 1; attempt <= 3; attempt++ {
		got, e := s.Claim(f.ctx, app.AuthenticatedBuildHost{ID: h.ID})
		if e != nil || got == nil || got.Attempt != attempt {
			t.Fatal(got, e)
		}
		f.clock.Advance(3*time.Minute - time.Millisecond)
		if e = s.RequeueOffline(f.ctx); e != nil {
			t.Fatal(e)
		}
		before, _ := s.Get(f.ctx, j.ID)
		if before.Status != "running" {
			t.Fatal(before)
		}
		f.clock.Advance(time.Millisecond)
		if e = s.RequeueOffline(f.ctx); e != nil {
			t.Fatal(e)
		}
		p, e := s.Progress(f.ctx, app.AuthenticatedBuildHost{ID: h.ID}, j.ID, domain.BuildJobProgress{Attempt: attempt, Phase: "late"})
		if e != nil || !p.Stop {
			t.Fatal(p, e)
		}
	}
	end, e := s.Get(f.ctx, j.ID)
	if e != nil || end.Status != "failed" || !bytes.Contains(end.Result, []byte("infrastructure")) {
		t.Fatal(end, e)
	}
}
func TestBuildJobsConcurrentClaims(t *testing.T) {
	f, s, m, h := jobFixture(t)
	var path string
	if e := f.store.SQL().QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&path); e != nil {
		t.Fatal(e)
	}
	db2, e := sqlite.Open(f.ctx, path)
	if e != nil {
		t.Fatal(e)
	}
	defer db2.Close()
	s2 := app.NewBuildJobService(db2, f.clock, nil, slog.Default())
	h2 := h
	h2.PublicKey = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	h2.Status = domain.BuildHostPending
	h2.ApprovedAt = nil
	h2.ApprovedBy = nil
	if e = f.store.Write(f.ctx, func(r app.Repos) error {
		if e := r.BuildHosts().Insert(f.ctx, &h2); e != nil {
			return e
		}
		h2.Approve(1, f.clock.Now(), domain.BuildHostEdit{})
		return r.BuildHosts().Approve(f.ctx, h2)
	}); e != nil {
		t.Fatal(e)
	}
	for n := 0; n < 10; n++ {
		m.SHA = fmt.Sprintf("%040x", n+1)
		j := submitJob(t, f, s, m)
		start := make(chan struct{})
		var wg sync.WaitGroup
		out := make(chan *domain.BuildJob, 2)
		errs := make(chan error, 2)
		for i, svc := range []*app.BuildJobService{s, s2} {
			host := []domain.BuildHost{h, h2}[i]
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				j, e := svc.Claim(f.ctx, app.AuthenticatedBuildHost{ID: host.ID})
				out <- j
				errs <- e
			}()
		}
		close(start)
		wg.Wait()
		close(out)
		close(errs)
		wins := 0
		var winner *domain.BuildJob
		for e := range errs {
			if e != nil {
				t.Fatal(e)
			}
		}
		for got := range out {
			if got != nil {
				wins++
				winner = got
			}
		}
		if wins != 1 {
			t.Fatalf("claims=%d", wins)
		}
		_, e := s.Complete(f.ctx, app.AuthenticatedBuildHost{ID: *winner.HostID}, j.ID, domain.BuildJobCompletion{Attempt: 1, Status: "passed", Result: []byte(`{"commands":[{"label":"build","exit_code":0,"log":""}]}`)})
		if e != nil {
			t.Fatal(e)
		}
	}
}

func TestBuildJobsEligibility(t *testing.T) {
	for _, tc := range []struct {
		name         string
		os           domain.BuildHostOS
		caps         []string
		status       domain.BuildHostStatus
		enabled      bool
		want         bool
		unauthorized bool
	}{
		{"matching", "linux", []string{"cmake"}, domain.BuildHostApproved, true, true, false},
		{"no capability", "linux", nil, domain.BuildHostApproved, true, false, false},
		{"wrong os", "windows", []string{"cmake"}, domain.BuildHostApproved, true, false, false},
		{"disabled", "linux", []string{"cmake"}, domain.BuildHostApproved, false, false, false},
		{"pending", "linux", []string{"cmake"}, domain.BuildHostPending, true, false, true},
		{"revoked", "linux", []string{"cmake"}, domain.BuildHostRevoked, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, s, m, h := jobFixture(t)
			submitJob(t, f, s, m)
			caps, _ := json.Marshal(tc.caps)
			if tc.caps == nil {
				caps = []byte(`[]`)
			}
			var approval, by, revoked any
			if tc.status == domain.BuildHostApproved {
				approval = f.clock.Now().Format("2006-01-02T15:04:05.000Z")
				by = 1
			}
			if tc.status == domain.BuildHostRevoked {
				revoked = f.clock.Now().Format("2006-01-02T15:04:05.000Z")
			}
			if _, e := f.store.SQL().Exec(`UPDATE build_hosts SET os=?,capabilities=?,status=?,enabled=?,approved_at=?,approved_by=?,revoked_at=? WHERE id=?`, tc.os, string(caps), tc.status, tc.enabled, approval, by, revoked, h.ID); e != nil {
				t.Fatal(e)
			}
			got, e := s.Claim(f.ctx, app.AuthenticatedBuildHost{ID: h.ID})
			if tc.unauthorized {
				if !errors.Is(e, auth.ErrUnauthorized) {
					t.Fatal(e)
				}
				return
			}
			if e != nil || (got != nil) != tc.want {
				t.Fatal(got, e)
			}
		})
	}
}
func TestBuildJobsCancellationAndFencing(t *testing.T) {
	f, s, m, h := jobFixture(t)
	j := submitJob(t, f, s, m)
	if _, e := s.Cancel(f.ctx, false, j.ID); domain.KindOf(e) != domain.KindForbidden {
		t.Fatal(e)
	}
	if _, e := s.Claim(f.ctx, app.AuthenticatedBuildHost{ID: h.ID}); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		got, e := s.Cancel(f.ctx, true, j.ID)
		if e != nil || got.Status != "cancelled" {
			t.Fatal(got, e)
		}
	}
	got, e := s.Progress(f.ctx, app.AuthenticatedBuildHost{ID: h.ID}, j.ID, domain.BuildJobProgress{Attempt: 1, Phase: "build"})
	if e != nil || !got.Stop {
		t.Fatal(got, e)
	}
	final, e := s.Complete(f.ctx, app.AuthenticatedBuildHost{ID: h.ID}, j.ID, domain.BuildJobCompletion{Attempt: 1, Status: "passed", Result: []byte(`{"commands":[{"label":"build","exit_code":0,"log":""}]}`)})
	if e != nil || final.Status != "cancelled" || final.Result != nil || final.AssignmentReleasedAt == nil {
		t.Fatal(final, e)
	}
}
func TestBuildJobsConcurrentHostCapacity(t *testing.T) {
	f, s, m, h := jobFixture(t)
	// Distinct targets avoid supersession while all remain compatible with one host.
	for _, preset := range []string{"cmake", "script", "dotnet"} {
		m.Preset = preset
		submitJob(t, f, s, m)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	out := make(chan *domain.BuildJob, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			j, e := s.Claim(f.ctx, app.AuthenticatedBuildHost{ID: h.ID})
			out <- j
			errs <- e
		}()
	}
	close(start)
	wg.Wait()
	close(out)
	close(errs)
	wins := 0
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	for j := range out {
		if j != nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatal("capacity exceeded", wins)
	}
}
func TestBuildJobsStorageQuotaAndRecovery(t *testing.T) {
	f, s, m, _ := jobFixture(t)
	s.StorageLimit = 3
	j := submitJob(t, f, s, m)
	m.SHA = fmt.Sprintf("%040x", 2)
	if _, _, e := s.Submit(f.ctx, m, bytes.NewBufferString("abc"), nil); domain.KindOf(e) != domain.KindTooLarge {
		t.Fatal(e)
	}
	// A rejected reservation never disturbs an existing payload.
	if e := s.RecoverStorage(f.ctx); e != nil {
		t.Fatal(e)
	}
	got, e := s.Get(f.ctx, j.ID)
	if e != nil || got.Status != "queued" {
		t.Fatal(got, e)
	}
}
func TestBuildJobsPublicationRollbackRetainsOrphan(t *testing.T) {
	f, s, m, _ := jobFixture(t)
	if _, e := f.store.SQL().Exec(`CREATE TRIGGER refuse_job BEFORE INSERT ON build_jobs BEGIN SELECT RAISE(ABORT,'refused'); END`); e != nil {
		t.Fatal(e)
	}
	if _, _, e := s.Submit(f.ctx, m, bytes.NewBufferString("abc"), nil); e == nil {
		t.Fatal("accepted refused publication")
	}
	if _, e := s.Get(f.ctx, m.ID()); domain.KindOf(e) != domain.KindNotFound {
		t.Fatal(e)
	}
	if e := s.RecoverStorage(f.ctx); e != nil {
		t.Fatal(e)
	}
	if _, e := f.store.SQL().Exec(`DROP TRIGGER refuse_job`); e != nil {
		t.Fatal(e)
	}
	conflict := m
	conflict.BundleSHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte("abd")))
	if _, _, e := s.Submit(f.ctx, conflict, bytes.NewBufferString("abd"), nil); domain.KindOf(e) != domain.KindConflict {
		t.Fatal("orphan was overwritten", e)
	}
	// The orphan occupies the entire allowance and must be adopted without charging twice.
	s.StorageLimit = 3
	submitJob(t, f, s, m)
	if e := s.RecoverStorage(f.ctx); e != nil {
		t.Fatal(e)
	}
}
func TestBuildJobsCrossTicketAndRepoValidation(t *testing.T) {
	f, s, m, _ := jobFixture(t)
	submitJob(t, f, s, m)
	otherID := f.ticket("Other", 1)
	m.TicketID = otherID
	if _, _, e := s.Submit(f.ctx, m, bytes.NewBufferString("abc"), nil); domain.KindOf(e) != domain.KindConflict {
		t.Fatal(e)
	}
	m.TicketID = 1
	m.Repo = "elsewhere/tools"
	if _, _, e := s.Submit(f.ctx, m, bytes.NewBufferString("abc"), nil); domain.KindOf(e) != domain.KindInvalid {
		t.Fatal(e)
	}
}

func TestBuildJobsOldCommitNewPresetCannotSupersedeNewer(t *testing.T) {
	f, s, m, _ := jobFixture(t)
	old := submitJob(t, f, s, m)
	m.SHA = fmt.Sprintf("%040x", 2)
	newer := submitJob(t, f, s, m)
	m.SHA = old.SHA
	m.Preset = "script"
	late := submitJob(t, f, s, m)
	stored, err := s.Get(f.ctx, late.ID)
	if err != nil || stored.FinishedAt == nil || late.FinishedAt == nil || !stored.FinishedAt.Equal(*late.FinishedAt) {
		t.Fatalf("terminal submission did not round trip: submitted=%+v stored=%+v error=%v", late, stored, err)
	}
	current, e := s.Get(f.ctx, newer.ID)
	if e != nil || current.Status != "queued" || late.Status != "superseded" {
		t.Fatal(current, late, e)
	}
}
func TestBuildJobsOfflineReportCannotOverwriteRetry(t *testing.T) {
	f, s, m, h := jobFixture(t)
	j := submitJob(t, f, s, m)
	who := app.AuthenticatedBuildHost{ID: h.ID}
	s.Claim(f.ctx, who)
	f.clock.Advance(3 * time.Minute)
	s.RequeueOffline(f.ctx)
	s.Claim(f.ctx, who)
	p, e := s.Progress(f.ctx, who, j.ID, domain.BuildJobProgress{Attempt: 1, Phase: "old"})
	if e != nil || !p.Stop || p.Attempt != 2 {
		t.Fatal(p, e)
	}
	_, e = s.Complete(f.ctx, who, j.ID, domain.BuildJobCompletion{Attempt: 1, Status: "passed", Result: []byte(`{"commands":[{"label":"build","exit_code":0,"log":""}]}`)})
	if domain.KindOf(e) != domain.KindConflict {
		t.Fatal(e)
	}
	got, _ := s.Get(f.ctx, j.ID)
	if got.Attempt != 2 || got.Phase != "claimed" || got.Result != nil {
		t.Fatal(got)
	}
}

type gatedJobReader struct {
	start   chan struct{}
	release chan struct{}
	once    sync.Once
	data    *bytes.Reader
}

func (r *gatedJobReader) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.start); <-r.release })
	return r.data.Read(p)
}
func TestBuildJobsConcurrentReservations(t *testing.T) {
	f, s, m, _ := jobFixture(t)
	s.StorageLimit = 3
	reader := &gatedJobReader{start: make(chan struct{}), release: make(chan struct{}), data: bytes.NewReader([]byte("abc"))}
	done := make(chan error, 1)
	go func() { _, _, e := s.Submit(f.ctx, m, reader, nil); done <- e }()
	<-reader.start
	_, _, sameErr := s.Submit(f.ctx, m, bytes.NewBufferString("abc"), nil)
	other := m
	other.SHA = fmt.Sprintf("%040x", 2)
	_, _, otherErr := s.Submit(f.ctx, other, bytes.NewBufferString("abc"), nil)
	close(reader.release)
	firstErr := <-done
	if firstErr != nil || domain.KindOf(sameErr) != domain.KindConflict || domain.KindOf(otherErr) != domain.KindTooLarge {
		t.Fatal(firstErr, sameErr, otherErr)
	}
}
func TestBuildJobsAbandonedPayloadsConsumeQuota(t *testing.T) {
	f, s, m, _ := jobFixture(t)
	s.StorageLimit = 5
	if _, _, e := s.Submit(f.ctx, m, bytes.NewBufferString("abd"), nil); domain.KindOf(e) != domain.KindInvalid {
		t.Fatal(e)
	}
	if e := s.RecoverStorage(f.ctx); e != nil {
		t.Fatal(e)
	}
	if _, _, e := s.Submit(f.ctx, m, bytes.NewBufferString("abc"), nil); domain.KindOf(e) != domain.KindTooLarge {
		t.Fatal(e)
	}
	s.StorageLimit = 6
	submitJob(t, f, s, m)
}
func TestBuildJobsActiveHeartbeatAndRevocation(t *testing.T) {
	f, s, m, h := jobFixture(t)
	j := submitJob(t, f, s, m)
	s.Claim(f.ctx, app.AuthenticatedBuildHost{ID: h.ID})
	f.clock.Advance(2 * time.Minute)
	at := f.clock.Now().Format("2006-01-02T15:04:05.000Z")
	if _, e := f.store.SQL().Exec(`UPDATE build_hosts SET last_seen_at=? WHERE id=?`, at, h.ID); e != nil {
		t.Fatal(e)
	}
	f.clock.Advance(time.Minute)
	if e := s.RequeueOffline(f.ctx); e != nil {
		t.Fatal(e)
	}
	got, _ := s.Get(f.ctx, j.ID)
	if got.Status != "running" {
		t.Fatal(got)
	}
	if e := f.store.Write(f.ctx, func(r app.Repos) error { return r.BuildHosts().Revoke(f.ctx, h.ID, f.clock.Now()) }); e != nil {
		t.Fatal(e)
	}
	if e := s.RequeueOffline(f.ctx); e != nil {
		t.Fatal(e)
	}
	got, _ = s.Get(f.ctx, j.ID)
	if got.Status != "queued" || got.Attempt != 2 || got.HostID != nil {
		t.Fatal(got)
	}
}

func TestBuildJobsCorruptReferencedPayloadPreventsRecovery(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprint(missing), func(t *testing.T) {
			f, _, m, _ := jobFixture(t)
			root := filepath.Join(t.TempDir(), "bundles")
			bs, e := buildbundle.New(root)
			if e != nil {
				t.Fatal(e)
			}
			s := app.NewBuildJobService(f.store, f.clock, bs, slog.Default())
			j := submitJob(t, f, s, m)
			path := filepath.Join(root, j.ID, "commit.bundle")
			if missing {
				e = os.Remove(path)
			} else {
				e = os.WriteFile(path, []byte("abd"), 0600)
			}
			if e != nil {
				t.Fatal(e)
			}
			if e = s.RecoverStorage(f.ctx); e == nil {
				t.Fatal("accepted unusable payload")
			}
		})
	}
}
func TestBuildJobsForeignHostCannotReport(t *testing.T) {
	f, s, m, h := jobFixture(t)
	j := submitJob(t, f, s, m)
	s.Claim(f.ctx, app.AuthenticatedBuildHost{ID: h.ID})
	other := h
	other.PublicKey = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32))
	other.Status = domain.BuildHostPending
	other.ApprovedAt = nil
	other.ApprovedBy = nil
	if e := f.store.Write(f.ctx, func(r app.Repos) error {
		if e := r.BuildHosts().Insert(f.ctx, &other); e != nil {
			return e
		}
		if e := other.Approve(1, f.clock.Now(), domain.BuildHostEdit{}); e != nil {
			return e
		}
		return r.BuildHosts().Approve(f.ctx, other)
	}); e != nil {
		t.Fatal(e)
	}
	_, e := s.Progress(f.ctx, app.AuthenticatedBuildHost{ID: other.ID}, j.ID, domain.BuildJobProgress{Attempt: 1, Phase: "foreign"})
	if domain.KindOf(e) != domain.KindForbidden {
		t.Fatal(e)
	}
	_, e = s.Complete(f.ctx, app.AuthenticatedBuildHost{ID: other.ID}, j.ID, domain.BuildJobCompletion{Attempt: 1, Status: "passed", Result: []byte(`{"commands":[{"label":"build","exit_code":0,"log":""}]}`)})
	if domain.KindOf(e) != domain.KindForbidden {
		t.Fatal(e)
	}
	got, _ := s.Get(f.ctx, j.ID)
	if got.Phase != "claimed" || got.Status != "running" {
		t.Fatal(got)
	}
}

func TestBuildJobsMultiSlotCapacity(t *testing.T) {
	f, s, m, h := jobFixture(t)
	if _, e := f.store.SQL().Exec(`UPDATE build_hosts SET max_parallel=2 WHERE id=?`, h.ID); e != nil {
		t.Fatal(e)
	}
	for _, preset := range []string{"cmake", "script", "dotnet"} {
		m.Preset = preset
		submitJob(t, f, s, m)
	}
	for i := 0; i < 3; i++ {
		j, e := s.Claim(f.ctx, app.AuthenticatedBuildHost{ID: h.ID})
		if e != nil || (j != nil) != (i < 2) {
			t.Fatal(i, j, e)
		}
	}
}
func TestBuildJobsSameCommitPresetsAndTargetIsolation(t *testing.T) {
	f, s, m, _ := jobFixture(t)
	first := submitJob(t, f, s, m)
	m.Preset = "script"
	second := submitJob(t, f, s, m)
	isolated := m
	isolated.Target.RequiredCapabilities = []string{}
	isolated.SHA = fmt.Sprintf("%040x", 3)
	third := submitJob(t, f, s, isolated)
	for _, id := range []string{first.ID, second.ID, third.ID} {
		j, e := s.Get(f.ctx, id)
		if e != nil || j.Status != "queued" {
			t.Fatal(j, e)
		}
	}
	m.SHA = fmt.Sprintf("%040x", 2)
	submitJob(t, f, s, m)
	for _, id := range []string{first.ID, second.ID} {
		j, _ := s.Get(f.ctx, id)
		if j.Status != "superseded" {
			t.Fatal(j)
		}
	}
	j, _ := s.Get(f.ctx, third.ID)
	if j.Status != "queued" {
		t.Fatal(j)
	}
	// A duplicate of an older check preserves the existing lifecycle.
	duplicate, created, e := s.Submit(f.ctx, domain.BuildJobSubmission{TicketID: first.TicketID, Repo: first.Repo, SHA: first.SHA, Preset: first.Preset, Target: first.Target, BundleBytes: first.BundleBytes, BundleSHA256: first.BundleSHA256}, bytes.NewBufferString("abc"), nil)
	if e != nil || created || duplicate.Status != "superseded" {
		t.Fatal(duplicate, created, e)
	}
}

func TestBuildJobsInterruptedReservationRecovery(t *testing.T) {
	f, _, m, _ := jobFixture(t)
	root := filepath.Join(t.TempDir(), "payloads")
	bs, e := buildbundle.New(root)
	if e != nil {
		t.Fatal(e)
	}
	s := app.NewBuildJobService(f.store, f.clock, bs, slog.Default())
	token := strings.Repeat("b", 64)
	if e = f.store.Write(f.ctx, func(r app.Repos) error {
		return r.BuildJobs().Reserve(f.ctx, app.BuildJobUpload{Token: token, JobID: m.ID(), Metadata: m, CreatedAt: f.clock.Now()})
	}); e != nil {
		t.Fatal(e)
	}
	if e = bs.Stage(token, m, bytes.NewBufferString("abd")); domain.KindOf(e) != domain.KindInvalid {
		t.Fatal(e)
	}
	if e = s.RecoverStorage(f.ctx); e != nil {
		t.Fatal(e)
	}
	if e = f.store.Read(f.ctx, func(r app.Repos) error {
		uploads, e := r.BuildJobs().Uploads(f.ctx)
		if e == nil && len(uploads) != 0 {
			t.Fatal("active interrupted reservation")
		}
		return e
	}); e != nil {
		t.Fatal(e)
	}
	s.StorageLimit = 3
	if _, _, e = s.Submit(f.ctx, m, bytes.NewBufferString("abc"), nil); domain.KindOf(e) != domain.KindTooLarge {
		t.Fatal("lost retained partial accounting", e)
	}
}
func TestBuildJobsStoppingTimeoutReleasesCapacity(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelled), func(t *testing.T) {
			f, s, m, h := jobFixture(t)
			j := submitJob(t, f, s, m)
			who := app.AuthenticatedBuildHost{ID: h.ID}
			s.Claim(f.ctx, who)
			want := "superseded"
			if cancelled {
				want = "cancelled"
				if _, e := s.Cancel(f.ctx, true, j.ID); e != nil {
					t.Fatal(e)
				}
			} else {
				m.SHA = fmt.Sprintf("%040x", 2)
				submitJob(t, f, s, m)
			}
			f.clock.Advance(3 * time.Minute)
			if e := s.RequeueOffline(f.ctx); e != nil {
				t.Fatal(e)
			}
			got, _ := s.Get(f.ctx, j.ID)
			if got.Status != want || got.AssignmentReleasedAt == nil || got.Attempt != 1 {
				t.Fatal(got)
			}
		})
	}
}
func TestBuildJobsSupersessionAndResultRace(t *testing.T) {
	f, s, m, h := jobFixture(t)
	for n := 0; n < 8; n++ {
		m.SHA = fmt.Sprintf("%040x", 2*n+1)
		old := submitJob(t, f, s, m)
		who := app.AuthenticatedBuildHost{ID: h.ID}
		if j, e := s.Claim(f.ctx, who); e != nil || j == nil || j.ID != old.ID {
			t.Fatal(j, e)
		}
		m.SHA = fmt.Sprintf("%040x", 2*n+2)
		start := make(chan struct{})
		errs := make(chan error, 2)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, _, e := s.Submit(f.ctx, m, bytes.NewBufferString("abc"), nil)
			errs <- e
		}()
		go func() {
			defer wg.Done()
			<-start
			_, e := s.Complete(f.ctx, who, old.ID, domain.BuildJobCompletion{Attempt: 1, Status: "passed", Result: []byte(`{"commands":[{"label":"build","exit_code":0,"log":""}]}`)})
			errs <- e
		}()
		close(start)
		wg.Wait()
		close(errs)
		for e := range errs {
			if e != nil {
				t.Fatal(e)
			}
		}
		got, _ := s.Get(f.ctx, old.ID)
		if got.Status != "passed" && got.Status != "superseded" || got.AssignmentReleasedAt == nil {
			t.Fatal(got)
		}
	}
}

func TestBuildJobsRecoveryPreservesLiveReservation(t *testing.T) {
	f, _, m, _ := jobFixture(t)
	root := filepath.Join(t.TempDir(), "bundles")
	bs, err := buildbundle.New(root)
	if err != nil {
		t.Fatal(err)
	}
	s := app.NewBuildJobService(f.store, f.clock, bs, slog.Default())
	s.StorageLimit = 3
	reader := &gatedJobReader{start: make(chan struct{}), release: make(chan struct{}), data: bytes.NewReader([]byte("abc"))}
	done := make(chan error, 1)
	go func() { _, _, err := s.Submit(f.ctx, m, reader, nil); done <- err }()
	<-reader.start
	var path string
	if err := f.store.SQL().QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&path); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(f.ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	bs2, err := buildbundle.New(root)
	if err != nil {
		t.Fatal(err)
	}
	second := app.NewBuildJobService(db, f.clock, bs2, slog.Default())
	second.StorageLimit = 3
	recoveryErr := second.RecoverStorage(f.ctx)
	_, _, sameErr := second.Submit(f.ctx, m, bytes.NewBufferString("abc"), nil)
	other := m
	other.SHA = fmt.Sprintf("%040x", 2)
	_, _, quotaErr := second.Submit(f.ctx, other, bytes.NewBufferString("abc"), nil)
	close(reader.release)
	firstErr := <-done
	if recoveryErr != nil || firstErr != nil || domain.KindOf(sameErr) != domain.KindConflict || domain.KindOf(quotaErr) != domain.KindTooLarge {
		t.Fatal(recoveryErr, firstErr, sameErr, quotaErr)
	}
}

func TestBuildJobsQueueOperationsExcludeHistoricalLogs(t *testing.T) {
	f, s, m, h := jobFixture(t)
	j := submitJob(t, f, s, m)
	command := `{"label":"build","exit_code":0,"log":"` + strings.Repeat("x", 16*1024) + `"}`
	result := `{"commands":[` + strings.TrimSuffix(strings.Repeat(command+",", 8), ",") + `]}`
	if err := (&domain.BuildJobCompletion{Attempt: 1, Status: "passed", Result: []byte(result)}).Validate(); err != nil {
		t.Fatal(err)
	}
	tx, err := f.store.SQL().BeginTx(f.ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := 0; i < 300; i++ {
		_, err := tx.Exec(`INSERT INTO build_jobs(id,ticket_id,repo,sha,preset,target,target_os,status,attempt,created_at,finished_at,result,bundle_bytes,bundle_sha256) SELECT ?,ticket_id,repo,?,preset,target,target_os,'passed',attempt,created_at,created_at,?,bundle_bytes,bundle_sha256 FROM build_jobs WHERE id=?`, fmt.Sprintf("%064x", i+1), fmt.Sprintf("%040x", i+10), result, j.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []struct {
		name string
		run  func() error
	}{
		{"recovery", func() error { return s.RequeueOffline(f.ctx) }},
		{"claim", func() error { _, err := s.Claim(f.ctx, app.AuthenticatedBuildHost{ID: h.ID}); return err }},
		{"submission", func() error {
			m.SHA = fmt.Sprintf("%040x", 999)
			_, _, err := s.Submit(f.ctx, m, bytes.NewBufferString("abc"), nil)
			return err
		}},
	} {
		t.Run(operation.name, func(t *testing.T) {
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			if err := operation.run(); err != nil {
				t.Fatal(err)
			}
			runtime.ReadMemStats(&after)
			allocated := after.TotalAlloc - before.TotalAlloc
			t.Logf("allocated %d bytes with 300 completed jobs", allocated)
			if allocated > 8*1024*1024 {
				t.Fatalf("queue operation allocated %d bytes for completed history", allocated)
			}
		})
	}
}

// 2026-11-12: workers held their leases for an hour each while the build
// server ran their Windows jobs. A build whose job is queued or running is
// not claimable, so a worker can release it and take other work; it comes
// back once the job reports.
func TestABuildWaitingOnItsBuildHostJobIsNotClaimable(t *testing.T) {
	f, s, m, h := jobFixture(t)
	if _, err := f.store.SQL().Exec(`UPDATE tickets SET stage='build', lane='in_progress' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	j := submitJob(t, f, s, m)
	f.requireNoClaim(agent, "a build whose Windows job is queued")
	if got, e := s.Claim(f.ctx, app.AuthenticatedBuildHost{ID: h.ID}); e != nil || got == nil || got.ID != j.ID {
		t.Fatal(got, e)
	}
	f.requireNoClaim(agent, "a build whose Windows job is running")
	report := domain.BuildJobCompletion{Attempt: 1, Status: "passed", Result: []byte(`{"commands":[{"label":"build","exit_code":0,"log":"ok"}]}`)}
	if _, e := s.Complete(f.ctx, app.AuthenticatedBuildHost{ID: h.ID}, j.ID, report); e != nil {
		t.Fatal(e)
	}
	if work := f.mustClaim(agent, app.ClaimInput{}); work.Ticket.Ticket.ID != 1 {
		t.Fatalf("claimed #%d after the job reported, want #1", work.Ticket.Ticket.ID)
	}
}

// 2026-11-12: #161 merged while its Windows job still waited for the one
// build host. A completed ticket's queued job is superseded instead of run.
func TestACompletedTicketsQueuedJobIsNotRun(t *testing.T) {
	f, s, m, h := jobFixture(t)
	submitJob(t, f, s, m)
	if _, err := f.store.SQL().Exec(`UPDATE tickets SET stage='done', lane='merged' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if got, e := s.Claim(f.ctx, app.AuthenticatedBuildHost{ID: h.ID}); e != nil || got != nil {
		t.Fatal("ran a job for a completed ticket", got, e)
	}
	var status string
	if err := f.store.SQL().QueryRow(`SELECT status FROM build_jobs WHERE ticket_id=1`).Scan(&status); err != nil || status != "superseded" {
		t.Fatal(status, err)
	}
}
