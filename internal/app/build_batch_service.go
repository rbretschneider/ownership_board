package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// Batched verification (docs/architecture.md, "Batched verification"). A repo
// that opts in has its queued commits verified together: a host merges up to
// the repo's batch size of ticket commits, in queue order, and runs the
// verification once. A pass is every member's pass, recorded against each
// member's own job and exact commit. A red batch is split in half and re-run
// until each culprit runs alone. Each ticket keeps its own BuildJob throughout;
// a batch only holds queued jobs and settles them.

// batchAssignment returns a batch for the host to run, or nil: a waiting
// bisection half first, otherwise a new batch led by lead when its repo opts in
// and enough queued commits can join it.
func (s *BuildJobService) batchAssignment(ctx context.Context, r Repos, h domain.BuildHost, lead *domain.BuildJob, now time.Time) (*domain.BuildBatch, error) {
	if !domain.SupportsBatches(h.AgentVersion) {
		return nil, nil
	}
	queued, err := r.BuildBatches().QueuedForHost(ctx, h)
	if err != nil {
		return nil, err
	}
	for i := range queued {
		b := &queued[i]
		live, err := s.refreshMembers(ctx, r, b)
		if err != nil {
			return nil, err
		}
		if len(live) < 2 {
			// Supersession left one commit or none: it is verified alone.
			if err = s.dissolve(ctx, r, b, now, domain.MemberReturned, true, "batch fell below two commits"); err != nil {
				return nil, err
			}
			continue
		}
		return b, s.assignBatch(ctx, r, b, h, now)
	}
	if lead == nil || lead.BatchSolo {
		return nil, nil
	}
	b, err := s.assemble(ctx, r, *lead, now)
	if err != nil || b == nil {
		return nil, err
	}
	if err = r.BuildBatches().Insert(ctx, *b); err != nil {
		return nil, err
	}
	return b, s.assignBatch(ctx, r, b, h, now)
}

// assemble builds a new batch around lead, or returns nil when the lead's repo
// does not batch, the lead is hot (likely red, so it would bisect the rest), or
// no other queued commit can join.
func (s *BuildJobService) assemble(ctx context.Context, r Repos, lead domain.BuildJob, now time.Time) (*domain.BuildBatch, error) {
	ticket, err := r.Tickets().Get(ctx, lead.TicketID)
	if err != nil {
		return nil, err
	}
	if ticket.RepoID <= 0 {
		return nil, nil
	}
	repo, err := r.CodeRepos().Get(ctx, ticket.RepoID)
	if err != nil {
		return nil, err
	}
	if !repo.BatchVerify || repo.FullName != lead.Repo {
		return nil, nil
	}
	if hot, err := isHot(ctx, r, lead.TicketID); err != nil || hot {
		return nil, err
	}
	size := repo.BatchMax
	if !domain.ValidBatchSize(size) {
		size = domain.DefaultBatchSize
	}
	candidates, err := r.BuildJobs().BatchCandidates(ctx, lead, 40)
	if err != nil {
		return nil, err
	}
	jobs := []domain.BuildJob{lead}
	tickets := map[int64]bool{lead.TicketID: true}
	for _, c := range candidates {
		if len(jobs) >= size {
			break
		}
		if tickets[c.TicketID] {
			continue
		}
		if hot, err := isHot(ctx, r, c.TicketID); err != nil {
			return nil, err
		} else if hot {
			continue
		}
		tickets[c.TicketID] = true
		jobs = append(jobs, c)
	}
	if len(jobs) < 2 {
		return nil, nil
	}
	// Queue order is merge order: the oldest commit leads.
	slices.SortStableFunc(jobs, func(a, b domain.BuildJob) int {
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}
		return compareStrings(a.ID, b.ID)
	})
	return newBatch(lead, jobs, "", now)
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func newBatch(like domain.BuildJob, jobs []domain.BuildJob, parent string, now time.Time) (*domain.BuildBatch, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	b := &domain.BuildBatch{ID: hex.EncodeToString(raw), Repo: like.Repo, Preset: like.Preset, Target: like.Target,
		Status: domain.BatchQueued, ParentID: parent, Attempt: 1, CreatedAt: now}
	for i, j := range jobs {
		b.Members = append(b.Members, domain.BuildBatchMember{JobID: j.ID, TicketID: j.TicketID, SHA: j.SHA, Position: i,
			Outcome: domain.MemberPending, BundleBytes: j.BundleBytes, BundleSHA256: j.BundleSHA256})
	}
	return b, nil
}

func (s *BuildJobService) assignBatch(ctx context.Context, r Repos, b *domain.BuildBatch, h domain.BuildHost, now time.Time) error {
	b.Status = domain.BatchRunning
	b.HostID = &h.ID
	b.StartedAt = &now
	b.FinishedAt, b.AssignmentReleasedAt = nil, nil
	b.Phase, b.Detail, b.Result = "claimed", "batch of "+b.TicketList(0), nil
	// Members the host must merge are the live ones only: a superseded commit
	// never joins the candidate.
	live := b.Members[:0:0]
	for _, m := range b.Members {
		if m.Outcome == domain.MemberPending {
			live = append(live, m)
		}
	}
	if err := r.BuildBatches().Save(ctx, *b); err != nil {
		return err
	}
	b.Members = live
	return nil
}

// refreshMembers marks members whose job is no longer held by the batch (a
// newer commit, a finished ticket or the owner replaced it) as superseded, and
// returns the live members in merge order. A stale member's result never counts.
func (s *BuildJobService) refreshMembers(ctx context.Context, r Repos, b *domain.BuildBatch) ([]domain.BuildBatchMember, error) {
	var live []domain.BuildBatchMember
	for i := range b.Members {
		m := &b.Members[i]
		if m.Outcome != domain.MemberPending {
			continue
		}
		j, err := r.BuildJobs().Get(ctx, m.JobID)
		if err != nil {
			return nil, err
		}
		if j.Status != "queued" || j.BatchID != b.ID || j.SHA != m.SHA {
			m.Outcome = domain.MemberSuperseded
			if j.BatchID == b.ID {
				if err = r.BuildJobs().SetBatch(ctx, j.ID, "", j.BatchSolo); err != nil {
					return nil, err
				}
			}
			continue
		}
		live = append(live, *m)
	}
	return live, nil
}

// dissolve ends a batch without a verdict and frees its live members.
func (s *BuildJobService) dissolve(ctx context.Context, r Repos, b *domain.BuildBatch, now time.Time, outcome string, solo bool, why string) error {
	if _, err := s.refreshMembers(ctx, r, b); err != nil {
		return err
	}
	for i := range b.Members {
		m := &b.Members[i]
		if m.Outcome != domain.MemberPending {
			continue
		}
		m.Outcome = outcome
		if err := r.BuildJobs().SetBatch(ctx, m.JobID, "", solo); err != nil {
			return err
		}
	}
	if b.Status == domain.BatchQueued || b.Status == domain.BatchRunning {
		b.Status = domain.BatchDissolved
	}
	b.FinishedAt = &now
	b.Detail = why
	return r.BuildBatches().Save(ctx, *b)
}

// isBatch reports whether id names a batch rather than a ticket job.
func isBatch(ctx context.Context, r Repos, id string) (domain.BuildBatch, bool, error) {
	if _, err := r.BuildJobs().Get(ctx, id); err == nil {
		return domain.BuildBatch{}, false, nil
	} else if domain.KindOf(err) != domain.KindNotFound {
		return domain.BuildBatch{}, false, err
	}
	b, err := r.BuildBatches().Get(ctx, id)
	if domain.KindOf(err) == domain.KindNotFound {
		return b, false, nil
	}
	return b, err == nil, err
}

// batchProgress is Progress for a batch assignment.
func (s *BuildJobService) batchProgress(ctx context.Context, r Repos, who AuthenticatedBuildHost, b domain.BuildBatch, p domain.BuildJobProgress, now time.Time) (BuildJobProgressReply, error) {
	reply := BuildJobProgressReply{Stop: true, Status: b.Status, Attempt: b.Attempt}
	if b.Attempt != p.Attempt || b.HostID == nil {
		return reply, nil
	}
	if *b.HostID != who.ID {
		return reply, domain.Forbiddenf("job is assigned to another host")
	}
	if b.Status != domain.BatchRunning {
		if p.Phase == "stopped" && b.AssignmentReleasedAt == nil {
			b.AssignmentReleasedAt = &now
			return reply, r.BuildBatches().Save(ctx, b)
		}
		return reply, nil
	}
	if b.AssignmentReleasedAt != nil {
		return reply, nil
	}
	live, err := s.refreshMembers(ctx, r, &b)
	if err != nil {
		return reply, err
	}
	if len(live) == 0 {
		// Every member was superseded: nothing this run says would count.
		b.Status = domain.BatchCancelled
		b.FinishedAt = &now
		b.Detail = "every commit in the batch was superseded"
		reply.Status = b.Status
		return reply, r.BuildBatches().Save(ctx, b)
	}
	reply.Stop = false
	b.Phase, b.Detail = p.Phase, p.Detail
	return reply, r.BuildBatches().Save(ctx, b)
}

// batchComplete is Complete for a batch assignment: it settles every member.
func (s *BuildJobService) batchComplete(ctx context.Context, r Repos, host domain.BuildHost, b domain.BuildBatch, c domain.BuildJobCompletion, now time.Time) (domain.BuildJob, error) {
	if b.Attempt != c.Attempt || b.HostID == nil {
		return b.Assignment(), domain.Conflictf("stale assignment")
	}
	if *b.HostID != host.ID {
		return b.Assignment(), domain.Forbiddenf("job is assigned to another host")
	}
	switch b.Status {
	case domain.BatchCancelled, domain.BatchDissolved:
		if b.AssignmentReleasedAt == nil {
			b.AssignmentReleasedAt = &now
			return b.Assignment(), r.BuildBatches().Save(ctx, b)
		}
		return b.Assignment(), nil
	case domain.BatchPassed, domain.BatchFailed:
		if bytes.Equal(b.Result, c.Result) {
			return b.Assignment(), nil
		}
		return b.Assignment(), domain.Conflictf("terminal report conflicts with stored result")
	}
	if b.Status != domain.BatchRunning || b.AssignmentReleasedAt != nil {
		return b.Assignment(), domain.Conflictf("stale assignment")
	}
	var result domain.BuildJobResult
	if err := json.Unmarshal(c.Result, &result); err != nil {
		return b.Assignment(), domain.Invalidf("invalid result")
	}
	excluded := map[string]bool{}
	for _, id := range result.Excluded {
		if !slices.ContainsFunc(b.Members, func(m domain.BuildBatchMember) bool { return m.JobID == id }) || id == b.Lead().JobID {
			return b.Assignment(), domain.Invalidf("excluded member is not in the batch")
		}
		excluded[id] = true
	}
	live, err := s.refreshMembers(ctx, r, &b)
	if err != nil {
		return b.Assignment(), err
	}
	b.Result = c.Result
	b.FinishedAt = &now
	b.AssignmentReleasedAt = &now
	b.Phase = c.Status
	// A member that did not merge cleanly was not verified: it runs alone, as before batching.
	var verified []domain.BuildBatchMember
	for _, m := range live {
		if !excluded[m.JobID] {
			verified = append(verified, m)
			continue
		}
		setOutcome(&b, m.JobID, domain.MemberConflict)
		if err = r.BuildJobs().SetBatch(ctx, m.JobID, "", true); err != nil {
			return b.Assignment(), err
		}
		if err = noteBatch(ctx, r, m.TicketID, now, b.ID, fmt.Sprintf("Commit %s did not merge cleanly into a verification batch with %s, so it is verified on its own.", short(m.SHA), b.TicketList(m.TicketID))); err != nil {
			return b.Assignment(), err
		}
	}
	reporter := fmt.Sprintf("build-host/%d batch %s", host.ID, b.ID[:12])
	switch {
	case c.Status == "passed":
		b.Status = domain.BatchPassed
		for _, m := range verified {
			if err = s.passMember(ctx, r, &b, m, reporter, now); err != nil {
				return b.Assignment(), err
			}
		}
	case domain.JobResultBlamesHost(c.Result):
		// No verdict: the host failed, not the commits. Run the batch again, or
		// after its attempts send each commit to be verified alone.
		if b.Attempt < BuildJobAttempts && len(verified) >= 2 {
			b.Status = domain.BatchQueued
			b.Attempt++
			b.HostID, b.StartedAt, b.FinishedAt, b.AssignmentReleasedAt = nil, nil, nil, nil
			b.Phase, b.Detail, b.Result = "", "", nil
		} else {
			b.Status = domain.BatchDissolved
			b.Detail = "no verdict from the host; each commit is verified alone"
			for _, m := range verified {
				setOutcome(&b, m.JobID, domain.MemberReturned)
				if err = r.BuildJobs().SetBatch(ctx, m.JobID, "", true); err != nil {
					return b.Assignment(), err
				}
			}
		}
	default:
		b.Status = domain.BatchFailed
		if err = s.bisect(ctx, r, &b, verified, now); err != nil {
			return b.Assignment(), err
		}
	}
	return b.Assignment(), r.BuildBatches().Save(ctx, b)
}

// passMember fans a batch pass out to one member: its own job passes and the
// board records the verdict for the member's exact commit.
func (s *BuildJobService) passMember(ctx context.Context, r Repos, b *domain.BuildBatch, m domain.BuildBatchMember, reporter string, now time.Time) error {
	j, err := r.BuildJobs().Get(ctx, m.JobID)
	if err != nil {
		return err
	}
	j.Status, j.Result, j.HostID, j.StartedAt = "passed", b.Result, b.HostID, b.StartedAt
	j.FinishedAt, j.AssignmentReleasedAt = &now, &now
	j.Phase = "passed"
	j.Detail = "verified in batch with " + b.TicketList(m.TicketID)
	if err = r.BuildJobs().Save(ctx, j); err != nil {
		return err
	}
	if err = r.BuildJobs().SetBatch(ctx, j.ID, "", false); err != nil {
		return err
	}
	setOutcome(b, m.JobID, domain.MemberPassed)
	if v, ok := domain.VerdictForJob(j, reporter, now); ok {
		if err = r.Verdicts().Record(ctx, v); err != nil {
			return err
		}
	}
	return noteBatch(ctx, r, m.TicketID, now, b.ID, fmt.Sprintf("Verification passed for commit %s in a batch with %s.", short(m.SHA), b.TicketList(m.TicketID)))
}

// bisect splits a red batch's verified members in half. A half of two or more
// becomes a queued batch; a single commit is verified alone, which gives it its
// own verdict and logs. A culprit is isolated in about log2(n) runs and the
// other commits still pass.
func (s *BuildJobService) bisect(ctx context.Context, r Repos, b *domain.BuildBatch, members []domain.BuildBatchMember, now time.Time) error {
	half := (len(members) + 1) / 2
	parts := [][]domain.BuildBatchMember{members[:half], members[half:]}
	lead := domain.BuildJob{Repo: b.Repo, Preset: b.Preset, Target: b.Target}
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		for _, m := range part {
			setOutcome(b, m.JobID, domain.MemberBisected)
		}
		if len(part) == 1 {
			m := part[0]
			if err := r.BuildJobs().SetBatch(ctx, m.JobID, "", true); err != nil {
				return err
			}
			if err := noteBatch(ctx, r, m.TicketID, now, b.ID, fmt.Sprintf("A verification batch with %s failed; commit %s is now verified on its own.", b.TicketList(m.TicketID), short(m.SHA))); err != nil {
				return err
			}
			continue
		}
		jobs := make([]domain.BuildJob, 0, len(part))
		for _, m := range part {
			// Free the job from the red batch before the half holds it.
			if err := r.BuildJobs().SetBatch(ctx, m.JobID, "", false); err != nil {
				return err
			}
			jobs = append(jobs, domain.BuildJob{ID: m.JobID, TicketID: m.TicketID, SHA: m.SHA, BundleBytes: m.BundleBytes, BundleSHA256: m.BundleSHA256})
		}
		child, err := newBatch(lead, jobs, b.ID, now)
		if err != nil {
			return err
		}
		if err = r.BuildBatches().Insert(ctx, *child); err != nil {
			return err
		}
		for _, m := range part {
			if err := noteBatch(ctx, r, m.TicketID, now, b.ID, fmt.Sprintf("A verification batch with %s failed; commit %s is re-checked in a smaller batch with %s.", b.TicketList(m.TicketID), short(m.SHA), child.TicketList(m.TicketID))); err != nil {
				return err
			}
		}
	}
	return nil
}

func setOutcome(b *domain.BuildBatch, jobID, outcome string) {
	for i := range b.Members {
		if b.Members[i].JobID == jobID {
			b.Members[i].Outcome = outcome
		}
	}
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// noteBatch records a batch event on a ticket's timeline.
func noteBatch(ctx context.Context, r Repos, ticketID int64, now time.Time, batchID, body string) error {
	entry := domain.NewEntry(ticketID, domain.TimelineComment, domain.SystemActor, now).WithBody(body)
	entry.Data["kind"] = "build_batch"
	entry.Data["batch_id"] = batchID
	return r.Timeline().Append(ctx, &entry)
}

// recoverBatches is the reaper for batches whose host went silent: the batch
// runs again, or after its attempts each commit is verified alone.
func (s *BuildJobService) recoverBatches(ctx context.Context, r Repos, now time.Time) error {
	batches, err := r.BuildBatches().ActiveAssignments(ctx)
	if err != nil {
		return err
	}
	hosts := map[int64]domain.BuildHost{}
	for i := range batches {
		b := &batches[i]
		if b.HostID == nil || b.StartedAt == nil {
			continue
		}
		h, ok := hosts[*b.HostID]
		if !ok {
			if h, err = r.BuildHosts().Get(ctx, *b.HostID); err != nil {
				return err
			}
			hosts[h.ID] = h
		}
		seen := b.StartedAt
		if h.LastSeenAt != nil {
			seen = h.LastSeenAt
		}
		if h.Status != domain.BuildHostRevoked && now.Sub(*seen) < 3*time.Minute {
			continue
		}
		if b.Status != domain.BatchRunning {
			b.AssignmentReleasedAt = &now
			if err = r.BuildBatches().Save(ctx, *b); err != nil {
				return err
			}
			continue
		}
		if b.Attempt >= BuildJobAttempts {
			b.AssignmentReleasedAt = &now
			if err = s.dissolve(ctx, r, b, now, domain.MemberReturned, true, "host connectivity lost after three attempts"); err != nil {
				return err
			}
			continue
		}
		b.Status = domain.BatchQueued
		b.Attempt++
		b.HostID, b.StartedAt, b.FinishedAt, b.AssignmentReleasedAt = nil, nil, nil, nil
		b.Phase, b.Detail, b.Result = "", "", nil
		if err = r.BuildBatches().Save(ctx, *b); err != nil {
			return err
		}
	}
	return nil
}

// batchRunners returns the runner rows for batches hosts hold.
func batchRunners(ctx context.Context, r Repos) ([]RunnerAssignment, error) {
	batches, err := r.BuildBatches().ActiveAssignments(ctx)
	if err != nil {
		return nil, err
	}
	out := []RunnerAssignment{}
	for _, b := range batches {
		if b.HostID == nil || len(b.Members) == 0 {
			continue
		}
		a := RunnerAssignment{HostID: *b.HostID, ID: b.ID, TicketID: b.Lead().TicketID, Repo: b.Repo, SHA: b.Lead().SHA, Preset: b.Preset,
			Status: b.Status, Phase: b.Phase, Detail: b.Detail, StartedAt: b.StartedAt, Attempt: b.Attempt}
		for _, m := range b.Members {
			if m.Outcome == domain.MemberPending {
				a.Batch = append(a.Batch, m.TicketID)
			}
		}
		if status := b.Status; status == domain.BatchDissolved {
			a.Status = "cancelled"
		}
		out = append(out, a)
	}
	return out, nil
}

// reportsExcluded reports whether a completion names excluded batch members.
func reportsExcluded(result []byte) bool {
	var r domain.BuildJobResult
	return json.Unmarshal(result, &r) == nil && len(r.Excluded) > 0
}

func assignedTo(j domain.BuildJob, who AuthenticatedBuildHost) bool {
	return j.Status == "running" && j.HostID != nil && *j.HostID == who.ID && j.AssignmentReleasedAt == nil
}

// heldByBatchOf reports whether j is a member of a batch running on who, so the
// host may fetch the member's bundle. A member superseded mid-run may still be
// fetched: its result never counts.
func heldByBatchOf(ctx context.Context, r Repos, j domain.BuildJob, who AuthenticatedBuildHost) bool {
	if j.BatchID == "" {
		return false
	}
	b, err := r.BuildBatches().Get(ctx, j.BatchID)
	return err == nil && b.Status == domain.BatchRunning && b.HostID != nil && *b.HostID == who.ID && b.AssignmentReleasedAt == nil
}

// cancelBatch is the owner stopping a batch: its commits return to the queue
// and may batch again.
func (s *BuildJobService) cancelBatch(ctx context.Context, r Repos, b domain.BuildBatch, now time.Time) (domain.BuildJob, error) {
	if b.Status == domain.BatchCancelled {
		return b.Assignment(), nil
	}
	if b.Status != domain.BatchQueued && b.Status != domain.BatchRunning {
		return b.Assignment(), domain.Conflictf("job is already terminal")
	}
	if err := s.dissolve(ctx, r, &b, now, domain.MemberReturned, false, "cancelled by the owner"); err != nil {
		return b.Assignment(), err
	}
	b.Status = domain.BatchCancelled
	return b.Assignment(), r.BuildBatches().Save(ctx, b)
}
