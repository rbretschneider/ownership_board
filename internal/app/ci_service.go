package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// CI read and action limits.
const (
	ciTicketRuns     = 10
	ciRepoRuns       = 20
	ciRunsWithJobs   = 5
	defaultLogBytes  = 6_000
	maxLogBytes      = 20_000
	maxCIReasonChars = 500
)

// CIService gives the board manager (and people) the CI of ticket PRs and
// default branches through the board, with the board's guards on every
// action and every action on the ticket's history.
type CIService struct {
	store   Store
	clock   Clock
	catalog CICatalog
	writer  ticketWriter
}

// NewCIService builds the service.
func NewCIService(store Store, clock Clock, sink EventSink, catalog CICatalog) *CIService {
	return &CIService{store: store, clock: clock, catalog: catalog, writer: ticketWriter{store: store, sink: sink, clock: clock}}
}

// CITarget names what to look at: a ticket's PR, or a repository's default
// branch. Exactly one is set.
type CITarget struct {
	TicketID int64
	RepoID   int64
}

// CIStatusView is a ticket's or repository's CI right now.
type CIStatusView struct {
	Repo     domain.Repo
	TicketID int64
	// Branch and HeadSHA are the branch looked at and its current head.
	Branch  string
	HeadSHA string
	// Runs are newest first; the newest few that matter carry their jobs.
	Runs    []domain.CIRun
	Runners []domain.CIRunner
	// RunnersNote says why runners are missing, when the provider refused.
	RunnersNote string
	ObservedAt  time.Time
}

// CIJobLog is a job and the end of its log.
type CIJobLog struct {
	Job  domain.CIJob
	Tail string
}

// CIRerunInput re-runs a run's failed jobs, or one job.
type CIRerunInput struct {
	TicketID int64
	RunID    int64
	JobID    int64
	Reason   string
}

// CICancelInput cancels a run.
type CICancelInput struct {
	TicketID int64
	RunID    int64
	Reason   string
}

// ciContext is a resolved target: its repository, ticket and provider.
type ciContext struct {
	repo     domain.Repo
	ticket   *domain.Ticket
	provider CIProvider
}

// Status reads the CI of a ticket's PR branch or of a repository's default
// branch.
func (s *CIService) Status(ctx context.Context, actor domain.Actor, target CITarget) (CIStatusView, error) {
	if err := requireManagerOrPerson(actor); err != nil {
		return CIStatusView{}, err
	}
	resolved, err := s.resolve(ctx, target)
	if err != nil {
		return CIStatusView{}, err
	}
	var view CIStatusView
	if resolved.ticket != nil {
		view, err = s.ticketStatus(ctx, resolved)
	} else {
		view, err = s.repoStatus(ctx, resolved)
	}
	view.ObservedAt = s.clock.Now()
	return view, err
}

// FailedLog returns a job's failed steps and the end of its log.
func (s *CIService) FailedLog(ctx context.Context, actor domain.Actor, target CITarget, jobID int64, maxBytes int) (CIJobLog, error) {
	if err := requireManagerOrPerson(actor); err != nil {
		return CIJobLog{}, err
	}
	if jobID <= 0 {
		return CIJobLog{}, domain.Invalidf("job_id is required")
	}
	resolved, err := s.resolve(ctx, target)
	if err != nil {
		return CIJobLog{}, err
	}
	job, err := resolved.provider.Job(ctx, resolved.repo.FullName, jobID)
	if err != nil {
		return CIJobLog{}, err
	}
	tail, err := resolved.provider.JobLogTail(ctx, resolved.repo.FullName, jobID, logBytes(maxBytes))
	if err != nil {
		return CIJobLog{}, err
	}
	return CIJobLog{Job: job, Tail: tail}, nil
}

// Rerun re-runs a completed run's failed jobs, or one job of a completed
// run, and records it on the ticket.
func (s *CIService) Rerun(ctx context.Context, actor domain.Actor, input CIRerunInput) (domain.CIRun, error) {
	if (input.RunID > 0) == (input.JobID > 0) {
		return domain.CIRun{}, domain.Invalidf("give exactly one of run_id or job_id")
	}
	resolved, reason, err := s.resolveAction(ctx, actor, input.TicketID, input.Reason)
	if err != nil {
		return domain.CIRun{}, err
	}
	if input.JobID > 0 {
		return s.rerunJob(ctx, actor, resolved, input.JobID, reason)
	}
	run, err := resolved.provider.Run(ctx, resolved.repo.FullName, input.RunID)
	if err != nil {
		return domain.CIRun{}, err
	}
	if err := domain.CheckCIRerun(run); err != nil {
		return domain.CIRun{}, err
	}
	if err := rerunFor(resolved, run)(ctx, resolved.repo.FullName, run.ID); err != nil {
		return domain.CIRun{}, err
	}
	return run, s.record(ctx, actor, resolved, ciRecord{action: domain.CIActionRerunRun, run: run, reason: reason})
}

// Cancel cancels a run that a newer commit replaced, and records it on the
// ticket. It never cancels CI on a branch's current head.
func (s *CIService) Cancel(ctx context.Context, actor domain.Actor, input CICancelInput) (domain.CIRun, error) {
	if input.RunID <= 0 {
		return domain.CIRun{}, domain.Invalidf("run_id is required")
	}
	resolved, reason, err := s.resolveAction(ctx, actor, input.TicketID, input.Reason)
	if err != nil {
		return domain.CIRun{}, err
	}
	run, err := resolved.provider.Run(ctx, resolved.repo.FullName, input.RunID)
	if err != nil {
		return domain.CIRun{}, err
	}
	head, err := resolved.provider.BranchHead(ctx, resolved.repo.FullName, run.HeadBranch)
	if err != nil {
		return domain.CIRun{}, err
	}
	if err := domain.CheckCICancel(run, head); err != nil {
		return domain.CIRun{}, err
	}
	if err := resolved.provider.CancelRun(ctx, resolved.repo.FullName, run.ID); err != nil {
		return domain.CIRun{}, err
	}
	return run, s.record(ctx, actor, resolved, ciRecord{action: domain.CIActionCancel, run: run, reason: reason})
}

// rerunFor picks how to re-run a completed run: only its failed jobs when it
// failed, the whole run when it was cancelled, since a run evicted by a
// concurrency group may have no failed job to re-run.
func rerunFor(resolved ciContext, run domain.CIRun) func(context.Context, string, int64) error {
	if run.Conclusion == domain.CICancelled {
		return resolved.provider.RerunRun
	}
	return resolved.provider.RerunFailedJobs
}

func (s *CIService) rerunJob(ctx context.Context, actor domain.Actor, resolved ciContext, jobID int64, reason string) (domain.CIRun, error) {
	job, err := resolved.provider.Job(ctx, resolved.repo.FullName, jobID)
	if err != nil {
		return domain.CIRun{}, err
	}
	run, err := resolved.provider.Run(ctx, resolved.repo.FullName, job.RunID)
	if err != nil {
		return domain.CIRun{}, err
	}
	if err := domain.CheckCIJobRerun(run, job); err != nil {
		return domain.CIRun{}, err
	}
	if err := resolved.provider.RerunJob(ctx, resolved.repo.FullName, job.ID); err != nil {
		return domain.CIRun{}, err
	}
	return run, s.record(ctx, actor, resolved, ciRecord{action: domain.CIActionRerunJob, run: run, jobID: job.ID, reason: reason})
}

func (s *CIService) ticketStatus(ctx context.Context, resolved ciContext) (CIStatusView, error) {
	view := CIStatusView{Repo: resolved.repo, TicketID: resolved.ticket.ID}
	var err error
	view.Branch, view.HeadSHA, err = ticketHead(ctx, resolved.provider, resolved.repo, *resolved.ticket)
	if err != nil || view.Branch == "" {
		return view, err
	}
	runs, err := resolved.provider.Runs(ctx, resolved.repo.FullName, domain.CIRunQuery{Branch: view.Branch, Limit: ciTicketRuns})
	if err != nil {
		return view, err
	}
	view.Runs, err = withJobs(ctx, resolved, runs, func(run domain.CIRun) bool {
		return domain.SameSHA(run.HeadSHA, view.HeadSHA) || isRunning(run)
	}, ciRunsWithJobs)
	return view, err
}

func (s *CIService) repoStatus(ctx context.Context, resolved ciContext) (CIStatusView, error) {
	repo := resolved.repo
	view := CIStatusView{Repo: repo, Branch: repo.DefaultBranch}
	var err error
	if repo.DefaultBranch != "" {
		if view.HeadSHA, err = resolved.provider.BranchHead(ctx, repo.FullName, repo.DefaultBranch); err != nil {
			return view, err
		}
	}
	runs, err := resolved.provider.Runs(ctx, repo.FullName, domain.CIRunQuery{Limit: ciRepoRuns})
	if err != nil {
		return view, err
	}
	if view.Runs, err = withJobs(ctx, resolved, runs, isRunning, ciRunsWithJobs); err != nil {
		return view, err
	}
	view.Runners, err = resolved.provider.Runners(ctx, repo.FullName)
	if err != nil {
		// Reading runners needs admin rights many tokens lack; the rest of the view still stands.
		view.Runners, view.RunnersNote = nil, err.Error()
	}
	return view, nil
}

// withJobs fills the jobs of the newest runs that match, at most limit of
// them, so a long history costs one call per run that matters.
func withJobs(ctx context.Context, resolved ciContext, runs []domain.CIRun, wanted func(domain.CIRun) bool, limit int) ([]domain.CIRun, error) {
	filled := 0
	for i := range runs {
		if filled == limit || !wanted(runs[i]) {
			continue
		}
		detailed, err := resolved.provider.Run(ctx, resolved.repo.FullName, runs[i].ID)
		if err != nil {
			return nil, err
		}
		runs[i] = detailed
		filled++
	}
	return runs, nil
}

// isRunning matches runs that have not completed: only those can have a job
// waiting for a runner.
func isRunning(run domain.CIRun) bool {
	return !run.IsCompleted()
}

// ticketHead resolves a ticket's head branch and commit: its pull request's
// when the PR link names one, else the branch the worker recorded.
func ticketHead(ctx context.Context, provider CIReader, repo domain.Repo, ticket domain.Ticket) (string, string, error) {
	if number, isPull := domain.PullNumberOf(ticket.PRURL); isPull {
		return provider.PullHead(ctx, repo.FullName, number)
	}
	if ticket.Branch == "" {
		return "", "", nil
	}
	sha, err := provider.BranchHead(ctx, repo.FullName, ticket.Branch)
	return ticket.Branch, sha, err
}

// resolve loads the target's repository and ticket and the provider's CI.
func (s *CIService) resolve(ctx context.Context, target CITarget) (ciContext, error) {
	if (target.TicketID > 0) == (target.RepoID > 0) {
		return ciContext{}, domain.Invalidf("give exactly one of ticket_id or repo_id")
	}
	var resolved ciContext
	err := s.store.Read(ctx, func(repos Repos) error {
		repoID := target.RepoID
		if target.TicketID > 0 {
			ticket, err := repos.Tickets().Get(ctx, target.TicketID)
			if err != nil {
				return err
			}
			resolved.ticket, repoID = &ticket, ticket.RepoID
		}
		if repoID == 0 {
			return domain.Invalidf("ticket %d has no repo, so it has no CI", target.TicketID)
		}
		repo, err := repos.CodeRepos().Get(ctx, repoID)
		resolved.repo = repo
		return err
	})
	if err != nil {
		return ciContext{}, err
	}
	resolved.provider, err = s.catalog.CI(ctx, resolved.repo.Provider)
	return resolved, err
}

// resolveAction checks who may act and why, and resolves the ticket the
// action is recorded on.
func (s *CIService) resolveAction(ctx context.Context, actor domain.Actor, ticketID int64, reason string) (ciContext, string, error) {
	if !actor.IsHuman() && !actor.IsBoardManager() {
		return ciContext{}, "", domain.Forbiddenf("only a person or the board manager can re-run or cancel CI")
	}
	if ticketID <= 0 {
		return ciContext{}, "", domain.Invalidf("ticket_id is required: every CI action is recorded on a ticket's history")
	}
	note, err := ciReason(actor, reason)
	if err != nil {
		return ciContext{}, "", err
	}
	resolved, err := s.resolve(ctx, CITarget{TicketID: ticketID})
	if err != nil {
		return ciContext{}, "", err
	}
	// Checked before acting on the provider; the history write checks again.
	if err := requireLeaseForAgent(*resolved.ticket, actor, s.clock.Now()); err != nil {
		return ciContext{}, "", err
	}
	return resolved, note, nil
}

func ciReason(actor domain.Actor, reason string) (string, error) {
	note := strings.TrimSpace(reason)
	if note == "" {
		return "", domain.Invalidf("reason is required: say in one line why, for the ticket's history")
	}
	if err := rejectHiddenFromAgent(actor, map[string]string{"reason": note}); err != nil {
		return "", err
	}
	return domain.LimitText("reason", note, maxCIReasonChars)
}

// ciRecord is one CI action for the ticket's history.
type ciRecord struct {
	action domain.CIActionKind
	run    domain.CIRun
	jobID  int64
	reason string
}

func (s *CIService) record(ctx context.Context, actor domain.Actor, resolved ciContext, action ciRecord) error {
	_, err := s.writer.change(ctx, actor, resolved.ticket.ID, func(_ context.Context, _ Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		entry := domain.NewEntry(ticket.ID, domain.TimelineCI, actor, s.clock.Now()).WithBody(action.reason).
			With("action", string(action.action)).With("run_id", action.run.ID).With("workflow", action.run.Workflow).
			With("head_sha", action.run.HeadSHA).With("repo", resolved.repo.FullName)
		if action.jobID > 0 {
			entry = entry.With("job_id", action.jobID)
		}
		return []domain.TimelineEntry{entry}, nil
	})
	if err != nil {
		return fmt.Errorf("the %s on run %d went through on the provider, but recording it on #%d failed: %w",
			action.action, action.run.ID, resolved.ticket.ID, err)
	}
	return nil
}

func logBytes(requested int) int {
	if requested <= 0 {
		return defaultLogBytes
	}
	return min(requested, maxLogBytes)
}
