package domain

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// The board manager keeps autonomous tickets moving at the lowest token cost.
// The board finds what needs its attention without any model: each flag below
// names a ticket and why, with a fingerprint that stays the same until the
// situation changes, so the manager handles each problem once.

// ManagerFlagKind says why a ticket needs the manager.
type ManagerFlagKind string

// The flag kinds, in the order the manager should handle them.
const (
	// FlagEscalated: the ticket is blocked, or waits on questions.
	FlagEscalated ManagerFlagKind = "escalated"
	// FlagCycling: the same failure came back, or repair or review rounds keep repeating.
	FlagCycling ManagerFlagKind = "cycling"
	// FlagBuildCap: more build passes than the board's cap.
	FlagBuildCap ManagerFlagKind = "build_cap"
	// FlagStalled: nothing has moved for longer than the stall limit.
	FlagStalled ManagerFlagKind = "stalled"
)

// repeatRounds is how many repair or review-fix rounds count as a cycle.
const repeatRounds = 3

// ManagerFlag is one reason one ticket needs the manager.
type ManagerFlag struct {
	TicketID    int64           `json:"ticket_id"`
	RepoID      int64           `json:"repo_id"`
	Title       string          `json:"title"`
	Stage       Stage           `json:"stage"`
	Kind        ManagerFlagKind `json:"kind"`
	Fingerprint string          `json:"fingerprint"`
	Detail      string          `json:"detail"`
}

// ManagerTicketFacts is what the board knows about one autonomous ticket.
type ManagerTicketFacts struct {
	Ticket        Ticket
	OpenQuestions int
	WaitingOn     []int64
	Attempts      StageAttempts
	// RecentJobs are the ticket's newest build jobs, newest first.
	RecentJobs []BuildJob
}

// ManagerUsage is the manager's spend on one UTC day.
type ManagerUsage struct {
	Day      string  `json:"day"`
	Sessions int     `json:"sessions"`
	CostUSD  float64 `json:"cost_usd"`
}

// ManagerFlags lists the flags of every open autonomous ticket in facts.
func ManagerFlags(facts []ManagerTicketFacts, settings BoardSettings, now time.Time) []ManagerFlag {
	var flags []ManagerFlag
	for _, fact := range facts {
		ticket := fact.Ticket
		if !ticket.IsAutonomous || ticket.Stage.IsCompleted() {
			continue
		}
		add := func(kind ManagerFlagKind, key, detail string) {
			flags = append(flags, ManagerFlag{TicketID: ticket.ID, RepoID: ticket.RepoID, Title: ticket.Title, Stage: ticket.Stage,
				Kind: kind, Fingerprint: fmt.Sprintf("%s:%d:%s", kind, ticket.ID, key), Detail: detail})
		}
		if ticket.BlockedReason != "" {
			add(FlagEscalated, "blocked:"+shortHash(ticket.BlockedReason), "blocked: "+clip(ticket.BlockedReason, 400))
		}
		if fact.OpenQuestions > 0 {
			add(FlagEscalated, fmt.Sprintf("questions:%d:%d", fact.OpenQuestions, ticket.UpdatedAt.Unix()),
				fmt.Sprintf("%d open question(s) waiting for an answer", fact.OpenQuestions))
		}
		if signature, jobs := repeatedFailure(fact.RecentJobs); signature != "" {
			add(FlagCycling, fmt.Sprintf("failure:%s:%d", shortHash(signature), jobs),
				fmt.Sprintf("the last %d build-host runs failed the same way: %s", jobs, signature))
		}
		if fact.Attempts.PRRepairs >= repeatRounds {
			add(FlagCycling, fmt.Sprintf("pr_repairs:%d", fact.Attempts.PRRepairs),
				fmt.Sprintf("the pull request went back for repair %d times", fact.Attempts.PRRepairs))
		}
		if fact.Attempts.ReviewFixes >= repeatRounds {
			add(FlagCycling, fmt.Sprintf("review_fixes:%d", fact.Attempts.ReviewFixes),
				fmt.Sprintf("review sent the work back for fixes %d times", fact.Attempts.ReviewFixes))
		}
		if cap := settings.ManagerBuildCap; cap > 0 && fact.Attempts.Build > cap {
			// One flag per pass over the cap: every further pass gets a fresh look.
			add(FlagBuildCap, fmt.Sprintf("build:%d", fact.Attempts.Build),
				fmt.Sprintf("%d build passes, over the cap of %d", fact.Attempts.Build, cap))
		}
		if stalled(fact, settings, now) {
			add(FlagStalled, fmt.Sprintf("since:%d", ticket.UpdatedAt.Unix()),
				fmt.Sprintf("no movement for %s (no lease, no build job running)", now.Sub(ticket.UpdatedAt).Round(time.Minute)))
		}
	}
	return flags
}

// stalled reports whether nothing works on an unblocked ticket and nothing has
// changed for longer than the stall limit.
func stalled(fact ManagerTicketFacts, settings BoardSettings, now time.Time) bool {
	ticket := fact.Ticket
	if ticket.BlockedReason != "" || fact.OpenQuestions > 0 || len(fact.WaitingOn) > 0 || ticket.IsLeased(now) {
		return false
	}
	for _, job := range fact.RecentJobs {
		if job.Status == "queued" || job.Status == "running" {
			return false
		}
	}
	limit := time.Duration(settings.ManagerStallMinutes) * time.Minute
	return limit > 0 && now.Sub(ticket.UpdatedAt) > limit
}

// repeatedFailure returns the shared signature of the newest consecutive failed
// jobs when at least two failed the same way, and how many did.
func repeatedFailure(jobs []BuildJob) (string, int) {
	var first string
	count := 0
	for _, job := range jobs {
		if job.Status == "queued" || job.Status == "running" || job.Status == "superseded" || job.Status == "cancelled" {
			continue
		}
		if job.Status != "failed" {
			break
		}
		signature := BuildFailureSignature(job)
		if count == 0 {
			first = signature
		} else if signature != first {
			break
		}
		count++
	}
	if count >= 2 && first != "" {
		return first, count
	}
	return "", 0
}

var (
	firstFailingCase = regexp.MustCompile(`first failing case: \S+: (.{1,200}?): (Failed|Timeout|Not Run|Exception)`)
	verifyFailed     = regexp.MustCompile(`Windows verification: FAILED \(([^)\n]{1,160})`)
	compilerError    = regexp.MustCompile(`error C\d{4}: [^\n]{1,120}`)
)

// BuildFailureSignature names a failed build job's failure: the first failing
// test when the log names one, a compiler error, the verification summary,
// or the job's own detail line.
func BuildFailureSignature(job BuildJob) string {
	var result struct {
		Commands []struct {
			Label    string `json:"label"`
			ExitCode int    `json:"exit_code"`
			Log      string `json:"log"`
		} `json:"commands"`
		FailureKind string `json:"failure_kind"`
	}
	_ = json.Unmarshal(job.Result, &result)
	if result.FailureKind != "" {
		return "host: " + result.FailureKind
	}
	for _, command := range result.Commands {
		if command.ExitCode == 0 {
			continue
		}
		if match := firstFailingCase.FindStringSubmatch(command.Log); match != nil {
			return "test: " + match[1] + " (" + match[2] + ")"
		}
		if match := compilerError.FindString(command.Log); match != "" {
			return "compile: " + match
		}
		if match := verifyFailed.FindStringSubmatch(command.Log); match != nil {
			return "verify: " + match[1]
		}
		return "command: " + command.Label
	}
	return clip(strings.TrimSpace(job.Detail), 160)
}

func clip(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "…"
}

// shortHash is a stable, short key for free text in a fingerprint.
func shortHash(text string) string {
	var h uint32 = 2166136261
	for i := 0; i < len(text); i++ {
		h ^= uint32(text[i])
		h *= 16777619
	}
	return fmt.Sprintf("%08x", h)
}

// MaxManagerSummaryLength bounds one session summary kept for the owner.
const MaxManagerSummaryLength = 8000

// ManagerSession is one board manager model session: when, on which model,
// what it cost, and its own summary of why it acted and what it did.
type ManagerSession struct {
	At      time.Time `json:"at"`
	Model   string    `json:"model"`
	CostUSD float64   `json:"cost_usd"`
	Summary string    `json:"summary"`
}
