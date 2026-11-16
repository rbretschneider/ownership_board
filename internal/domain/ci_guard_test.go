package domain

import "testing"

const (
	headSHA  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	olderSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func runOn(sha string, status CIStatus, conclusion CIConclusion) CIRun {
	return CIRun{ID: 7, Workflow: "CI", HeadBranch: "feat/x", HeadSHA: sha, Status: status, Conclusion: conclusion}
}

func TestCancellingARunOnTheCurrentHeadIsRefused(t *testing.T) {
	err := CheckCICancel(runOn(headSHA, CIInProgress, ""), headSHA[:12])
	if KindOf(err) != KindConflict {
		t.Fatalf("err = %v, want a conflict: the board never cancels CI on a current head", err)
	}
}

func TestCancellingARunReplacedByANewerHeadIsAllowed(t *testing.T) {
	if err := CheckCICancel(runOn(olderSHA, CIQueued, ""), headSHA); err != nil {
		t.Fatalf("err = %v, want the obsolete run cancellable", err)
	}
}

func TestCancellingIsRefusedWhenTheBranchHeadIsUnknown(t *testing.T) {
	if err := CheckCICancel(runOn(olderSHA, CIQueued, ""), ""); KindOf(err) != KindConflict {
		t.Fatalf("err = %v, want a conflict when no newer head can be confirmed", err)
	}
}

func TestCancellingACompletedRunIsRefused(t *testing.T) {
	if err := CheckCICancel(runOn(olderSHA, CICompleted, CIFailure), headSHA); KindOf(err) != KindConflict {
		t.Fatalf("err = %v, want a conflict for a finished run", err)
	}
}

func TestARunIsReRunOnlyAfterItCompletesWithSomethingToRedo(t *testing.T) {
	cases := []struct {
		name    string
		run     CIRun
		allowed bool
	}{
		{"still running", runOn(headSHA, CIInProgress, ""), false},
		{"still queued", runOn(headSHA, CIQueued, ""), false},
		{"passed", runOn(headSHA, CICompleted, CISuccess), false},
		{"failed", runOn(headSHA, CICompleted, CIFailure), true},
		{"cancelled", runOn(headSHA, CICompleted, CICancelled), true},
		{"timed out", runOn(headSHA, CICompleted, CITimedOut), true},
	}
	for _, c := range cases {
		err := CheckCIRerun(c.run)
		if (err == nil) != c.allowed {
			t.Errorf("%s: err = %v, allowed want %v", c.name, err, c.allowed)
		}
	}
}

func TestAJobIsReRunOnlyAfterItsRunCompletes(t *testing.T) {
	job := CIJob{ID: 3, RunID: 7}
	if err := CheckCIJobRerun(runOn(headSHA, CIInProgress, ""), job); KindOf(err) != KindConflict {
		t.Fatalf("err = %v, want a conflict while the run is going", err)
	}
	if err := CheckCIJobRerun(runOn(headSHA, CICompleted, CIFailure), job); err != nil {
		t.Fatalf("err = %v, want the job re-runnable", err)
	}
	if err := CheckCIJobRerun(runOn(headSHA, CICompleted, CIFailure), CIJob{ID: 3, RunID: 8}); KindOf(err) != KindInvalid {
		t.Fatalf("err = %v, want a job from another run refused", err)
	}
}

func TestShortHashesMatchTheirFullCommit(t *testing.T) {
	if !SameSHA(headSHA[:7], headSHA) || SameSHA(headSHA, olderSHA) || SameSHA("", "") || SameSHA("aaa", headSHA) {
		t.Fatal("SameSHA must match prefixes of at least 7 characters only")
	}
}
