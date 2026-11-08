package domain

import (
	"regexp"
	"strings"
)

// StageAttempts counts how often a ticket went through each delivery step,
// derived only from its recorded history: stage moves, the work-model
// signals workers report, and the comment a worker posts when it sends a
// commit to Windows verification. Zero means the step never started.
type StageAttempts struct {
	// Build counts passes at the code: each entry into the build stage plus
	// each return to the code after a failed Windows run or a review that
	// requested fixes. An entry from review is a CI or PR repair.
	Build int
	// Test counts Windows verification runs, one per commit sent to Windows.
	Test int
	// TestFailures counts commits whose Windows verification failed.
	TestFailures int
	// Review counts fresh-context adversarial review rounds.
	Review int
	// ReviewFixes counts review rounds that sent the work back for fixes.
	ReviewFixes int
	// PR counts entries into the PR stage with a pull request.
	PR int
	// PRRepairs counts returns from an open PR to build (failed check,
	// conflict or requested change).
	PRRepairs int
}

// windowsQueued matches the comment a worker posts when it sends a commit to
// Windows verification. Sending the same commit again is the same run.
var windowsQueued = regexp.MustCompile(`^Windows verification queued for commit ([0-9a-f]{7,40})\b`)

// workSignal is the part of a work model after the model name, such as
// "adversarial review" or "build-server:queued:Windows x64 · awaiting Windows".
func workSignal(model string) string {
	_, signal, _ := strings.Cut(model, " · ")
	return signal
}

// CountStageAttempts derives step attempts from a ticket's timeline, which
// must be in recorded order. Entries of other types are ignored.
//
// A Windows run is identified by the commit in the worker's "Windows
// verification queued for commit <sha>" comment, so a worker re-reading the
// verdict of an unchanged commit does not count again. A failed verdict is a
// "· awaiting repair" report while that commit is the latest one sent.
func CountStageAttempts(entries []TimelineEntry) StageAttempts {
	var a StageAttempts
	isReviewing := false
	commit := ""
	tested, failed := map[string]bool{}, map[string]bool{}
	for _, entry := range entries {
		switch entry.Type {
		case TimelineStage:
			from, _ := entry.Data["from"].(string)
			to, _ := entry.Data["to"].(string)
			// A stage change clears the work model, ending any review round.
			isReviewing = false
			switch Stage(to) {
			case StageBuild:
				a.Build++
				if Stage(from) == StagePR {
					a.PRRepairs++
				}
			case StagePR:
				a.PR++
			}
		case TimelineComment:
			if !entry.Actor.IsAgent() {
				continue
			}
			if match := windowsQueued.FindStringSubmatch(entry.Body); match != nil {
				commit = match[1]
				if !tested[commit] {
					tested[commit] = true
					a.Test++
				}
			}
		case TimelineEdited:
			model, hasModel := entry.Data["model"].(string)
			if !hasModel {
				continue
			}
			signal := workSignal(model)
			isReview := strings.Contains(signal, "adversarial review")
			if isReviewing && !isReview {
				// The author picked the work up again after a review round
				// that did not publish: the review requested fixes.
				a.ReviewFixes++
				a.Build++
			}
			if isReview && !isReviewing {
				a.Review++
			}
			isReviewing = isReview
			if signal == "awaiting repair" && commit != "" && !failed[commit] {
				failed[commit] = true
				a.TestFailures++
				a.Build++
			}
		}
	}
	return a
}
