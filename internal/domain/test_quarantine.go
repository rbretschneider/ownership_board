package domain

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// Flaky-test quarantine (docs/amendments/flaky-quarantine.md). The board finds
// tests that fail on unrelated tickets and then pass on the same commit, keeps
// a per-repo list of them, and sends the list with every build job. A repo's
// verification script may skip the listed tests; one that ignores the list
// behaves exactly as before.

// Quarantine limits.
const (
	MaxTestNameBytes     = 300
	MaxQuarantinedTests  = 200
	MaxFailedTestsPerRun = 50
	// QuarantineReleasePasses is how many passing runs a test needs, after its
	// fix ticket merges, to leave the list.
	QuarantineReleasePasses = 3
	// FlakeCheckWindow is how far back another ticket's failure of the same
	// test makes a new failure suspect enough for a flake check.
	FlakeCheckWindow = 14 * 24 * time.Hour
	// FlakeCheckAgentVersion is the first build-host agent that runs only the
	// listed tests of a flake check, and writes the quarantine file.
	FlakeCheckAgentVersion = "1.2"
)

// Quarantine statuses. A probation test's fix merged: it runs again and leaves
// the list after QuarantineReleasePasses passing runs, or returns to active on
// its next failure.
const (
	QuarantineActive    = "active"
	QuarantineProbation = "probation"
	QuarantineReleased  = "released"
)

// Quarantine sources.
const (
	QuarantineDetected = "detected"
	QuarantineSeeded   = "seeded"
)

// Build job kinds. A flake check re-runs only some tests of a commit that
// already has its verdict; it never gives a verdict itself.
const (
	JobKindCommit     = "commit"
	JobKindFlakeCheck = "flake_check"
)

// QuarantinedTest is one test on a repo's quarantine list.
type QuarantinedTest struct {
	RepoID      int64           `json:"repo_id"`
	Test        string          `json:"test"`
	Status      string          `json:"status"`
	Source      string          `json:"source"`
	AddedBy     string          `json:"added_by"`
	Reason      string          `json:"reason"`
	Evidence    json.RawMessage `json:"evidence"`
	FixTicketID *int64          `json:"fix_ticket_id"`
	Passes      int             `json:"passes"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	ReleasedAt  *time.Time      `json:"released_at"`
}

// QuarantineEvidence is why a test is on the list: the failures that made it
// suspect and the run that passed it on a failing commit.
type QuarantineEvidence struct {
	Tickets  []int64 `json:"tickets"`
	Failures int     `json:"failures"`
	PassedOn string  `json:"passed_on,omitempty"`
	PassRun  string  `json:"pass_run,omitempty"`
}

// TestFailure is one test that failed in one run on one ticket's commit.
type TestFailure struct {
	RunID      string
	Repo       string
	TicketID   int64
	SHA        string
	Test       string
	RecordedAt time.Time
}

// ValidTestName reports whether name can be a quarantined test: one line,
// UTF-8, 1 to MaxTestNameBytes bytes.
func ValidTestName(name string) bool {
	return name != "" && len(name) <= MaxTestNameBytes && utf8.ValidString(name) &&
		!strings.ContainsAny(name, "\r\n\x00") && strings.TrimSpace(name) == name
}

var (
	ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	// CTest's summary: "The following tests FAILED:" then "  12 - name (Failed)".
	ctestFailed = regexp.MustCompile(`^\s*\d+\s+-\s+(.+?)\s+\([^()]*\)\s*$`)
	// A numbered list under a "--- CTest failed tests:" heading: "12:name".
	numberedFailed = regexp.MustCompile(`^\s*\d+:(\S.*)$`)
	// "--- first failing case: [report.xml: ]name: Failed".
	firstFailing = regexp.MustCompile(`first failing case: (?:\S+\.xml: )?(.+?): (?:Failed|Timeout|Exception|SEGFAULT|Not Run|Subprocess aborted)\s*$`)
	// The board's own convention, for scripts that report names directly.
	ownershipFailed = regexp.MustCompile(`^ownership-test-failed: (.+)$`)
)

// FailedTests reads the names of failed tests from a build result's command
// logs. It understands CTest's summary, a numbered list under a "CTest failed
// tests" heading, "first failing case:" lines and "ownership-test-failed: name"
// lines. Names are unique, in order of appearance, at most MaxFailedTestsPerRun.
func FailedTests(result []byte) []string {
	var r BuildJobResult
	if json.Unmarshal(result, &r) != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		name = strings.TrimSpace(name)
		if !ValidTestName(name) || seen[name] || len(out) >= MaxFailedTestsPerRun {
			return
		}
		seen[name] = true
		out = append(out, name)
	}
	for _, c := range r.Commands {
		mode := ""
		for _, line := range strings.Split(ansiEscape.ReplaceAllString(c.Log, ""), "\n") {
			line = strings.TrimRight(line, "\r")
			if m := firstFailing.FindStringSubmatch(line); m != nil {
				add(m[1])
			}
			if m := ownershipFailed.FindStringSubmatch(line); m != nil {
				add(m[1])
			}
			switch {
			case strings.Contains(line, "The following tests FAILED:"):
				mode = "ctest"
				continue
			case strings.HasPrefix(line, "--- CTest failed tests"):
				mode = "numbered"
				continue
			}
			switch mode {
			case "ctest":
				if m := ctestFailed.FindStringSubmatch(line); m != nil {
					add(m[1])
				} else if strings.TrimSpace(line) != "" {
					mode = ""
				}
			case "numbered":
				if m := numberedFailed.FindStringSubmatch(line); m != nil {
					add(m[1])
				} else {
					mode = ""
				}
			}
		}
	}
	return out
}
