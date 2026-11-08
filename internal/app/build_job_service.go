package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/rbretschneider/ownership_board/internal/auth"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

type BuildJobService struct {
	store        Store
	clock        Clock
	bundles      BundleStorage
	logger       *slog.Logger
	StorageLimit int64
}

func NewBuildJobService(store Store, clock Clock, bundles BundleStorage, logger *slog.Logger) *BuildJobService {
	return &BuildJobService{store: store, clock: clock, bundles: bundles, logger: logger, StorageLimit: domain.BuildJobStorageLimit}
}
func (s *BuildJobService) now() time.Time { return s.clock.Now().UTC().Truncate(time.Millisecond) }
func validateJobRepo(ctx context.Context, r Repos, m domain.BuildJobSubmission) error {
	ticket, e := r.Tickets().Get(ctx, m.TicketID)
	if e != nil {
		return e
	}
	if ticket.RepoID <= 0 {
		return domain.Invalidf("ticket requires an assigned repository")
	}
	repo, e := r.CodeRepos().Get(ctx, ticket.RepoID)
	if e != nil {
		return e
	}
	if repo.FullName != m.Repo {
		return domain.Invalidf("repository differs from ticket assignment")
	}
	return nil
}
func existingJob(ctx context.Context, r Repos, m domain.BuildJobSubmission) (domain.BuildJob, bool, error) {
	j, e := r.BuildJobs().Get(ctx, m.ID())
	if domain.KindOf(e) == domain.KindNotFound {
		return j, false, nil
	}
	if e != nil {
		return j, false, e
	}
	if j.TicketID != m.TicketID {
		return j, false, domain.Conflictf("job_ticket_conflict")
	}
	return j, true, nil
}

// Submit reserves capacity before streaming, then publishes in a separate transaction.
// finish validates transport completion before any job becomes visible.
func (s *BuildJobService) Submit(ctx context.Context, m domain.BuildJobSubmission, src io.Reader, finish func() error) (domain.BuildJob, bool, error) {
	var j domain.BuildJob
	if e := m.Validate(); e != nil {
		return j, false, e
	}
	tokenBytes := make([]byte, 32)
	if _, e := rand.Read(tokenBytes); e != nil {
		return j, false, e
	}
	token := hex.EncodeToString(tokenBytes)
	duplicate, orphan := false, false
	e := s.store.Write(ctx, func(r Repos) error {
		if e := validateJobRepo(ctx, r, m); e != nil {
			return e
		}
		var e error
		j, duplicate, e = existingJob(ctx, r, m)
		if e != nil || duplicate {
			return e
		}
		sizes, e := s.bundles.PayloadSizes()
		if e != nil {
			return e
		}
		uploads, e := r.BuildJobs().Uploads(ctx)
		if e != nil {
			return e
		}
		total := int64(0)
		for _, n := range sizes {
			total += n
		}
		for _, u := range uploads {
			if u.JobID == m.ID() {
				return domain.Conflictf("upload_in_progress")
			}
			remaining := u.Metadata.BundleBytes - sizes[u.Token+".partial"]
			if _, exists := sizes[u.JobID+"/commit.bundle"]; exists {
				remaining = 0
			}
			if remaining > 0 {
				total += remaining
			}
		}
		_, orphan = sizes[m.ID()+"/commit.bundle"]
		needed := m.BundleBytes
		if orphan {
			if e = s.bundles.Verify(m.ID(), m.BundleBytes, m.BundleSHA256); e != nil {
				return domain.Conflictf("retained bundle conflicts with upload")
			}
			needed = 0
		}
		if total > s.StorageLimit || needed > s.StorageLimit-total {
			return domain.TooLargef("bundle storage exceeds 2 GiB capacity")
		}
		return r.BuildJobs().Reserve(ctx, BuildJobUpload{Token: token, JobID: m.ID(), Metadata: m, CreatedAt: s.now()})
	})
	if e != nil || duplicate {
		return j, false, e
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := s.store.Write(cleanupCtx, func(r Repos) error { return r.BuildJobs().FinishUpload(cleanupCtx, token) }); err != nil {
			s.logger.Error("abandon upload reservation", "error", err)
		}
	}()
	if orphan {
		hash := sha256.New()
		n, err := io.Copy(hash, io.LimitReader(src, m.BundleBytes+1))
		e = err
		if e == nil && (n != m.BundleBytes || fmt.Sprintf("%x", hash.Sum(nil)) != m.BundleSHA256) {
			e = domain.Invalidf("bundle size or digest mismatch")
		}
	} else {
		e = s.bundles.Stage(token, m, src)
	}
	if e != nil {
		return domain.BuildJob{}, false, e
	}
	if finish != nil {
		if e = finish(); e != nil {
			return domain.BuildJob{}, false, e
		}
	}
	e = s.store.Write(ctx, func(r Repos) error {
		if e := validateJobRepo(ctx, r, m); e != nil {
			return e
		}
		var e error
		j, duplicate, e = existingJob(ctx, r, m)
		if e != nil || duplicate {
			return e
		}
		if e = s.bundles.Promote(token, m); e != nil {
			return e
		}
		jobs, e := r.BuildJobs().List(ctx)
		if e != nil {
			return e
		}
		seen := map[string]bool{}
		latestSHA := ""
		for _, old := range jobs {
			if old.TicketID == m.TicketID && old.Target.Canonical() == m.Target.Canonical() && !seen[old.SHA] {
				seen[old.SHA] = true
				latestSHA = old.SHA
			}
		}
		now := s.now()
		j = domain.BuildJob{ID: m.ID(), TicketID: m.TicketID, Repo: m.Repo, SHA: m.SHA, Preset: m.Preset, Target: m.Target, Status: "queued", Attempt: 1, CreatedAt: now, BundleBytes: m.BundleBytes, BundleSHA256: m.BundleSHA256}
		if seen[m.SHA] && latestSHA != m.SHA {
			j.Status = "superseded"
			j.FinishedAt = &now
		}
		if e = r.BuildJobs().Insert(ctx, j); e != nil {
			return e
		}
		if !seen[m.SHA] {
			for _, old := range jobs {
				if old.TicketID == j.TicketID && old.Target.Canonical() == j.Target.Canonical() && old.SHA != j.SHA && (old.Status == "queued" || old.Status == "running") {
					old.Status = "superseded"
					old.FinishedAt = &now
					if e = r.BuildJobs().Save(ctx, old); e != nil {
						return e
					}
				}
			}
		}
		return r.BuildJobs().FinishUpload(ctx, token)
	})
	if e != nil {
		return domain.BuildJob{}, false, e
	}
	return j, !duplicate, nil
}
func (s *BuildJobService) Get(ctx context.Context, id string) (domain.BuildJob, error) {
	var j domain.BuildJob
	if !domain.ValidBuildJobID(id) {
		return j, domain.Invalidf("invalid job id")
	}
	e := s.store.Read(ctx, func(r Repos) error { var e error; j, e = r.BuildJobs().Get(ctx, id); return e })
	return j, e
}
func approvedJobHost(ctx context.Context, r Repos, who AuthenticatedBuildHost) (domain.BuildHost, error) {
	h, e := r.BuildHosts().Get(ctx, who.ID)
	if domain.KindOf(e) == domain.KindNotFound {
		return h, auth.ErrUnauthorized
	}
	if e != nil {
		return h, e
	}
	if h.Status != domain.BuildHostApproved {
		return h, auth.ErrUnauthorized
	}
	return h, nil
}
func (s *BuildJobService) Claim(ctx context.Context, who AuthenticatedBuildHost) (*domain.BuildJob, error) {
	var out *domain.BuildJob
	e := s.store.Write(ctx, func(r Repos) error {
		h, e := approvedJobHost(ctx, r, who)
		if e != nil {
			return e
		}
		now := s.now()
		if e = s.recover(ctx, r, now); e != nil {
			return e
		}
		if !h.Enabled {
			return nil
		}
		jobs, e := r.BuildJobs().List(ctx)
		if e != nil {
			return e
		}
		active := 0
		for _, j := range jobs {
			if j.HostID != nil && *j.HostID == h.ID && j.AssignmentReleasedAt == nil {
				active++
			}
		}
		if active >= h.MaxParallel {
			return nil
		}
		for _, j := range jobs {
			if j.Status == "queued" && j.Target.Matches(h) {
				j.Status = "running"
				j.HostID = &h.ID
				j.StartedAt = &now
				j.Phase = "claimed"
				if e = r.BuildJobs().Assign(ctx, j); e != nil {
					return e
				}
				out = &j
				break
			}
		}
		return nil
	})
	return out, e
}
func (s *BuildJobService) OpenBundle(ctx context.Context, who AuthenticatedBuildHost, id string) (io.ReadCloser, domain.BuildJob, error) {
	var out io.ReadCloser
	var j domain.BuildJob
	if !domain.ValidBuildJobID(id) {
		return nil, j, domain.Invalidf("invalid job id")
	}
	e := s.store.Write(ctx, func(r Repos) error {
		if _, e := approvedJobHost(ctx, r, who); e != nil {
			return e
		}
		if e := s.recover(ctx, r, s.now()); e != nil {
			return e
		}
		var e error
		j, e = r.BuildJobs().Get(ctx, id)
		if e != nil {
			return e
		}
		if j.Status != "running" || j.HostID == nil || *j.HostID != who.ID || j.AssignmentReleasedAt != nil {
			return domain.Forbiddenf("job is not assigned to this host")
		}
		out, e = s.bundles.Open(id)
		return e
	})
	if e != nil && out != nil {
		out.Close()
		out = nil
	}
	return out, j, e
}

type BuildJobProgressReply struct {
	Stop    bool   `json:"stop"`
	Status  string `json:"status"`
	Attempt int    `json:"attempt"`
}

func (s *BuildJobService) Progress(ctx context.Context, who AuthenticatedBuildHost, id string, p domain.BuildJobProgress) (BuildJobProgressReply, error) {
	var reply BuildJobProgressReply
	if !domain.ValidBuildJobID(id) {
		return reply, domain.Invalidf("invalid job id")
	}
	if e := p.Validate(); e != nil {
		return reply, e
	}
	e := s.store.Write(ctx, func(r Repos) error {
		if _, e := approvedJobHost(ctx, r, who); e != nil {
			return e
		}
		now := s.now()
		if e := s.recover(ctx, r, now); e != nil {
			return e
		}
		j, e := r.BuildJobs().Get(ctx, id)
		if e != nil {
			return e
		}
		reply = BuildJobProgressReply{Stop: true, Status: j.Status, Attempt: j.Attempt}
		if j.Attempt != p.Attempt {
			return nil
		}
		if j.HostID == nil {
			return nil
		}
		if *j.HostID != who.ID {
			return domain.Forbiddenf("job is assigned to another host")
		}
		if j.Status == "superseded" || j.Status == "cancelled" {
			if p.Phase == "stopped" && j.AssignmentReleasedAt == nil {
				j.AssignmentReleasedAt = &now
				return r.BuildJobs().Save(ctx, j)
			}
			return nil
		}
		if j.Status != "running" || j.AssignmentReleasedAt != nil {
			return nil
		}
		reply.Stop = false
		j.Phase = p.Phase
		j.Detail = p.Detail
		return r.BuildJobs().Save(ctx, j)
	})
	return reply, e
}
func (s *BuildJobService) Complete(ctx context.Context, who AuthenticatedBuildHost, id string, c domain.BuildJobCompletion) (domain.BuildJob, error) {
	var j domain.BuildJob
	if !domain.ValidBuildJobID(id) {
		return j, domain.Invalidf("invalid job id")
	}
	if e := c.Validate(); e != nil {
		return j, e
	}
	e := s.store.Write(ctx, func(r Repos) error {
		if _, e := approvedJobHost(ctx, r, who); e != nil {
			return e
		}
		now := s.now()
		if e := s.recover(ctx, r, now); e != nil {
			return e
		}
		var e error
		j, e = r.BuildJobs().Get(ctx, id)
		if e != nil {
			return e
		}
		if j.Attempt != c.Attempt || j.HostID == nil {
			return domain.Conflictf("stale assignment")
		}
		if *j.HostID != who.ID {
			return domain.Forbiddenf("job is assigned to another host")
		}
		if j.Status == "superseded" || j.Status == "cancelled" {
			if j.AssignmentReleasedAt == nil {
				j.AssignmentReleasedAt = &now
				return r.BuildJobs().Save(ctx, j)
			}
			return nil
		}
		if j.Status == "passed" || j.Status == "failed" {
			if j.Status == c.Status && bytes.Equal(j.Result, c.Result) {
				return nil
			}
			return domain.Conflictf("terminal report conflicts with stored result")
		}
		if j.Status != "running" || j.AssignmentReleasedAt != nil {
			return domain.Conflictf("stale assignment")
		}
		j.Status = c.Status
		j.Result = c.Result
		j.FinishedAt = &now
		j.AssignmentReleasedAt = &now
		j.Phase = c.Status
		return r.BuildJobs().Save(ctx, j)
	})
	return j, e
}
func (s *BuildJobService) Cancel(ctx context.Context, isOwner bool, id string) (domain.BuildJob, error) {
	var j domain.BuildJob
	if !isOwner {
		return j, domain.Forbiddenf("administrator required")
	}
	if !domain.ValidBuildJobID(id) {
		return j, domain.Invalidf("invalid job id")
	}
	e := s.store.Write(ctx, func(r Repos) error {
		var e error
		j, e = r.BuildJobs().Get(ctx, id)
		if e != nil {
			return e
		}
		if j.Status == "cancelled" {
			return nil
		}
		if j.Status != "queued" && j.Status != "running" {
			return domain.Conflictf("job is already terminal")
		}
		now := s.now()
		j.Status = "cancelled"
		j.FinishedAt = &now
		return r.BuildJobs().Save(ctx, j)
	})
	return j, e
}

// RecoverStorage must finish before accepting HTTP requests. Interrupted uploads
// are retained; referenced payload corruption prevents startup.
func (s *BuildJobService) RecoverStorage(ctx context.Context) error {
	return s.store.Write(ctx, func(r Repos) error {
		if _, e := s.bundles.PayloadSizes(); e != nil {
			return e
		}
		uploads, e := r.BuildJobs().Uploads(ctx)
		if e != nil {
			return e
		}
		for _, u := range uploads {
			if e = r.BuildJobs().FinishUpload(ctx, u.Token); e != nil {
				return e
			}
		}
		jobs, e := r.BuildJobs().List(ctx)
		if e != nil {
			return e
		}
		for _, j := range jobs {
			if e = s.bundles.Verify(j.ID, j.BundleBytes, j.BundleSHA256); e != nil {
				return fmt.Errorf("stored bundle %s: %w", j.ID, e)
			}
		}
		return nil
	})
}
