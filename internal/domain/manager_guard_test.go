package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const ctestFailureLog = "The following tests FAILED:\n" +
	"\t3202 - mc_thread_perf_tests:AC-14: eight tapped holes rebuild within budget (Failed) perf\n" +
	"Windows verification: FAILED (ctest failed with exit code 8)"

func windowsJob(status, sha, log string) BuildJob {
	job := BuildJob{Status: status, SHA: sha, Target: BuildJobTarget{OS: "windows"}}
	if log != "" {
		job.Result, _ = json.Marshal(map[string]any{"commands": []map[string]any{{"label": "verify", "exit_code": 1, "log": log}}})
	}
	return job
}

func TestFailingTestsReadsCTestSummaries(t *testing.T) {
	tests := FailingTests(windowsJob("failed", "", ctestFailureLog))
	if len(tests) != 1 || tests[0] != "mc_thread_perf_tests:AC-14: eight tapped holes rebuild within budget" {
		t.Fatalf("tests = %q", tests)
	}
}

func TestOwnBuildFailureTellsCodeFromHost(t *testing.T) {
	own := []BuildJob{windowsJob("failed", "dc367f204c56", ctestFailureLog)}
	if _, what, isOwn := OwnBuildFailure(own); !isOwn || !strings.Contains(what, "AC-14") {
		t.Fatalf("own failure = %v %q, want the failing test", isOwn, what)
	}
	host := windowsJob("failed", "abc", "")
	host.Result = json.RawMessage(`{"commands":[],"failure_kind":"infrastructure"}`)
	if _, _, isOwn := OwnBuildFailure([]BuildJob{host}); isOwn {
		t.Fatal("a host failure counted as the ticket's own")
	}
	// A newer pass wins over an older failure; a running job is skipped.
	fixed := []BuildJob{{Status: "running"}, windowsJob("passed", "new", ""), windowsJob("failed", "old", ctestFailureLog)}
	if _, _, isOwn := OwnBuildFailure(fixed); isOwn {
		t.Fatal("an older failure counted after a newer pass")
	}
}

func TestEveryManagerFlagCarriesTheBoardVerdictWithoutChangingItsFingerprint(t *testing.T) {
	now := time.Date(2026, 11, 16, 20, 0, 0, 0, time.UTC)
	ticket := Ticket{ID: 175, IsAutonomous: true, Stage: StageBuild, UpdatedAt: now.Add(-5 * time.Hour)}
	plain := ManagerFlags([]ManagerTicketFacts{{Ticket: ticket}}, DefaultBoardSettings(), now)
	noted := ManagerFlags([]ManagerTicketFacts{{Ticket: ticket,
		RecentJobs: []BuildJob{windowsJob("failed", "dc367f204c56229351702ac51d7dee1c67349daa", ctestFailureLog)}}}, DefaultBoardSettings(), now)
	if len(plain) == 0 || len(noted) == 0 {
		t.Fatalf("flags = %+v / %+v, want a stall flag", plain, noted)
	}
	if !strings.Contains(noted[0].Detail, "own failing tests: mc_thread_perf_tests:AC-14") {
		t.Fatalf("detail = %q, want the board verdict and failing test", noted[0].Detail)
	}
	if noted[0].Fingerprint != plain[0].Fingerprint {
		t.Fatalf("fingerprint changed: %q vs %q", noted[0].Fingerprint, plain[0].Fingerprint)
	}
}

func TestCIFlagsOnATicketCarryItsBoardVerdict(t *testing.T) {
	ticket := Ticket{ID: 175, Title: "Threading"}
	sensor := ciSensor{facts: CIRepoFacts{Repo: Repo{ID: 31, FullName: "acme/cad"},
		Tickets: []CITicketFacts{{Ticket: ticket, Branch: "ob/175", BuildNote: "board verdict: windows failed on dc367f20"}}}}
	flag := sensor.flag(FlagCICancelled, &ticket, "run:1", "run 1 cancelled")
	if !strings.HasSuffix(flag.Detail, "[board verdict: windows failed on dc367f20]") {
		t.Fatalf("detail = %q", flag.Detail)
	}
}

func TestManagerFilingsNameFilesAndDuplicatesAreFound(t *testing.T) {
	if _, err := NormalizeDeclaredFiles(nil); err == nil {
		t.Fatal("a manager filing without files was accepted")
	}
	if _, err := NormalizeDeclaredFiles([]string{"not a path"}); err == nil {
		t.Fatal("a non-path was accepted")
	}
	files, err := NormalizeDeclaredFiles([]string{"./.github/workflows/ci.yml", "tools/ci/test_check.py", ".github/workflows/ci.yml"})
	if err != nil || len(files) != 2 || files[0] != ".github/workflows/ci.yml" {
		t.Fatalf("files = %q %v", files, err)
	}
	open := Ticket{Spec: "Fix the audit group in `.github/workflows/ci.yml` and its policy test."}
	if shared := ManagerFilingOverlap(files, open); len(shared) != 1 || shared[0] != ".github/workflows/ci.yml" {
		t.Fatalf("shared = %q, want the CI workflow (design overlap ignores it; duplicates must not)", shared)
	}
	if !strings.Contains(WithDeclaredFiles("Spec.", files), "- `tools/ci/test_check.py`") {
		t.Fatal("declared files missing from the spec")
	}
}
