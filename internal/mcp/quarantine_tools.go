package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

type listQuarantine struct{ s Services }

func (listQuarantine) Definition() Definition {
	return Definition{
		Name: "list_quarantine",
		Description: "List a repo's quarantined flaky tests: tests the board found failing on unrelated tickets and then passing on the same commit, " +
			"or that a person or delegate put there. Build jobs carry the active ones; repo scripts may skip them.",
		InputSchema: object(map[string]any{"repo_id": prop("integer", "The repo (see list_repos).")}, "repo_id"),
	}
}

func (t listQuarantine) Call(ctx context.Context, _ domain.Actor, raw json.RawMessage) (string, error) {
	var args struct {
		RepoID int64 `json:"repo_id"`
	}
	if err := decodeArguments(raw, &args); err != nil {
		return "", err
	}
	if t.s.Jobs == nil {
		return "", domain.Invalidf("build jobs are not available on this board")
	}
	list, err := t.s.Jobs.QuarantineList(ctx, args.RepoID)
	if err != nil {
		return "", err
	}
	if len(list) == 0 {
		return "No test is quarantined in this repo.", nil
	}
	var b strings.Builder
	for _, q := range list {
		fix := "no fix ticket"
		if q.FixTicketID != nil {
			fix = fmt.Sprintf("fix #%d", *q.FixTicketID)
		}
		fmt.Fprintf(&b, "- %s [%s, %s by %s, %s]: %s\n", q.Test, q.Status, q.Source, q.AddedBy, fix, q.Reason)
	}
	return b.String(), nil
}

type quarantineTest struct{ s Services }

func (quarantineTest) Definition() Definition {
	return Definition{
		Name: "quarantine_test",
		Description: "Put a flaky test on a repo's quarantine list by hand (recorded as seeded, with your token's name and the board's failure history as evidence). " +
			"Only a delegate or manager token may call this, for tests the owner agreed to quarantine. Without fix_ticket_id the board files an autonomous fix ticket.",
		InputSchema: object(map[string]any{
			"repo_id":       prop("integer", "The repo (see list_repos)."),
			"test":          prop("string", "The exact test name as the repo's runs report it, for example `suite:case`."),
			"reason":        prop("string", "Why the test is flaky, in a sentence."),
			"fix_ticket_id": prop("integer", "An open ticket in the same repo that already fixes the test; the test leaves the list after it merges."),
		}, "repo_id", "test"),
	}
}

func (t quarantineTest) Call(ctx context.Context, actor domain.Actor, raw json.RawMessage) (string, error) {
	var args struct {
		RepoID      int64  `json:"repo_id"`
		Test        string `json:"test"`
		Reason      string `json:"reason"`
		FixTicketID *int64 `json:"fix_ticket_id"`
	}
	if err := decodeArguments(raw, &args); err != nil {
		return "", err
	}
	if t.s.Jobs == nil {
		return "", domain.Invalidf("build jobs are not available on this board")
	}
	q, err := t.s.Jobs.Quarantine(ctx, actor, app.QuarantineInput{RepoID: args.RepoID, Test: args.Test, Reason: args.Reason, FixTicketID: args.FixTicketID})
	if err != nil {
		return "", err
	}
	fix := "no fix ticket"
	if q.FixTicketID != nil {
		fix = fmt.Sprintf("fix ticket #%d", *q.FixTicketID)
	}
	return fmt.Sprintf("Quarantined %q (%s); evidence %s.", q.Test, fix, string(q.Evidence)), nil
}
