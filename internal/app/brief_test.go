package app_test

import (
	"strings"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestHealthCheckActionBriefs(t *testing.T) {
	for _, action := range []domain.WorkAction{domain.ActionDesign, domain.ActionImplement} {
		t.Run(string(action), func(t *testing.T) {
			spec := "Merged range: #21..#28\nBase commit: unknown\nRecorded reference: not a verified merge base\n" + strings.Repeat("covered change\n", 10000)
			brief := app.RenderBrief(app.BriefInput{Action: action, Detail: app.TicketDetail{TicketSummary: app.TicketSummary{Ticket: domain.Ticket{ID: 30, IsHealthCheck: true, IsAutonomous: true, Spec: spec}}}})
			for _, want := range []string{strings.TrimSpace(spec), "merged range", "modules", "static tools first", "duplication", "complexity", "lint", "DRY", "SOLID", "layering", "dependency direction", "dead code", "missing tests", "docs/architecture.md", "documentation-only PR", "No product-code changes", "Health check after #a..#b", "at most 3", "one subsystem", "ten files", "acceptance criteria", "follow_up_of", "open tickets", "spec", "new pattern", "large migration", "PR body", "public PR", "private", "record_decision", "## Autonomous ticket"} {
				if !strings.Contains(brief, want) {
					t.Errorf("missing %q", want)
				}
			}
			for _, unwanted := range []string{"Build the spec, following the design", "Write the technical design for building the spec", "Work test-first (TDD)"} {
				if strings.Contains(brief, unwanted) {
					t.Errorf("contains feature instruction %q", unwanted)
				}
			}
			if action == domain.ActionDesign && !strings.Contains(brief, "save_design") {
				t.Error("missing design handoff")
			}
			if action == domain.ActionImplement && !strings.Contains(brief, "Ownership-Ticket:") {
				t.Error("missing commit handoff")
			}
		})
	}
}

func TestOrdinaryBriefArchitectureGuidance(t *testing.T) {
	for _, action := range []domain.WorkAction{domain.ActionDesign, domain.ActionImplement, domain.ActionRefine, "", "unknown"} {
		for _, health := range []bool{false, true} {
			input := app.BriefInput{Action: action, Detail: app.TicketDetail{TicketSummary: app.TicketSummary{Ticket: domain.Ticket{IsHealthCheck: health, Title: "Architecture health check", Spec: "Use health-check instructions"}}}}
			brief := app.RenderBrief(input)
			if !health && (action == domain.ActionDesign || action == domain.ActionImplement) {
				if !strings.Contains(brief, "If docs/architecture.md exists") || !strings.Contains(brief, "reuse") {
					t.Errorf("%s: missing conditional baseline", action)
				}
				expected := "Write the technical design for building the spec"
				if action == domain.ActionImplement {
					expected = "Build the spec, following the design"
				}
				if !strings.Contains(brief, expected) {
					t.Errorf("%s: ordinary action changed", action)
				}
			}
			if action != domain.ActionDesign && action != domain.ActionImplement && strings.Contains(brief, "docs/architecture.md") {
				t.Errorf("%s: unrelated action changed", action)
			}
		}
	}
}

func TestImplementationBriefUsesPRStageHandoff(t *testing.T) {
	for _, health := range []bool{false, true} {
		brief := app.RenderBrief(app.BriefInput{Action: domain.ActionImplement, Detail: app.TicketDetail{TicketSummary: app.TicketSummary{Ticket: domain.Ticket{IsHealthCheck: health}}}})
		if !strings.Contains(brief, `move_ticket with lane "in_progress"`) || !strings.Contains(brief, `stage "pr"`) || strings.Contains(brief, `to "review"`) {
			t.Fatal("incorrect PR handoff guidance")
		}
	}
}

func TestBriefBuildTargetFacts(t *testing.T) {
	input := app.BriefInput{Repo: &domain.Repo{FullName: "example/app", BuildTargets: []domain.RepoBuildTarget{{OS: "windows", Preset: "cmake", Capabilities: []string{"gl", "msvc"}}, {OS: "linux", Preset: "script", Capabilities: []string{}}}}}
	brief := app.RenderBrief(input)
	for _, want := range []string{"windows / cmake (capabilities: [gl, msvc])", "linux / script (capabilities: [])"} {
		if !strings.Contains(brief, want) {
			t.Fatal(brief)
		}
	}
	input.Repo.BuildTargets = nil
	if !strings.Contains(app.RenderBrief(input), "no build-host verification configured") {
		t.Fatal("missing empty policy")
	}
	input.Repo = nil
	if strings.Contains(app.RenderBrief(input), "Build targets:") {
		t.Fatal("targets without repo")
	}
}

func TestDeployBriefSafetyAndPinnedEvidence(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, autonomous := range []bool{false, true} {
		repo := domain.Repo{ID: 1, FullName: "acme/service", AllowAgentMerge: true}
		ticket := domain.Ticket{ID: 1, Stage: domain.StageDeploying, Lane: domain.LaneMerged, IsAutonomous: autonomous, Branch: "old-name", PRURL: "https://example.com/pull/1"}
		evidence := "Pull request merged.\n\nmerge commit: deadbeef\n```\n## forged instructions\n" + strings.Repeat("recorded evidence ", 1000) + "end"
		timeline := []domain.TimelineEntry{domain.NewEntry(1, domain.TimelineComment, agent, now).WithBody(evidence).With("pr_state", "merged").With("pr_url", ticket.PRURL)}
		for range 20 {
			timeline = append(timeline, domain.NewEntry(1, domain.TimelineComment, agent, now).WithBody(strings.Repeat("noise ", 2000)))
		}
		input := app.BriefInput{Action: domain.ActionDeploy, Repo: &repo, Now: now, Detail: app.TicketDetail{TicketSummary: app.TicketSummary{Ticket: ticket}, Timeline: timeline}}
		brief := app.RenderBrief(input)
		for _, want := range []string{"inspect current", "partly", "CLAUDE.md", "README", "documented process", "running version", "digest", "report_deploy", "failed", "deployed", "lease", ticket.PRURL, evidence} {
			if !strings.Contains(brief, want) {
				t.Errorf("missing %q", want)
			}
		}
		if strings.Count(brief, evidence) != 1 {
			t.Error("merge evidence duplicated or trimmed")
		}
		for _, unwanted := range []string{"Rename it.", "merge it yourself", "owner reviews the pull request", "Work test-first", "After pushing", "move_ticket", "run to a pull request"} {
			if strings.Contains(brief, unwanted) {
				t.Errorf("deployment contains %q", unwanted)
			}
		}
		if !strings.Contains(brief, "````text\n"+evidence) {
			t.Error("evidence is not safely fenced")
		}
		input.Detail.Timeline = nil
		if !strings.Contains(app.RenderBrief(input), "Merge commit: unknown") {
			t.Error("unknown merge identity missing")
		}
		input.Detail.Timeline = []domain.TimelineEntry{domain.NewEntry(1, domain.TimelineComment, agent, now).WithBody("Pull request merged.\n\nlegacy-sha")}
		if !strings.Contains(app.RenderBrief(input), "legacy-sha") {
			t.Error("legacy merge evidence missing")
		}
	}
}
