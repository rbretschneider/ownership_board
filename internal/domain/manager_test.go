package domain

import (
	"encoding/json"
	"strings"
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
	// One more pass, or one more identical failure, is new news for the manager.
	worse := []ManagerTicketFacts{facts[0]}
	worse[0].Attempts.Build++
	worse[0].RecentJobs = append([]BuildJob{failedJob(sameFailure)}, facts[0].RecentJobs...)
	before := map[string]bool{}
	for _, flag := range byTicket[1] {
		before[flag.Fingerprint] = true
	}
	for _, flag := range ManagerFlags(worse, settings, now) {
		if (flag.Kind == FlagBuildCap || strings.Contains(flag.Fingerprint, ":failure:")) && before[flag.Fingerprint] {
			t.Errorf("%s kept its fingerprint after another pass", flag.Fingerprint)
		}
	}
}

func TestManagerFlagsAStuckBuildJob(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	started := now.Add(-5 * time.Hour)
	ticket := Ticket{ID: 7, IsAutonomous: true, Stage: StageBuild, UpdatedAt: now.Add(-10 * time.Minute)}
	cases := map[string][]BuildJob{
		"stalled:7:job_queued:abcdef123456":  {{Status: "queued", SHA: "abcdef1234567890", CreatedAt: now.Add(-3 * time.Hour)}},
		"stalled:7:job_running:abcdef123456": {{Status: "running", SHA: "abcdef1234567890", CreatedAt: started, StartedAt: &started}},
		"":                                   {{Status: "queued", SHA: "abc", CreatedAt: now.Add(-30 * time.Minute)}},
	}
	for want, jobs := range cases {
		var got string
		for _, flag := range ManagerFlags([]ManagerTicketFacts{{Ticket: ticket, RecentJobs: jobs}}, DefaultBoardSettings(), now) {
			got = flag.Fingerprint
		}
		if got != want {
			t.Errorf("jobs %+v: flag %q, want %q", jobs, got, want)
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
