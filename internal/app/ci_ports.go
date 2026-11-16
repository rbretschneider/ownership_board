package app

import (
	"context"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// CIReader reads one hosting provider's CI for a repository, named by its
// full name ("owner/name"). Adapters map their provider's vocabulary onto the
// domain's CI model.
type CIReader interface {
	// Runs lists recent runs, newest first, without their jobs.
	Runs(ctx context.Context, repo string, query domain.CIRunQuery) ([]domain.CIRun, error)
	// Run reads one run with its jobs.
	Run(ctx context.Context, repo string, runID int64) (domain.CIRun, error)
	Job(ctx context.Context, repo string, jobID int64) (domain.CIJob, error)
	// JobLogTail returns at most maxBytes from the end of a job's log.
	JobLogTail(ctx context.Context, repo string, jobID int64, maxBytes int) (string, error)
	// BranchHead returns the branch's current head commit, or "" when the
	// branch does not exist.
	BranchHead(ctx context.Context, repo, branch string) (string, error)
	// PullHead returns a pull request's head branch and commit.
	PullHead(ctx context.Context, repo string, number int) (branch, sha string, err error)
	Runners(ctx context.Context, repo string) ([]domain.CIRunner, error)
}

// CIActions changes one hosting provider's CI.
type CIActions interface {
	RerunFailedJobs(ctx context.Context, repo string, runID int64) error
	// RerunRun re-runs every job of a run, for a run cancelled before its
	// jobs could fail (a concurrency-group eviction has no failed job).
	RerunRun(ctx context.Context, repo string, runID int64) error
	RerunJob(ctx context.Context, repo string, jobID int64) error
	CancelRun(ctx context.Context, repo string, runID int64) error
}

// CIProvider is a provider's CI, read and act.
type CIProvider interface {
	CIReader
	CIActions
}

// CICatalog hands out the CI of a provider the board is connected to, with
// the same credentials it syncs repositories with.
type CICatalog interface {
	CI(ctx context.Context, provider domain.Provider) (CIProvider, error)
}
