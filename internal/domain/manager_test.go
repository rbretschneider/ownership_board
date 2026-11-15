package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func failedJob(log string) BuildJob {
	result, _ := json.Marshal(map[string]any{"commands": []map[string]any{{"label": "verify script", "exit_code": 1, "log": log}}})
	return BuildJob{Status: "failed", Result: result}
}

func flagKinds(flags []ManagerFlag) map[ManagerFlagKind]int {
	kinds := map[ManagerFlagKind]int{}
	for _, flag := range flags {
		kinds[flag.Kind]++
	}
	return kinds
}

func TestManagerFlagsCycleBuildCapAndStall(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	settings := DefaultBoardSettings()
	sameFailure := "--- first failing case: win-debug/q.xml: mc_ui_tests:Project links: Failed"
	facts := []ManagerTicketFacts{
		{
			Ticket:     Ticket{ID: 1, IsAutonomous: true, Stage: StageBuild, UpdatedAt: now.Add(-time.Hour)},
			Attempts:   StageAttempts{Build: 11, PRRepairs: 3},
			RecentJobs: []BuildJob{failedJob(sameFailure), failedJob(sameFailure), {Status: "passed"}},
		},
		{
			// Stalled: no lease, no job, untouched for 3 h.
			Ticket: Ticket{ID: 2, IsAutonomous: true, Stage: StageBuild, UpdatedAt: now.Add(-3 * time.Hour)},
		},
		{
			// Not autonomous: never the manager's business.
			Ticket:   Ticket{ID: 3, Stage: StageBuild, UpdatedAt: now.Add(-5 * time.Hour)},
			Attempts: StageAttempts{Build: 40},
		},
		{
			// A queued job means it is moving, not stalled.
			Ticket:     Ticket{ID: 4, IsAutonomous: true, Stage: StageBuild, UpdatedAt: now.Add(-5 * time.Hour)},
			RecentJobs: []BuildJob{{Status: "queued"}},
		},
	}
	flags := ManagerFlags(facts, settings, now)
	byTicket := map[int64][]ManagerFlag{}
	for _, flag := range flags {
		byTicket[flag.TicketID] = append(byTicket[flag.TicketID], flag)
	}
	if kinds := flagKinds(byTicket[1]); kinds[FlagCycling] != 2 || kinds[FlagBuildCap] != 1 || kinds[FlagStalled] != 0 {
		t.Errorf("#1 flags = %+v, want 2 cycling (same failure, PR repairs) and the build cap", byTicket[1])
	}
	if kinds := flagKinds(byTicket[2]); kinds[FlagStalled] != 1 || len(byTicket[2]) != 1 {
		t.Errorf("#2 flags = %+v, want stalled only", byTicket[2])
	}
	if len(byTicket[3]) != 0 || len(byTicket[4]) != 0 {
		t.Errorf("#3/#4 flags = %+v %+v, want none", byTicket[3], byTicket[4])
	}
	again := ManagerFlags(facts, settings, now.Add(time.Minute))
	for i := range flags {
		if flags[i].Kind != FlagStalled && flags[i].Fingerprint != again[i].Fingerprint {
			t.Errorf("fingerprint changed without a change in the ticket: %s vs %s", flags[i].Fingerprint, again[i].Fingerprint)
		}
	}
}

func TestBuildFailureSignatureNamesTheFailingTest(t *testing.T) {
	cases := map[string]string{
		"x\n--- first failing case: win-debug/q.xml: policy:loader: Timeout\n": "test: policy:loader (Timeout)",
		"src/a.cpp(12): error C2059: syntax error: ')'":                        "compile: error C2059: syntax error: ')'",
		"Windows verification: FAILED (NFR-031: full QA exceeded 15 minutes)":  "verify: NFR-031: full QA exceeded 15 minutes",
	}
	for log, want := range cases {
		if got := BuildFailureSignature(failedJob(log)); got != want {
			t.Errorf("signature(%q) = %q, want %q", log, got, want)
		}
	}
	host, _ := json.Marshal(map[string]any{"failure_kind": "infrastructure"})
	if got := BuildFailureSignature(BuildJob{Status: "failed", Result: host}); got != "host: infrastructure" {
		t.Errorf("host failure signature = %q", got)
	}
}
