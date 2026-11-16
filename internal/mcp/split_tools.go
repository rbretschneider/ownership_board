package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

type splitTicket struct{ s Services }

func (splitTicket) Definition() Definition {
	child := object(map[string]any{
		"title": prop("string", "Short title of this story."),
		"kind":  map[string]any{"type": "string", "enum": []string{string(domain.KindStory), string(domain.KindBug)}, "description": "Defaults to the parent's kind."},
		"body":  prop("string", "One or two sentences: which part of the parent this story delivers."),
		"spec": prop("string", "This story's own spec in markdown: problem, scope, out of scope, numbered acceptance criteria with the tests that prove them, the files it touches, and `Build tier: small | normal`. "+
			"Every criterion that leaves the parent lands in exactly one story, with its tests."),
		"after": map[string]any{"type": "array", "items": map[string]any{"type": "integer"},
			"description": "Earlier stories (1-based positions in this list) this one waits for. Omit to wait for the story before it; pass [] for none."},
	}, "title", "spec")
	return Definition{
		Name: "split_ticket",
		Description: "Split a ticket that is bigger than one half-day Sonnet story into child stories, in build order. " +
			"Each child gets its own spec, inherits the repo and autonomous mode, and waits for the children before it. " +
			"Without keep_spec the parent becomes an epic that nobody works and that closes itself when every child is deployed; stop work on it. " +
			"With keep_spec (autonomous tickets in design or build only) the parent keeps building that narrowed core and the children wait for it. " +
			"A split moves whole features with their acceptance criteria and tests; never use it to drop or defer a test for code that stays. " +
			"The split is recorded as a decision on the parent.",
		InputSchema: object(map[string]any{
			"id":        ticketIDProp(),
			"reason":    prop("string", "One or two sentences: why this is too big for one story (criteria, files, modules)."),
			"keep_spec": prop("string", "The parent's narrowed spec when it keeps the core; omit for a full split."),
			"children":  map[string]any{"type": "array", "items": child, "minItems": 1, "maxItems": domain.MaxSplitChildren},
		}, "id", "reason", "children"),
	}
}

func (t splitTicket) Call(ctx context.Context, actor domain.Actor, raw json.RawMessage) (string, error) {
	var args struct {
		ID       int64  `json:"id"`
		Reason   string `json:"reason"`
		KeepSpec string `json:"keep_spec"`
		Children []struct {
			Title string `json:"title"`
			Kind  string `json:"kind"`
			Body  string `json:"body"`
			Spec  string `json:"spec"`
			After *[]int `json:"after"`
		} `json:"children"`
	}
	if err := decodeArguments(raw, &args); err != nil {
		return "", err
	}
	draft := domain.SplitDraft{Reason: args.Reason, KeepSpec: args.KeepSpec}
	for _, child := range args.Children {
		parsed := domain.SplitChildDraft{Title: child.Title, Kind: child.Kind, Body: child.Body, Spec: child.Spec}
		if child.After != nil {
			parsed.After, parsed.HasAfter = *child.After, true
		}
		draft.Children = append(draft.Children, parsed)
	}
	result, err := t.s.Board.Split(ctx, actor, args.ID, draft)
	if err != nil {
		return "", err
	}
	return splitOutcome(result, draft.IsNarrowing()), nil
}

// splitOutcome says what the split filed and what the caller does next.
func splitOutcome(result app.SplitResult, isNarrowing bool) string {
	refs := make([]string, len(result.ChildIDs))
	for i, id := range result.ChildIDs {
		refs[i] = fmt.Sprintf("#%d", id)
	}
	parent := result.Parent.Ticket
	if isNarrowing {
		return fmt.Sprintf("Ticket #%d (%s) keeps its narrowed core; filed %s, which wait for it. Carry on with the narrowed spec only.",
			parent.ID, parent.Title, strings.Join(refs, ", "))
	}
	return fmt.Sprintf("Ticket #%d (%s) is now an epic split into %s in build order. It closes itself when they are all deployed. Stop work on #%d.",
		parent.ID, parent.Title, strings.Join(refs, ", "), parent.ID)
}
