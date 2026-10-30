package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

var relationKinds = []string{
	string(domain.RelationCausedBy), string(domain.RelationFollowUpOf),
	string(domain.RelationDuplicateOf), string(domain.RelationRelatesTo),
}

// linkKinds are the kinds link_tickets accepts: the relation kinds plus
// depends_on, which create_ticket takes as its own list instead.
var linkKinds = append(append([]string{}, relationKinds...), string(domain.RelationDependsOn))

type addWorkLink struct{ s Services }

func (addWorkLink) Definition() Definition {
	return Definition{
		Name: "add_work_link",
		Description: "Record a pull request or commit that implements a ticket you hold the lease on. " +
			"Record every commit you push for the ticket and its pull request.",
		InputSchema: object(map[string]any{
			"id":   ticketIDProp(),
			"kind": map[string]any{"type": "string", "enum": []string{string(domain.WorkLinkPR), string(domain.WorkLinkCommit)}},
			"url":  prop("string", "For a pr: the pull request URL. For a commit: optional URL of the commit. Must be on the repo's host."),
			"sha":  prop("string", "For a commit: the full or abbreviated hash (7-40 hex characters)."),
		}, "id", "kind"),
	}
}

func (t addWorkLink) Call(ctx context.Context, actor domain.Actor, raw json.RawMessage) (string, error) {
	var args struct {
		ID   int64  `json:"id"`
		Kind string `json:"kind"`
		URL  string `json:"url"`
		SHA  string `json:"sha"`
	}
	if err := decodeArguments(raw, &args); err != nil {
		return "", err
	}
	detail, err := t.s.Board.AddWorkLink(ctx, actor, args.ID, domain.WorkLinkDraft{Kind: args.Kind, URL: args.URL, SHA: args.SHA})
	return outcome(detail, "work link recorded.", err)
}

type recordDecision struct{ s Services }

func (recordDecision) Definition() Definition {
	return Definition{
		Name: "record_decision",
		Description: "Record a significant choice on a ticket you hold the lease on: approach, trade-off, scope cut, dependency, or a deviation from the spec. " +
			"You may replace your own or delegated decisions with supersedes_id; you cannot replace the owner's binding decisions (ask a question instead).",
		InputSchema: object(map[string]any{
			"id":            ticketIDProp(),
			"summary":       prop("string", "The decision in one sentence (at most 300 characters)."),
			"rationale":     prop("string", "Why (at most 2000 characters)."),
			"alternatives":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": domain.MaxDecisionAlternatives, "description": "Options considered and rejected."},
			"supersedes_id": prop("integer", "The decision this replaces, if any."),
		}, "id", "summary"),
	}
}

func (t recordDecision) Call(ctx context.Context, actor domain.Actor, raw json.RawMessage) (string, error) {
	var args struct {
		ID           int64    `json:"id"`
		Summary      string   `json:"summary"`
		Rationale    string   `json:"rationale"`
		Alternatives []string `json:"alternatives"`
		SupersedesID *int64   `json:"supersedes_id"`
	}
	if err := decodeArguments(raw, &args); err != nil {
		return "", err
	}
	draft := domain.DecisionDraft{Summary: args.Summary, Rationale: args.Rationale, Alternatives: args.Alternatives, SupersedesID: args.SupersedesID}
	detail, err := t.s.Board.RecordDecision(ctx, actor, args.ID, draft)
	return outcome(detail, "decision recorded.", err)
}

type linkTickets struct{ s Services }

func (linkTickets) Definition() Definition {
	return Definition{
		Name: "link_tickets",
		Description: "Relate two tickets, read as \"<id> <kind> <other_id>\", e.g. 57 caused_by 42. You must hold the lease on one of them. " +
			"For depends_on (57 depends_on 42: agents cannot claim 57 until 42 is merged) you need the lease on <id> or to have filed it; " +
			"both tickets must be in the same repo and the link must not close a cycle.",
		InputSchema: object(map[string]any{
			"id":       ticketIDProp(),
			"kind":     map[string]any{"type": "string", "enum": linkKinds},
			"other_id": prop("integer", "The other ticket's id."),
		}, "id", "kind", "other_id"),
	}
}

func (t linkTickets) Call(ctx context.Context, actor domain.Actor, raw json.RawMessage) (string, error) {
	var args struct {
		ID      int64  `json:"id"`
		Kind    string `json:"kind"`
		OtherID int64  `json:"other_id"`
	}
	if err := decodeArguments(raw, &args); err != nil {
		return "", err
	}
	detail, err := t.s.Board.LinkTickets(ctx, actor, args.ID, args.Kind, args.OtherID)
	return outcome(detail, "linked.", err)
}

type findTicketsForWork struct{ s Services }

func (findTicketsForWork) Definition() Definition {
	return Definition{
		Name: "find_tickets_for_work",
		Description: "Find the tickets a commit or pull request was recorded on. Use it to confirm an Ownership-Ticket trailer " +
			"found in git before trusting it. Give exactly one of sha or pr_url.",
		InputSchema: object(map[string]any{
			"sha":    prop("string", "Commit hash or prefix of at least 7 hex characters."),
			"pr_url": prop("string", "Pull request URL."),
		}),
	}
}

func (t findTicketsForWork) Call(ctx context.Context, _ domain.Actor, raw json.RawMessage) (string, error) {
	var args struct {
		SHA   string `json:"sha"`
		PRURL string `json:"pr_url"`
	}
	if err := decodeArguments(raw, &args); err != nil {
		return "", err
	}
	found, err := t.s.Board.FindTicketsForWork(ctx, args.SHA, args.PRURL)
	if err != nil {
		return "", err
	}
	if len(found) == 0 {
		return "No ticket has that commit or pull request recorded. A trailer naming a ticket is not confirmed.", nil
	}
	var b strings.Builder
	for _, summary := range found {
		fmt.Fprintf(&b, "- #%d (%s)\n", summary.Ticket.ID, summary.Ticket.Lane)
	}
	return b.String(), nil
}
