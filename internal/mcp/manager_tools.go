package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

// Tools for the board manager token. They act only on autonomous tickets; the
// board refuses the rest. Ordinary agents get a clear refusal.

type managerFlags struct{ s Services }

func (managerFlags) Definition() Definition {
	return Definition{
		Name: "manager_flags",
		Description: "Board manager only. Lists autonomous tickets that need you: escalated (blocked or open questions), cycling " +
			"(same failure again, repeated repair or review rounds), over the build cap, or stalled. Each flag has a fingerprint; " +
			"handle each once. Also returns today's spend against the daily budget.",
		InputSchema: object(map[string]any{
			"repo_id": prop("integer", "Only this repo (see list_repos). Omit for every repo."),
		}),
	}
}

func (t managerFlags) Call(ctx context.Context, actor domain.Actor, raw json.RawMessage) (string, error) {
	var args struct {
		RepoID int64 `json:"repo_id"`
	}
	if err := decodeArguments(raw, &args); err != nil {
		return "", err
	}
	flags, err := t.s.Board.ManagerFlags(ctx, actor, app.RepoScope{RepoID: args.RepoID})
	if err != nil {
		return "", err
	}
	status, err := t.s.Board.ManagerStatus(ctx, actor)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	fmt.Fprintf(&out, "Budget today: $%.2f of $%d spent in %d session(s); $%.2f left.\n",
		status.Today.CostUSD, status.Settings.ManagerDailyBudgetUSD, status.Today.Sessions, status.RemainingUSD)
	if len(flags) == 0 {
		out.WriteString("No flags: every autonomous ticket in scope is moving.")
		return out.String(), nil
	}
	for _, flag := range flags {
		fmt.Fprintf(&out, "- #%d [%s] %s (stage %s, repo %d): %s. Fingerprint: %s\n",
			flag.TicketID, flag.Kind, flag.Title, flag.Stage, flag.RepoID, flag.Detail, flag.Fingerprint)
	}
	return out.String(), nil
}

type answerQuestions struct{ s Services }

func (answerQuestions) Definition() Definition {
	return Definition{
		Name: "answer_questions",
		Description: "Board manager only, autonomous tickets only. Answer a ticket's open questions so its worker can continue. " +
			"Pick an option by index, or write text. Product-level answers are recorded like any decision, so the owner can review " +
			"and override them.",
		InputSchema: object(map[string]any{
			"id": ticketIDProp(),
			"answers": map[string]any{
				"type": "array",
				"items": object(map[string]any{
					"question_id": prop("integer", "The question's id, from get_ticket."),
					"choice":      prop("integer", "Zero-based index of the chosen option, if any."),
					"text":        prop("string", "Your answer or direction, in a sentence or two."),
				}, "question_id"),
			},
		}, "id", "answers"),
	}
}

func (t answerQuestions) Call(ctx context.Context, actor domain.Actor, raw json.RawMessage) (string, error) {
	var args struct {
		ID      int64 `json:"id"`
		Answers []struct {
			QuestionID int64  `json:"question_id"`
			Choice     *int   `json:"choice"`
			Text       string `json:"text"`
		} `json:"answers"`
	}
	if err := decodeArguments(raw, &args); err != nil {
		return "", err
	}
	drafts := make([]domain.AnswerDraft, 0, len(args.Answers))
	for _, answer := range args.Answers {
		drafts = append(drafts, domain.AnswerDraft{QuestionID: answer.QuestionID, Choice: answer.Choice, Text: answer.Text})
	}
	detail, err := t.s.Board.Answer(ctx, actor, args.ID, drafts)
	return outcome(detail, "answers recorded.", err)
}

type unblockTicket struct{ s Services }

func (unblockTicket) Definition() Definition {
	return Definition{
		Name: "unblock_ticket",
		Description: "Board manager only, autonomous tickets only. Lift a block once its cause is fixed or decided, with a short " +
			"note telling the next worker pass what changed.",
		InputSchema: object(map[string]any{
			"id":      ticketIDProp(),
			"comment": prop("string", "What changed, for the next pass."),
		}, "id"),
	}
}

func (t unblockTicket) Call(ctx context.Context, actor domain.Actor, raw json.RawMessage) (string, error) {
	var args struct {
		ID      int64  `json:"id"`
		Comment string `json:"comment"`
	}
	if err := decodeArguments(raw, &args); err != nil {
		return "", err
	}
	detail, err := t.s.Board.Unblock(ctx, actor, args.ID, args.Comment)
	return outcome(detail, "ticket unblocked.", err)
}
