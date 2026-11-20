package reposource

import (
	"strings"
	"testing"
)

// The shape of makercad's 2026-11-20 nightly: a test fails, thousands of
// passing lines follow, and the log ends in artifact-upload errors. The
// failed test and its assertion must reach the reader, not only the tail.
func TestTheFailedTestFromEarlierInTheLogReachesTheReader(t *testing.T) {
	var log strings.Builder
	stamp := "2026-11-20T14:07:43.6723086Z "
	log.WriteString(stamp + "-- Performing Test CMAKE_HAVE_LIBC_PTHREAD - Failed\n")
	log.WriteString(strings.Repeat(stamp+"3620/3636 Test #3620: mc_solver_perf_tests:budget ....   Passed    0.03 sec\n", 300))
	log.WriteString(stamp + "Start 3622: mc_fillet_corpus_tests:F-09 p95\n")
	log.WriteString(stamp + "3622/3636 Test #3622: mc_fillet_corpus_tests:F-09 p95 ....***Failed    1.31 sec\n")
	log.WriteString(stamp + "TimedLoop.cpp(80): FAILED:\n")
	log.WriteString(stamp + "  REQUIRE( verdict.isWithin )\n")
	log.WriteString(stamp + "with messages:\n")
	log.WriteString(stamp + "  p95 42.2686 ms, max 48.6153 ms, limit 30 ms\n")
	log.WriteString(strings.Repeat(stamp+"3630/3636 Test #3630: mc_other:case ....   Passed    0.01 sec\n", 300))
	log.WriteString(strings.Repeat(stamp+"never-generated build/win-debug/queue-msbuild-errors.log\n", 200))
	log.WriteString(stamp + "##[error]Failed to CreateArtifact: Artifact storage quota has been hit.\n")

	text, err := readFailuresAndTail(strings.NewReader(log.String()), 6000)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(text) > 6000 {
		t.Fatalf("answer is %d bytes, want at most 6000", len(text))
	}
	for _, want := range []string{"Test #3622: mc_fillet_corpus_tests:F-09 p95 ....***Failed", "REQUIRE( verdict.isWithin )", "limit 30 ms", "Artifact storage quota has been hit"} {
		if !strings.Contains(text, want) {
			t.Fatalf("answer lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "2026-11-20T") {
		t.Fatalf("answer kept timestamps:\n%s", text)
	}
	if !strings.HasPrefix(text, failuresHeader) || !strings.Contains(text, tailHeader) {
		t.Fatalf("answer does not separate failures from the tail:\n%s", text)
	}
}

func TestALogWhoseFailureIsInTheTailHasNoFailureSection(t *testing.T) {
	log := strings.Repeat("noise line\n", 500) + "error: verify failed\n"
	text, err := readFailuresAndTail(strings.NewReader(log), 200)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(text, failuresHeader) || !strings.HasSuffix(text, "error: verify failed") {
		t.Fatalf("answer = %q, want only the tail", text)
	}
}

func TestFailureLinesKeepTheLatestWithinTheBudget(t *testing.T) {
	var log strings.Builder
	for i := 0; i < 200; i++ {
		log.WriteString("case " + strings.Repeat("x", 40) + "\n")
		log.WriteString("error: early failure\n")
		log.WriteString(strings.Repeat("filler\n", 10))
	}
	log.WriteString("error: the last real failure\n")
	log.WriteString(strings.Repeat("cleanup line that is long enough to push things out\n", 200))
	text, err := readFailuresAndTail(strings.NewReader(log.String()), 2000)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(text, "error: the last real failure") || len(text) > 2000 {
		t.Fatalf("answer (%d bytes) lacks the latest failure:\n%s", len(text), text)
	}
}
