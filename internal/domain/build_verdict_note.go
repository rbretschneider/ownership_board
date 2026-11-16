package domain

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// The board's own build verdicts (build-host jobs) live beside the provider's
// CI. A manager that reads only the CI checks can see every CI job green while
// the ticket's own tests fail on the build host, and park the ticket on an
// unrelated CI problem (2026-11-16). These helpers put the newest board verdict
// and its failing tests next to every flag, and tell the dependency guard when
// a ticket's own code is failing.

// maxNamedTests bounds the failing tests named in one note.
const maxNamedTests = 5

var ctestFailedLine = regexp.MustCompile(`(?m)^\s*\d+\s+-\s+(.{1,200}?)\s+\((Failed|Timeout|Not Run|Exception|SEGFAULT|Subprocess aborted)\)`)

// FailingTests returns the failing test names a build job's logs list, first
// seen first, at most maxNamedTests.
func FailingTests(job BuildJob) []string {
	var result struct {
		Commands []struct {
			ExitCode int    `json:"exit_code"`
			Log      string `json:"log"`
		} `json:"commands"`
	}
	_ = json.Unmarshal(job.Result, &result)
	seen := map[string]bool{}
	var tests []string
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name != "" && !seen[name] && len(tests) < maxNamedTests {
			seen[name] = true
			tests = append(tests, name)
		}
	}
	for _, command := range result.Commands {
		if command.ExitCode == 0 {
			continue
		}
		for _, match := range firstFailingCase.FindAllStringSubmatch(command.Log, -1) {
			add(match[1])
		}
		for _, match := range ctestFailedLine.FindAllStringSubmatch(command.Log, -1) {
			add(match[1])
		}
	}
	return tests
}

// isSettled reports whether a job has a verdict: passed or failed.
func isSettled(job BuildJob) bool {
	return job.Status == "passed" || job.Status == "failed"
}

// NewestSettledBuild returns the newest job with a verdict. jobs are newest first.
func NewestSettledBuild(jobs []BuildJob) (BuildJob, bool) {
	for _, job := range jobs {
		if isSettled(job) {
			return job, true
		}
	}
	return BuildJob{}, false
}

// OwnBuildFailure reports whether the newest board verdict failed in the
// ticket's own code: a failing test or a compile error, not the host. It
// returns that job and a short description.
func OwnBuildFailure(jobs []BuildJob) (BuildJob, string, bool) {
	job, ok := NewestSettledBuild(jobs)
	if !ok || job.Status != "failed" {
		return BuildJob{}, "", false
	}
	if tests := FailingTests(job); len(tests) > 0 {
		return job, "failing tests: " + strings.Join(tests, "; "), true
	}
	signature := BuildFailureSignature(job)
	if strings.HasPrefix(signature, "test: ") || strings.HasPrefix(signature, "compile: ") {
		return job, signature, true
	}
	return job, "", false
}

// LatestBuildNote is one line on a ticket's newest board build for the
// manager: running, passed, or failed with what failed. It is "" when the
// ticket has no build job.
func LatestBuildNote(jobs []BuildJob) string {
	if len(jobs) == 0 {
		return ""
	}
	if newest := jobs[0]; newest.Status == "queued" || newest.Status == "running" {
		note := fmt.Sprintf("board %s build on %s is %s", newest.Target.OS, ShortSHA(newest.SHA), newest.Status)
		if previous, ok := NewestSettledBuild(jobs[1:]); ok {
			note += "; previous verdict: " + verdictText(previous)
		}
		return note
	}
	job, ok := NewestSettledBuild(jobs)
	if !ok {
		return ""
	}
	return "board verdict: " + verdictText(job)
}

func verdictText(job BuildJob) string {
	text := fmt.Sprintf("%s %s on %s", job.Target.OS, job.Status, ShortSHA(job.SHA))
	if job.Status != "failed" {
		return text
	}
	if tests := FailingTests(job); len(tests) > 0 {
		return text + " — the ticket's own failing tests: " + strings.Join(tests, "; ")
	}
	if signature := BuildFailureSignature(job); signature != "" {
		return text + " — " + clip(signature, 200)
	}
	return text
}

// WithBuildNote appends a build note to a flag's detail.
func WithBuildNote(detail, note string) string {
	if note == "" {
		return detail
	}
	return detail + " [" + note + "]"
}
