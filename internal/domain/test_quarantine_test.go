package domain

import (
	"encoding/json"
	"slices"
	"testing"
)

func resultWithLog(log string) []byte {
	raw, _ := json.Marshal(BuildJobResult{Commands: []BuildJobCommand{{Label: "verify", ExitCode: 8, Log: log}}})
	return raw
}

func TestFailedTestsReadsEveryKnownFormat(t *testing.T) {
	log := "\x1b[31;1mThe following tests FAILED:\x1b[0m\n" +
		"\t 12 - mc_ui_tests:Start screen fits (Failed)\n" +
		"\t 40 - mc_gl:Native Fillet hovers (Timeout)\n" +
		"Errors while running CTest\n" +
		"--- CTest failed tests: win-release\n" +
		"3328:mc_ui_tests:Start screen fits\n" +
		"3496:mc_diag:Document composition binds\n" +
		"CTest failure invocation: run=1\n" +
		"--- first failing case: win-debug/queue-gl.xml: mc_composition_gl_tests:Hover click picks meet 50ms: Failed \n" +
		"--- first failing case: policy:qt_text_policy: Failed\n" +
		"ownership-test-failed: custom suite/case\n"
	got := FailedTests(resultWithLog(log))
	want := []string{"mc_ui_tests:Start screen fits", "mc_gl:Native Fillet hovers", "mc_diag:Document composition binds",
		"mc_composition_gl_tests:Hover click picks meet 50ms", "policy:qt_text_policy", "custom suite/case"}
	if !slices.Equal(got, want) {
		t.Fatalf("FailedTests = %q\nwant %q", got, want)
	}
}

func TestFailedTestsIgnoresPassingSummariesAndJunk(t *testing.T) {
	log := "100% tests passed, 0 tests failed out of 1\n1/1 Test #7: mc_ui:Timeline ... Passed 0.03 sec\n"
	if got := FailedTests(resultWithLog(log)); len(got) != 0 {
		t.Fatalf("FailedTests = %q, want none", got)
	}
	if FailedTests([]byte("not json")) != nil {
		t.Fatal("garbage result")
	}
}

func TestValidTestName(t *testing.T) {
	for name, want := range map[string]bool{"a:b c": true, "": false, " a": false, "a\nb": false, string(make([]byte, 301)): false} {
		if ValidTestName(name) != want {
			t.Errorf("ValidTestName(%q) != %v", name, want)
		}
	}
}
