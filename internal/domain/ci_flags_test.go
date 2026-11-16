package domain

import (
	"strings"
	"testing"
	"time"
)

var ciNow = time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)

func ciRepo() Repo {
	return Repo{ID: 4, FullName: "acme/api", DefaultBranch: "main"}
}

func ciTicket() CITicketFacts {
	return CITicketFacts{Ticket: Ticket{ID: 64, Title: "brand scan", IsAutonomous: true}, Branch: "feat/scan", HeadSHA: headSHA}
}

func ciRun(id int64, workflow, branch, sha string, status CIStatus, conclusion CIConclusion) CIRun {
	return CIRun{ID: id, Workflow: workflow, Event: "push", HeadBranch: branch, HeadSHA: sha, Status: status,
		Conclusion: conclusion, CreatedAt: ciNow.Add(-5 * time.Minute)}
}

func flagsFor(facts CIRepoFacts, cancelled ...int64) []ManagerFlag {
	board := map[int64]bool{}
	for _, id := range cancelled {
		board[id] = true
	}
	return CIFlags(CISensorInput{Repos: []CIRepoFacts{facts}, BoardCancelled: board, QueueLimit: 30 * time.Minute, Now: ciNow})
}

func kinds(flags []ManagerFlag) []ManagerFlagKind {
	out := make([]ManagerFlagKind, 0, len(flags))
	for _, flag := range flags {
		out = append(out, flag.Kind)
	}
	return out
}

func TestAJobWaitingPastTheQueueLimitRaisesAFlagOnItsTicket(t *testing.T) {
	run := ciRun(10, "CI", "feat/scan", headSHA, CIQueued, "")
	run.Jobs = []CIJob{{ID: 100, RunID: 10, Name: "windows", Status: CIQueued, Labels: []string{"self-hosted"}, CreatedAt: ciNow.Add(-45 * time.Minute)}}
	flags := flagsFor(CIRepoFacts{Repo: ciRepo(), Runs: []CIRun{run}, Tickets: []CITicketFacts{ciTicket()}})
	if len(flags) != 1 || flags[0].Kind != FlagCIQueued || flags[0].TicketID != 64 || !strings.Contains(flags[0].Detail, "job 100") {
		t.Fatalf("flags = %+v, want one ci_queued flag on #64 naming job 100", flags)
	}
}

func TestAJobInsideTheQueueLimitIsQuiet(t *testing.T) {
	run := ciRun(10, "CI", "feat/scan", headSHA, CIQueued, "")
	run.Jobs = []CIJob{{ID: 100, RunID: 10, Status: CIQueued, CreatedAt: ciNow.Add(-10 * time.Minute)}}
	if flags := flagsFor(CIRepoFacts{Repo: ciRepo(), Runs: []CIRun{run}}); len(flags) != 0 {
		t.Fatalf("flags = %+v, want none", flags)
	}
}

func TestARunHeldWithNoJobsCountsAsQueued(t *testing.T) {
	run := ciRun(11, "CI", "main", headSHA, CIQueued, "")
	run.CreatedAt = ciNow.Add(-2 * time.Hour)
	flags := flagsFor(CIRepoFacts{Repo: ciRepo(), Runs: []CIRun{run}})
	if len(flags) != 1 || flags[0].Kind != FlagCIQueued || flags[0].TicketID != 0 || !strings.Contains(flags[0].Detail, "concurrency") {
		t.Fatalf("flags = %+v, want one repo-level ci_queued flag pointing at a concurrency hold", flags)
	}
}

func TestACancelledRunOnACurrentHeadRaisesAFlag(t *testing.T) {
	cancelled := ciRun(12, "CI", "feat/scan", headSHA, CICompleted, CICancelled)
	flags := flagsFor(CIRepoFacts{Repo: ciRepo(), Runs: []CIRun{cancelled}, Tickets: []CITicketFacts{ciTicket()}})
	if len(flags) != 1 || flags[0].Kind != FlagCICancelled || flags[0].TicketID != 64 {
		t.Fatalf("flags = %+v, want one ci_cancelled flag on #64", flags)
	}
}

func TestACancelledRunIsQuietWhenTheBoardCancelledItOrANewerRunReplacedIt(t *testing.T) {
	cancelled := ciRun(12, "CI", "feat/scan", headSHA, CICompleted, CICancelled)
	if flags := flagsFor(CIRepoFacts{Repo: ciRepo(), Runs: []CIRun{cancelled}, Tickets: []CITicketFacts{ciTicket()}}, 12); len(flags) != 0 {
		t.Fatalf("flags = %+v, want none for a run the board cancelled", flags)
	}
	rerun := ciRun(13, "CI", "feat/scan", headSHA, CIInProgress, "")
	if flags := flagsFor(CIRepoFacts{Repo: ciRepo(), Runs: []CIRun{rerun, cancelled}, Tickets: []CITicketFacts{ciTicket()}}); len(flags) != 0 {
		t.Fatalf("flags = %+v, want none once a newer run of the workflow covers the head", flags)
	}
	superseded := ciRun(14, "CI", "feat/scan", olderSHA, CICompleted, CICancelled)
	if flags := flagsFor(CIRepoFacts{Repo: ciRepo(), Runs: []CIRun{superseded}, Tickets: []CITicketFacts{ciTicket()}}); len(flags) != 0 {
		t.Fatalf("flags = %+v, want none for a cancelled run on an old head", flags)
	}
}

func TestAFailedDefaultBranchOrReleaseRunRaisesOneFlagPerWorkflow(t *testing.T) {
	runs := []CIRun{
		ciRun(22, "CI", "main", headSHA, CICompleted, CIFailure),
		ciRun(21, "CI", "main", olderSHA, CICompleted, CIFailure),
		ciRun(20, "Release dry run", "feat/scan", headSHA, CICompleted, CITimedOut),
		ciRun(19, "CI", "feat/scan", headSHA, CICompleted, CIFailure),
	}
	flags := flagsFor(CIRepoFacts{Repo: ciRepo(), Runs: runs})
	got := kinds(flags)
	if len(got) != 2 || got[0] != FlagCIFailed || got[1] != FlagCIFailed {
		t.Fatalf("flags = %+v, want one default-branch and one release ci_failed flag", flags)
	}
	if !strings.Contains(flags[0].Detail, "run 22") || !strings.Contains(flags[1].Detail, "run 20") {
		t.Fatalf("flags = %+v, want the newest run of each workflow", flags)
	}
}

func TestADefaultBranchThatStaysRedKeepsOneFingerprint(t *testing.T) {
	first := flagsFor(CIRepoFacts{Repo: ciRepo(), Runs: []CIRun{ciRun(21, "CI", "main", olderSHA, CICompleted, CIFailure)}})
	second := flagsFor(CIRepoFacts{Repo: ciRepo(), Runs: []CIRun{ciRun(22, "CI", "main", headSHA, CICompleted, CIFailure)}})
	if len(first) != 1 || len(second) != 1 || first[0].Fingerprint != second[0].Fingerprint {
		t.Fatalf("fingerprints %+v vs %+v, want one problem per red workflow, not one per push", first, second)
	}
}

func TestAGreenOrRunningDefaultBranchIsQuiet(t *testing.T) {
	runs := []CIRun{
		ciRun(23, "CI", "main", headSHA, CIInProgress, ""),
		ciRun(22, "CI", "main", olderSHA, CICompleted, CIFailure),
		ciRun(21, "Nightly", "main", olderSHA, CICompleted, CISuccess),
	}
	if flags := flagsFor(CIRepoFacts{Repo: ciRepo(), Runs: runs}); len(flags) != 0 {
		t.Fatalf("flags = %+v, want none while a newer run may fix it", flags)
	}
}

func TestAnOfflineRunnerRaisesAFlag(t *testing.T) {
	runners := []CIRunner{{Name: "win-1", IsOnline: false, Labels: []string{"windows"}}, {Name: "lin-1", IsOnline: true}}
	flags := flagsFor(CIRepoFacts{Repo: ciRepo(), Runners: runners})
	if len(flags) != 1 || flags[0].Kind != FlagRunnerOffline || !strings.Contains(flags[0].Detail, "win-1") {
		t.Fatalf("flags = %+v, want one runner_offline flag for win-1", flags)
	}
}

func TestABranchSharingMainsCommitRaisesOnlyItsOwnFlag(t *testing.T) {
	cancelled := ciRun(12, "CI", "feat/scan", headSHA, CICompleted, CICancelled)
	flags := flagsFor(CIRepoFacts{Repo: ciRepo(), DefaultHead: headSHA, Runs: []CIRun{cancelled}, Tickets: []CITicketFacts{ciTicket()}})
	if len(flags) != 1 || flags[0].TicketID != 64 {
		t.Fatalf("flags = %+v, want one flag on #64 and none on main", flags)
	}
}
