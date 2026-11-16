package domain

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The board reads a repository's CI (GitHub Actions, Forgejo Actions) through
// one provider-agnostic model, so the manager sees a run as a run whatever
// hosts it. Providers map their own vocabulary onto these values.

// CIStatus is where a run or job is in its life.
type CIStatus string

// The CI statuses.
const (
	// CIQueued covers every state before a runner starts: queued, pending,
	// waiting on a concurrency group or an approval.
	CIQueued     CIStatus = "queued"
	CIInProgress CIStatus = "in_progress"
	CICompleted  CIStatus = "completed"
)

// CIConclusion is how a completed run or job ended; empty until it completes.
type CIConclusion string

// The CI conclusions.
const (
	CISuccess   CIConclusion = "success"
	CIFailure   CIConclusion = "failure"
	CICancelled CIConclusion = "cancelled"
	CISkipped   CIConclusion = "skipped"
	CITimedOut  CIConclusion = "timed_out"
	// CIOther is a conclusion the board has no use for (neutral, stale,
	// action_required), kept so it is never mistaken for a pass.
	CIOther CIConclusion = "other"
)

// IsFailure reports whether the conclusion means the code or the CI broke.
func (c CIConclusion) IsFailure() bool {
	return c == CIFailure || c == CITimedOut
}

// CIRun is one workflow run.
type CIRun struct {
	ID         int64        `json:"id"`
	Workflow   string       `json:"workflow"`
	Event      string       `json:"event"`
	HeadBranch string       `json:"head_branch"`
	HeadSHA    string       `json:"head_sha"`
	Status     CIStatus     `json:"status"`
	Conclusion CIConclusion `json:"conclusion"`
	Attempt    int          `json:"attempt"`
	URL        string       `json:"url"`
	CreatedAt  time.Time    `json:"created_at"`
	StartedAt  *time.Time   `json:"started_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
	// Jobs is filled only when the caller asked for them.
	Jobs []CIJob `json:"jobs"`
}

// CIJob is one job of a run.
type CIJob struct {
	ID          int64        `json:"id"`
	RunID       int64        `json:"run_id"`
	Name        string       `json:"name"`
	Status      CIStatus     `json:"status"`
	Conclusion  CIConclusion `json:"conclusion"`
	RunnerName  string       `json:"runner_name"`
	Labels      []string     `json:"labels"`
	CreatedAt   time.Time    `json:"created_at"`
	StartedAt   *time.Time   `json:"started_at"`
	CompletedAt *time.Time   `json:"completed_at"`
	// FailedSteps names the steps that failed, when the provider says.
	FailedSteps []string `json:"failed_steps"`
	URL         string   `json:"url"`
}

// CIRunner is one CI runner that serves the repository.
type CIRunner struct {
	Name     string   `json:"name"`
	IsOnline bool     `json:"is_online"`
	IsBusy   bool     `json:"is_busy"`
	Labels   []string `json:"labels"`
}

// IsCompleted reports whether the run has finished.
func (r CIRun) IsCompleted() bool {
	return r.Status == CICompleted
}

// IsRelease reports whether the run builds a release: a release event, or a
// workflow whose name says release (a tag push or a release dry run).
func (r CIRun) IsRelease() bool {
	return r.Event == "release" || strings.Contains(strings.ToLower(r.Workflow), "release")
}

// QueuedFor is how long the job has waited for a runner, or zero once one
// took it.
func (j CIJob) QueuedFor(now time.Time) time.Duration {
	if j.Status != CIQueued || j.CreatedAt.IsZero() {
		return 0
	}
	return now.Sub(j.CreatedAt)
}

// QueuedFor is how long a run with no jobs yet (a run held by a concurrency
// group, or one no runner has looked at) has waited, or zero.
func (r CIRun) QueuedFor(now time.Time) time.Duration {
	if r.Status != CIQueued || r.CreatedAt.IsZero() || len(r.Jobs) > 0 {
		return 0
	}
	return now.Sub(r.CreatedAt)
}

// CIRunQuery narrows a run listing.
type CIRunQuery struct {
	// Branch keeps only runs on this branch; empty means every branch.
	Branch string
	Limit  int
}

// CIActionKind is something the board did to a repository's CI.
type CIActionKind string

// The CI actions the board can take.
const (
	CIActionRerunRun CIActionKind = "rerun_run"
	CIActionRerunJob CIActionKind = "rerun_job"
	CIActionCancel   CIActionKind = "cancel_run"
)

// TimelineCI records a CI action taken through the board, data {action,
// run_id, job_id, head_sha, workflow}, with the reason as the body.
const TimelineCI TimelineType = "ci"

// pullPath matches the end of a pull request URL on GitHub (/pull/12) and
// Forgejo or Gitea (/pulls/12).
var pullPath = regexp.MustCompile(`/pulls?/([0-9]{1,9})$`)

// PullNumberOf returns the pull request number a PR URL names.
func PullNumberOf(prURL string) (int, bool) {
	parsed, err := url.Parse(strings.TrimSpace(prURL))
	if err != nil || prURL == "" {
		return 0, false
	}
	match := pullPath.FindStringSubmatch(strings.TrimRight(parsed.Path, "/"))
	if match == nil {
		return 0, false
	}
	number, err := strconv.Atoi(match[1])
	return number, err == nil && number > 0
}
