package app_test

import (
	"strings"
	"testing"

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
