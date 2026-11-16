package domain

// The CI guards are the board's rules for touching CI, enforced server-side
// so no prompt can talk its way past them. On 2026-11-16 a manager with a raw
// provider token cancelled CI on a PR's current head and blamed a "stale
// head"; these rules make that impossible through the board.

// CheckCICancel allows cancelling run only when it is still going and a newer
// commit has replaced its head on the run's branch. branchHead is the
// branch's current head as the provider reports it right now; empty means
// the branch is gone or unknown, and the board then refuses rather than
// guess.
func CheckCICancel(run CIRun, branchHead string) error {
	if run.IsCompleted() {
		return Conflictf("run %d already completed; there is nothing to cancel", run.ID)
	}
	if branchHead == "" {
		return Conflictf("the board could not confirm a newer commit on %q than run %d's %s, so it will not cancel it",
			run.HeadBranch, run.ID, ShortSHA(run.HeadSHA))
	}
	if SameSHA(run.HeadSHA, branchHead) {
		return Conflictf("run %d is on %s, the current head of %q; the board never cancels CI on a current head. "+
			"Check why it is slow or queued instead (ci_status)", run.ID, ShortSHA(run.HeadSHA), run.HeadBranch)
	}
	return nil
}

// CheckCIRerun allows a re-run only once the run has completed, and a whole
// run re-run only when something in it failed or was cancelled: re-running
// green jobs spends CI time on an answer the board already has.
func CheckCIRerun(run CIRun) error {
	if !run.IsCompleted() {
		return Conflictf("run %d is still %s; re-run only after it completes", run.ID, run.Status)
	}
	if run.Conclusion == CISuccess || run.Conclusion == CISkipped {
		return Conflictf("run %d passed; there is no failed job to re-run", run.ID)
	}
	return nil
}

// CheckCIJobRerun allows re-running one job only once its run has completed.
func CheckCIJobRerun(run CIRun, job CIJob) error {
	if job.RunID != run.ID {
		return Invalidf("job %d is not part of run %d", job.ID, run.ID)
	}
	if !run.IsCompleted() {
		return Conflictf("run %d is still %s; re-run job %d only after the run completes", run.ID, run.Status, job.ID)
	}
	return nil
}

// ShortSHA shortens a commit hash for messages.
func ShortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// SameSHA compares two commit hashes, either of which may be abbreviated.
func SameSHA(a, b string) bool {
	if len(a) < MinSHAPrefixLength || len(b) < MinSHAPrefixLength {
		return false
	}
	n := min(len(a), len(b))
	return a[:n] == b[:n]
}
