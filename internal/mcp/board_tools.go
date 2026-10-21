package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

// Services are the use cases the tools call.
type Services struct {
	Board  *app.BoardService
	Agents *app.AgentService
	Repos  *app.RepoService
}

// maxToolWaitSeconds keeps a long-polling claim inside typical MCP client
// tool timeouts.
const maxToolWaitSeconds = 300

// BoardTools returns every tool the board offers.
func BoardTools(services Services) []Tool {
	return []Tool{
		claimWork{services}, getTicket{services}, listRepos{services}, askQuestions{services},
		saveSpec{services}, moveTicket{services}, addComment{services}, updateTicket{services},
		heartbeat{services}, releaseTicket{services}, createTicket{services},
		addWorkLink{services}, recordDecision{services}, linkTickets{services}, findTicketsForWork{services},
	}
}

type claimWork struct{ s Services }

func (claimWork) Definition() Definition {
	return Definition{
		Name: "claim_work",
		Description: "Claim the next ticket that needs an agent and get its brief. Order: unfinished In Progress work, then New tickets to refine, then Ready tickets to build. " +
			"Set wait_seconds to block until work appears instead of polling. Returns 'No work' when there is nothing to do.",
		InputSchema: object(map[string]any{
			"repo":          prop("string", "Full name of the repo you are working in, such as acme/api. Omit to take work for any repo."),
			"wait_seconds":  prop("integer", fmt.Sprintf("Seconds to wait for work, 0 to %d.", maxToolWaitSeconds)),
			"lease_minutes": prop("integer", "How long to hold the ticket before it can be reclaimed, default 30."),
		}),
	}
}

func (t claimWork) Call(ctx context.Context, actor domain.Actor, raw json.RawMessage) (string, error) {
	var args struct {
		Repo         string `json:"repo"`
		WaitSeconds  int    `json:"wait_seconds"`
		LeaseMinutes int    `json:"lease_minutes"`
	}
	if err := decodeArguments(raw, &args); err != nil {
		return "", err
	}
	input := app.ClaimInput{Repo: args.Repo, WaitSeconds: min(args.WaitSeconds, maxToolWaitSeconds), LeaseMinutes: args.LeaseMinutes}
	work, isClaimed, err := t.s.Agents.Claim(ctx, actor, input)
	if err != nil || !isClaimed {
		return "No work is waiting right now.", err
	}
	return work.Brief, nil
}

type getTicket struct{ s Services }

func (getTicket) Definition() Definition {
	return Definition{
		Name:        "get_ticket",
		Description: "Read a ticket's full brief without claiming it.",
		InputSchema: object(map[string]any{"id": ticketIDProp()}, "id"),
	}
}

func (t getTicket) Call(ctx context.Context, _ domain.Actor, raw json.RawMessage) (string, error) {
	var args struct {
		ID int64 `json:"id"`
	}
	if err := decodeArguments(raw, &args); err != nil {
		return "", err
	}
	return t.s.Agents.Brief(ctx, args.ID)
}

type listRepos struct{ s Services }

func (listRepos) Definition() Definition {
	return Definition{
		Name:        "list_repos",
		Description: "List the repos on the board with their ids, to work out which repo a ticket belongs to.",
		InputSchema: object(map[string]any{}),
	}
}

func (t listRepos) Call(ctx context.Context, _ domain.Actor, raw json.RawMessage) (string, error) {
	if err := decodeArguments(raw, &struct{}{}); err != nil {
		return "", err
	}
	repos, err := t.s.Repos.List(ctx)
	if err != nil {
		return "", err
	}
	if len(repos) == 0 {
		return "No repos are on the board yet.", nil
	}
	var b strings.Builder
	for _, repo := range repos {
		archived := ""
		if repo.IsArchived {
			archived = " [archived]"
		}
		fmt.Fprintf(&b, "- repo_id %d: %s (%s)%s %s\n", repo.ID, repo.FullName, repo.Provider, archived, repo.Description)
	}
	return b.String(), nil
}

type askQuestions struct{ s Services }

func (askQuestions) Definition() Definition {
	question := object(map[string]any{
		"prompt":      prop("string", "The question, answerable from a phone."),
		"options":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Up to 8 short choices. Strongly preferred over open questions."},
		"recommended": prop("integer", "Index of the option you recommend."),
	}, "prompt")
	return Definition{
		Name: "ask_questions",
		Description: "Post one round of clarifying questions. The ticket moves to Pending Answers, the human is notified, and your lease ends. " +
			"Once every question is answered the ticket returns to New for you to refine again.",
		InputSchema: object(map[string]any{
			"id":        ticketIDProp(),
			"summary":   prop("string", "One paragraph: what you understood and why you are asking."),
			"questions": map[string]any{"type": "array", "items": question, "minItems": 1, "maxItems": domain.MaxQuestionsPerAsk},
		}, "id", "summary", "questions"),
	}
}

func (t askQuestions) Call(ctx context.Context, actor domain.Actor, raw json.RawMessage) (string, error) {
	var args struct {
		ID        int64  `json:"id"`
		Summary   string `json:"summary"`
		Questions []struct {
			Prompt      string   `json:"prompt"`
			Options     []string `json:"options"`
			Recommended *int     `json:"recommended"`
		} `json:"questions"`
	}
	if err := decodeArguments(raw, &args); err != nil {
		return "", err
	}
	input := app.AskInput{Summary: args.Summary}
	for _, q := range args.Questions {
		input.Questions = append(input.Questions, domain.QuestionDraft{Prompt: q.Prompt, Options: q.Options, Recommended: q.Recommended})
	}
	detail, err := t.s.Board.Ask(ctx, actor, args.ID, input)
	return outcome(detail, "questions posted; the human has been notified. Stop work on this ticket.", err)
}

type saveSpec struct{ s Services }

func (saveSpec) Definition() Definition {
	return Definition{
		Name:        "save_spec",
		Description: "Save the refined specification (markdown) for a ticket. Replaces any earlier spec.",
		InputSchema: object(map[string]any{"id": ticketIDProp(), "spec": prop("string", "Problem, scope, out of scope, numbered acceptance criteria, approach and risks.")}, "id", "spec"),
	}
}

func (t saveSpec) Call(ctx context.Context, actor domain.Actor, raw json.RawMessage) (string, error) {
	var args struct {
		ID   int64  `json:"id"`
		Spec string `json:"spec"`
	}
	if err := decodeArguments(raw, &args); err != nil {
		return "", err
	}
	detail, err := t.s.Board.SetSpec(ctx, actor, args.ID, args.Spec)
	return outcome(detail, "spec saved.", err)
}

type moveTicket struct{ s Services }

func (moveTicket) Definition() Definition {
	lanes := make([]string, 0, len(domain.Lanes()))
	for _, lane := range domain.Lanes() {
		lanes = append(lanes, string(lane.ID))
	}
	return Definition{
		Name:        "move_ticket",
		Description: "Move a ticket to another lane. Moving ends your lease. Use 'ready' after refining and 'review' once the PR is open.",
		InputSchema: object(map[string]any{
			"id":   ticketIDProp(),
			"lane": map[string]any{"type": "string", "enum": lanes},
		}, "id", "lane"),
	}
}

func (t moveTicket) Call(ctx context.Context, actor domain.Actor, raw json.RawMessage) (string, error) {
	var args struct {
		ID   int64  `json:"id"`
		Lane string `json:"lane"`
	}
	if err := decodeArguments(raw, &args); err != nil {
		return "", err
	}
	detail, err := t.s.Board.Move(ctx, actor, args.ID, app.MoveInput{Lane: args.Lane})
	return outcome(detail, "moved.", err)
}

type addComment struct{ s Services }

func (addComment) Definition() Definition {
	return Definition{
		Name:        "add_comment",
		Description: "Add a markdown comment to a ticket's timeline, such as what changed and how it was verified.",
		InputSchema: object(map[string]any{"id": ticketIDProp(), "body": prop("string", "Markdown text.")}, "id", "body"),
	}
}

func (t addComment) Call(ctx context.Context, actor domain.Actor, raw json.RawMessage) (string, error) {
	var args struct {
		ID   int64  `json:"id"`
		Body string `json:"body"`
	}
	if err := decodeArguments(raw, &args); err != nil {
		return "", err
	}
	detail, err := t.s.Board.Comment(ctx, actor, args.ID, args.Body)
	return outcome(detail, "comment added.", err)
}

type updateTicket struct{ s Services }

func (updateTicket) Definition() Definition {
	return Definition{
		Name:        "update_ticket",
		Description: "Set a ticket's repo, pull request link, branch or title. Only the fields you pass change.",
		InputSchema: object(map[string]any{
			"id":      ticketIDProp(),
			"repo_id": prop("integer", "The repo the ticket belongs to (see list_repos)."),
			"pr_url":  prop("string", "Absolute URL of the pull request."),
			"branch":  prop("string", "The branch name."),
			"title":   prop("string", "A clearer title."),
		}, "id"),
	}
}

func (t updateTicket) Call(ctx context.Context, actor domain.Actor, raw json.RawMessage) (string, error) {
	var args struct {
		ID     int64   `json:"id"`
		RepoID *int64  `json:"repo_id"`
		PRURL  *string `json:"pr_url"`
		Branch *string `json:"branch"`
		Title  *string `json:"title"`
	}
	if err := decodeArguments(raw, &args); err != nil {
		return "", err
	}
	edit := domain.TicketEdit{RepoID: args.RepoID, PRURL: args.PRURL, Branch: args.Branch, Title: args.Title}
	detail, err := t.s.Board.Edit(ctx, actor, args.ID, edit)
	return outcome(detail, "updated.", err)
}

type heartbeat struct{ s Services }

func (heartbeat) Definition() Definition {
	return Definition{
		Name:        "heartbeat",
		Description: "Extend your lease on a ticket you are still working on.",
		InputSchema: object(map[string]any{"id": ticketIDProp(), "lease_minutes": prop("integer", "New lease length from now, default 30.")}, "id"),
	}
}

func (t heartbeat) Call(ctx context.Context, actor domain.Actor, raw json.RawMessage) (string, error) {
	var args struct {
		ID           int64 `json:"id"`
		LeaseMinutes int   `json:"lease_minutes"`
	}
	if err := decodeArguments(raw, &args); err != nil {
		return "", err
	}
	detail, err := t.s.Agents.Heartbeat(ctx, actor, args.ID, args.LeaseMinutes)
	return outcome(detail, "lease extended.", err)
}

type releaseTicket struct{ s Services }

func (releaseTicket) Definition() Definition {
	return Definition{
		Name:        "release_ticket",
		Description: "Give up your lease without moving the ticket, so another session can pick it up.",
		InputSchema: object(map[string]any{"id": ticketIDProp()}, "id"),
	}
}

func (t releaseTicket) Call(ctx context.Context, actor domain.Actor, raw json.RawMessage) (string, error) {
	var args struct {
		ID int64 `json:"id"`
	}
	if err := decodeArguments(raw, &args); err != nil {
		return "", err
	}
	detail, err := t.s.Agents.Release(ctx, actor, args.ID)
	return outcome(detail, "released.", err)
}

type createTicket struct{ s Services }

func (createTicket) Definition() Definition {
	return Definition{
		Name:        "create_ticket",
		Description: "File a new story or bug, such as a follow-up you found while working. It starts in New.",
		InputSchema: object(map[string]any{
			"kind":    map[string]any{"type": "string", "enum": []string{string(domain.KindStory), string(domain.KindBug)}},
			"title":   prop("string", "Short title."),
			"body":    prop("string", "Markdown details."),
			"repo_id": prop("integer", "The repo, if known."),
			"relation": map[string]any{"type": "string", "enum": relationKinds,
				"description": "How the new ticket relates to related_ticket_id, e.g. follow_up_of. Needs the lease on that ticket."},
			"related_ticket_id": prop("integer", "The ticket the new one relates to."),
		}, "kind", "title"),
	}
}

func (t createTicket) Call(ctx context.Context, actor domain.Actor, raw json.RawMessage) (string, error) {
	var args struct {
		Kind            string `json:"kind"`
		Title           string `json:"title"`
		Body            string `json:"body"`
		RepoID          int64  `json:"repo_id"`
		Relation        string `json:"relation"`
		RelatedTicketID int64  `json:"related_ticket_id"`
	}
	if err := decodeArguments(raw, &args); err != nil {
		return "", err
	}
	draft := domain.TicketDraft{RepoID: args.RepoID, Kind: args.Kind, Title: args.Title, Body: args.Body}
	if (args.Relation == "") != (args.RelatedTicketID == 0) {
		return "", &ArgumentError{Message: "give relation and related_ticket_id together"}
	}
	if args.Relation != "" {
		detail, err := t.s.Board.CreateRelated(ctx, actor, draft, args.Relation, args.RelatedTicketID)
		return outcome(detail, "created and linked.", err)
	}
	detail, err := t.s.Board.Create(ctx, actor, draft)
	return outcome(detail, "created.", err)
}

// outcome is the one-line confirmation every mutating tool returns.
func outcome(detail app.TicketDetail, what string, err error) (string, error) {
	if err != nil {
		return "", err
	}
	ticket := detail.Ticket
	return fmt.Sprintf("Ticket #%d (%s) is in %s: %s", ticket.ID, ticket.Title, ticket.Lane.Label(), what), nil
}
