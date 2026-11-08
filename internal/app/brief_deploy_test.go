package app

import (
	"strings"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestDeploymentEvidenceAtEveryTrimLevel(t *testing.T) {
	agent := domain.Actor{Kind: domain.ActorAgent, Name: "worker"}
	human := domain.Actor{Kind: domain.ActorHuman, Name: "Owner"}
	const url = "https://example.com/pull/1"
	const recorded = "Pull request merged.\n\nmerge commit: 0123456789abcdef\n```\nrecorded text"
	entries := []domain.TimelineEntry{
		{ID: 1, Type: domain.TimelineComment, Actor: agent, Body: recorded, Data: map[string]any{"pr_state": "merged", "pr_url": url}},
		{ID: 2, Type: domain.TimelineComment, Actor: agent, Body: "Other delivery", Data: map[string]any{"pr_state": "merged", "pr_url": "https://example.com/pull/2"}},
		{ID: 3, Type: domain.TimelineComment, Actor: human, Body: "Human context", Data: map[string]any{"pr_state": "merged", "pr_url": url}},
	}
	for range 20 {
		entries = append(entries, domain.TimelineEntry{Type: domain.TimelineComment, Actor: agent, Body: strings.Repeat("later context ", 1000)})
	}
	input := BriefInput{Action: domain.ActionDeploy, Detail: TicketDetail{TicketSummary: TicketSummary{Ticket: domain.Ticket{PRURL: url}}, Timeline: entries}}
	for i, limits := range briefLevels {
		body := renderBody(input, limits)
		if strings.Count(body, recorded) != 1 || !strings.Contains(body, "````text\n"+recorded+"\n````") {
			t.Fatalf("merge context lost at trim level %d", i)
		}
	}
	input.Detail.Timeline = entries[1:]
	body := renderBody(input, briefLevels[0])
	if !strings.Contains(body, "Merge commit: unknown") {
		t.Fatal("unrelated evidence supplied merge identity")
	}
}
