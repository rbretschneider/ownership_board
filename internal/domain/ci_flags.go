package domain

import (
	"fmt"
	"time"
)

// CI sensors: what in a repository's CI needs the board manager. Like the
// ticket flags they run no model; each fingerprint stays the same until the
// situation changes, so the manager looks at each problem once.

// The CI flag kinds.
const (
	// FlagCIQueued: a CI job (or a run with no jobs yet) waited for a runner
	// longer than the queue limit.
	FlagCIQueued ManagerFlagKind = "ci_queued"
	// FlagCICancelled: the newest run of a workflow on a current head was
	// cancelled, and not through the board: a concurrency group evicted it,
	// or a person cancelled it on the provider. The head has no CI result.
	FlagCICancelled ManagerFlagKind = "ci_cancelled"
	// FlagCIFailed: the newest run of a workflow on the default branch, or of
	// a release workflow, failed.
	FlagCIFailed ManagerFlagKind = "ci_failed"
	// FlagRunnerOffline: a CI runner that serves the repository is offline.
	FlagRunnerOffline ManagerFlagKind = "runner_offline"
)

// DefaultCIQueueLimit is how long a CI job may wait for a runner before the
// manager hears about it.
const DefaultCIQueueLimit = 30 * time.Minute

// CITicketFacts is an open autonomous ticket's place in its repository's CI.
type CITicketFacts struct {
	Ticket Ticket
	// Branch is the ticket's head branch (its PR's, or the one recorded).
	Branch string
	// HeadSHA is that branch's current head, or "" when unknown.
	HeadSHA string
}

// CIRepoFacts is what the board read from one repository's CI.
type CIRepoFacts struct {
	Repo Repo
	// DefaultHead is the default branch's current head, or "" when unknown.
	DefaultHead string
	// Runs are recent runs, newest first. Runs that have not completed carry
	// their jobs.
	Runs    []CIRun
	Runners []CIRunner
	Tickets []CITicketFacts
}

// CISensorInput is everything the CI sensors look at.
type CISensorInput struct {
	Repos []CIRepoFacts
	// BoardCancelled holds the runs cancelled through the board (ci_cancel),
	// which are never a surprise.
	BoardCancelled map[int64]bool
	QueueLimit     time.Duration
	Now            time.Time
}

// CIFlags lists what in the repositories' CI needs the board manager.
func CIFlags(input CISensorInput) []ManagerFlag {
	limit := input.QueueLimit
	if limit <= 0 {
		limit = DefaultCIQueueLimit
	}
	var flags []ManagerFlag
	for _, facts := range input.Repos {
		sensor := ciSensor{facts: facts, cancelled: input.BoardCancelled, limit: limit, now: input.Now}
		flags = append(flags, sensor.queued()...)
		flags = append(flags, sensor.cancelledOnHeads()...)
		flags = append(flags, sensor.failed()...)
		flags = append(flags, sensor.offlineRunners()...)
	}
	return flags
}

// ciSensor checks one repository.
type ciSensor struct {
	facts     CIRepoFacts
	cancelled map[int64]bool
	limit     time.Duration
	now       time.Time
}

func (s ciSensor) queued() []ManagerFlag {
	var flags []ManagerFlag
	for _, run := range s.facts.Runs {
		waited, what := s.longestWait(run)
		if waited <= s.limit {
			continue
		}
		detail := fmt.Sprintf("%s in run %d (%s on %q, %s) has waited %s for a runner, over the %s limit",
			what, run.ID, run.Workflow, run.HeadBranch, ShortSHA(run.HeadSHA), waited.Round(time.Minute), s.limit)
		flags = append(flags, s.flag(FlagCIQueued, s.ticketOn(run.HeadBranch), fmt.Sprintf("run:%d", run.ID), detail))
	}
	return flags
}

// longestWait is the longest any part of run has waited for a runner, and
// what waited.
func (s ciSensor) longestWait(run CIRun) (time.Duration, string) {
	if waited := run.QueuedFor(s.now); waited > 0 {
		return waited, "the run (no job started; often a concurrency group holding it)"
	}
	var longest time.Duration
	what := ""
	for _, job := range run.Jobs {
		if waited := job.QueuedFor(s.now); waited > longest {
			longest, what = waited, fmt.Sprintf("job %d %q (labels %v)", job.ID, job.Name, job.Labels)
		}
	}
	return longest, what
}

func (s ciSensor) cancelledOnHeads() []ManagerFlag {
	var flags []ManagerFlag
	for _, head := range s.heads() {
		for _, run := range newestPerWorkflow(s.facts.Runs, head.covers) {
			if run.Conclusion != CICancelled || s.cancelled[run.ID] {
				continue
			}
			detail := fmt.Sprintf("run %d (%s) on %s, the current head of %q, was cancelled outside the board "+
				"(a concurrency group evicting it, or a person); that head has no CI result", run.ID, run.Workflow, ShortSHA(run.HeadSHA), head.branch)
			flags = append(flags, s.flag(FlagCICancelled, head.ticket, fmt.Sprintf("run:%d", run.ID), detail))
		}
	}
	return flags
}

func (s ciSensor) failed() []ManagerFlag {
	var flags []ManagerFlag
	defaultBranch := s.facts.Repo.DefaultBranch
	categories := []struct {
		name    string
		matches func(CIRun) bool
	}{
		{"default", func(run CIRun) bool {
			return defaultBranch != "" && run.HeadBranch == defaultBranch && run.Event != "pull_request" && !run.IsRelease()
		}},
		{"release", CIRun.IsRelease},
	}
	for _, category := range categories {
		for _, run := range newestPerWorkflow(s.facts.Runs, category.matches) {
			if !run.IsCompleted() || !run.Conclusion.IsFailure() {
				continue
			}
			detail := fmt.Sprintf("the newest %s run of %s, run %d on %q at %s, ended %s", category.name, run.Workflow,
				run.ID, run.HeadBranch, ShortSHA(run.HeadSHA), run.Conclusion)
			// Keyed by workflow, not run: a branch that stays red is one problem, not one per push.
			flags = append(flags, s.flag(FlagCIFailed, nil, fmt.Sprintf("%s:%s", category.name, shortHash(run.Workflow)), detail))
		}
	}
	return flags
}

func (s ciSensor) offlineRunners() []ManagerFlag {
	var flags []ManagerFlag
	for _, runner := range s.facts.Runners {
		if runner.IsOnline {
			continue
		}
		detail := fmt.Sprintf("CI runner %q (labels %v) is offline; jobs that need it will queue", runner.Name, runner.Labels)
		flags = append(flags, s.flag(FlagRunnerOffline, nil, "runner:"+shortHash(runner.Name), detail))
	}
	return flags
}

// ciHead is a branch head the board cares about.
type ciHead struct {
	branch string
	sha    string
	ticket *Ticket
}

// covers reports whether run ran on this head: same branch, same commit. A
// fresh branch shares its commit with main, and must not raise main's flags.
func (h ciHead) covers(run CIRun) bool {
	return run.HeadBranch == h.branch && SameSHA(run.HeadSHA, h.sha)
}

func (s ciSensor) heads() []ciHead {
	var heads []ciHead
	if s.facts.DefaultHead != "" {
		heads = append(heads, ciHead{branch: s.facts.Repo.DefaultBranch, sha: s.facts.DefaultHead})
	}
	for i := range s.facts.Tickets {
		ticket := s.facts.Tickets[i]
		if ticket.HeadSHA != "" {
			heads = append(heads, ciHead{branch: ticket.Branch, sha: ticket.HeadSHA, ticket: &s.facts.Tickets[i].Ticket})
		}
	}
	return heads
}

func (s ciSensor) ticketOn(branch string) *Ticket {
	for i := range s.facts.Tickets {
		if branch != "" && s.facts.Tickets[i].Branch == branch {
			return &s.facts.Tickets[i].Ticket
		}
	}
	return nil
}

// flag builds a CI flag, on the ticket when there is one, else on the repo.
func (s ciSensor) flag(kind ManagerFlagKind, ticket *Ticket, key, detail string) ManagerFlag {
	repo := s.facts.Repo
	flag := ManagerFlag{RepoID: repo.ID, Title: repo.FullName, Kind: kind, Detail: detail,
		Fingerprint: fmt.Sprintf("%s:r%d:%s", kind, repo.ID, key)}
	if ticket != nil {
		flag.TicketID, flag.Title, flag.Stage = ticket.ID, ticket.Title, ticket.Stage
	}
	return flag
}

// newestPerWorkflow returns, for each workflow, the newest run that matches,
// in the order the workflows first appear. runs are newest first.
func newestPerWorkflow(runs []CIRun, matches func(CIRun) bool) []CIRun {
	seen := map[string]bool{}
	var newest []CIRun
	for _, run := range runs {
		if seen[run.Workflow] || !matches(run) {
			continue
		}
		seen[run.Workflow] = true
		newest = append(newest, run)
	}
	return newest
}
