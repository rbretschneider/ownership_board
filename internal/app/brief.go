package app

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// Brief limits keep the document focused on what an agent needs now.
const (
	briefRecentComments = 10
	branchSlugMaxLength = 40
)

var nonSlugRunes = regexp.MustCompile(`[^a-z0-9]+`)

// actionInstructions is the prompt for each action. These words are the
// contract between the board and any agent, so they are written for an LLM.
var actionInstructions = map[domain.WorkAction]string{
	domain.ActionRefine: `You are acting as a business analyst. Decide whether this request can be built without guessing.

- If anything material is unclear, ask. Post ONE round of at most 5 questions with a one-paragraph summary of what you understood. Prefer multiple-choice options and mark the one you recommend; the human usually answers from a phone. Asking moves the ticket to Pending Answers and notifies the human. Then stop working on this ticket.
- Otherwise write the spec: problem, scope, out of scope, acceptance criteria as a numbered list, and notes on approach and risk. Save it, then move the ticket to "ready".
- An answer marked "your call" means: use your recommendation, and record the decision in the spec.
- Do not write code while refining.`,

	domain.ActionImplement: `The ticket is now In Progress and leased to you. Build the spec in the project's repository.

- Work on the branch suggested below unless the repository has its own convention.
- Send a heartbeat well before the lease expires while you work.
- If you reach a decision only the human can make, ask questions; that pauses the ticket and releases your lease.
- When the pull request is open: set pr_url and branch on the ticket, comment with what changed and how you verified it, and move the ticket to "review".`,

	domain.ActionResume: `This ticket was already In Progress: a previous session stopped, or the human sent it back from review. Read the newest comments first; they say what is left. Then carry on exactly as for an implementation: finish, update the pull request, comment, and move the ticket to "review".`,
}

// noRepoInstructions apply whenever a ticket has no repo yet.
const noRepoInstructions = `This ticket has no repo yet. Work out which repo it belongs to from the request and the list above, then set repo_id on the ticket. If you cannot tell with confidence, make "Which repo is this for?" one of your questions, with the likely repos as options. A ticket must have a repo before it moves to "ready".`

// BriefInput is everything a brief is rendered from.
type BriefInput struct {
	Detail TicketDetail
	// Repo is the ticket's repo, or nil when it has none.
	Repo *domain.Repo
	// KnownRepos lists candidate repos when the ticket has none.
	KnownRepos []domain.Repo
	// Action may be empty when the agent is only reading.
	Action domain.WorkAction
}

// RenderBrief writes one markdown document holding everything an agent needs to
// act on a ticket.
func RenderBrief(input BriefInput) string {
	var b strings.Builder
	ticket := input.Detail.Ticket
	fmt.Fprintf(&b, "# #%d %s\n\n", ticket.ID, ticket.Title)
	writeFacts(&b, ticket, input.Repo, input.Action)
	writeSection(&b, "Request", ticket.Body, "_No details were given beyond the title._")
	writeSection(&b, "Refined spec", ticket.Spec, "_Not refined yet._")
	writeQuestions(&b, input.Detail.Questions)
	writeComments(&b, input.Detail.Timeline)
	if input.Repo == nil {
		writeKnownRepos(&b, input.KnownRepos)
	}
	if instructions, isKnown := actionInstructions[input.Action]; isKnown {
		writeSection(&b, "What to do now", instructions, "")
	}
	return b.String()
}

// SuggestedBranch is the branch name an agent should use for a ticket.
func SuggestedBranch(ticket domain.Ticket) string {
	slug := strings.Trim(nonSlugRunes.ReplaceAllString(strings.ToLower(ticket.Title), "-"), "-")
	if len(slug) > branchSlugMaxLength {
		slug = strings.TrimRight(slug[:branchSlugMaxLength], "-")
	}
	return fmt.Sprintf("ob/%d-%s", ticket.ID, slug)
}

func writeFacts(b *strings.Builder, ticket domain.Ticket, repo *domain.Repo, action domain.WorkAction) {
	fmt.Fprintf(b, "- Ticket id: %d\n- Kind: %s\n- Lane: %s (%s)\n", ticket.ID, ticket.Kind, ticket.Lane.Label(), ticket.Lane)
	writeRepoFact(b, repo)
	if action != "" {
		fmt.Fprintf(b, "- Action: **%s**\n", action)
	}
	if ticket.Lease != nil {
		fmt.Fprintf(b, "- Lease: %s until %s\n", ticket.Lease.Holder, ticket.Lease.ExpiresAt.UTC().Format(timeLayout))
	}
	branch := ticket.Branch
	if branch == "" {
		branch = SuggestedBranch(ticket)
	}
	fmt.Fprintf(b, "- Branch: `%s`\n", branch)
	if ticket.PRURL != "" {
		fmt.Fprintf(b, "- Pull request: %s\n", ticket.PRURL)
	}
	if repo != nil && repo.Description != "" {
		fmt.Fprintf(b, "\nAbout the repo: %s\n", repo.Description)
	}
	b.WriteString("\n")
}

func writeRepoFact(b *strings.Builder, repo *domain.Repo) {
	if repo == nil {
		b.WriteString("- Repo: **none yet** (see below)\n")
		return
	}
	fmt.Fprintf(b, "- Repo: `%s` on %s (repo_id %d)", repo.FullName, repo.Provider, repo.ID)
	if repo.URL != "" {
		fmt.Fprintf(b, ", %s", repo.URL)
	}
	if repo.DefaultBranch != "" {
		fmt.Fprintf(b, ", default branch `%s`", repo.DefaultBranch)
	}
	b.WriteString("\n")
}

func writeKnownRepos(b *strings.Builder, known []domain.Repo) {
	b.WriteString("## Repos on the board\n\n")
	if len(known) == 0 {
		b.WriteString("_No repos are on the board yet. Ask which repo this is for._\n\n")
	}
	for _, repo := range known {
		fmt.Fprintf(b, "- repo_id %d: `%s` (%s)", repo.ID, repo.FullName, repo.Provider)
		if repo.Description != "" {
			fmt.Fprintf(b, " — %s", repo.Description)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(b, "\n%s\n\n", noRepoInstructions)
}

func writeSection(b *strings.Builder, heading, body, whenEmpty string) {
	text := strings.TrimSpace(body)
	if text == "" {
		text = whenEmpty
	}
	fmt.Fprintf(b, "## %s\n\n%s\n\n", heading, text)
}

func writeQuestions(b *strings.Builder, questions []domain.Question) {
	if len(questions) == 0 {
		return
	}
	b.WriteString("## Questions and answers\n\n")
	round := 0
	for i := range questions {
		question := &questions[i]
		if question.Round != round {
			round = question.Round
			fmt.Fprintf(b, "### Round %d\n\n", round)
		}
		writeQuestion(b, question)
	}
}

func writeQuestion(b *strings.Builder, question *domain.Question) {
	fmt.Fprintf(b, "**Q%d.%d (id %d):** %s\n", question.Round, question.Position+1, question.ID, question.Prompt)
	for index, option := range question.Options {
		marker := ""
		if question.Recommended != nil && *question.Recommended == index {
			marker = " _(recommended)_"
		}
		fmt.Fprintf(b, "  %d. %s%s\n", index, option, marker)
	}
	fmt.Fprintf(b, "\n**A:** %s\n\n", describeAnswer(question))
}

func describeAnswer(question *domain.Question) string {
	answer := question.Answer
	if answer == nil {
		return "_waiting for an answer_"
	}
	var parts []string
	if answer.Choice != nil {
		parts = append(parts, fmt.Sprintf("option %d, %q", *answer.Choice, question.Options[*answer.Choice]))
	}
	if answer.IsDeferred {
		parts = append(parts, "your call (use your recommendation)")
	}
	if answer.Text != "" {
		parts = append(parts, answer.Text)
	}
	return strings.Join(parts, "; ")
}

func writeComments(b *strings.Builder, timeline []domain.TimelineEntry) {
	var comments []domain.TimelineEntry
	for _, entry := range timeline {
		if entry.Type == domain.TimelineComment {
			comments = append(comments, entry)
		}
	}
	if len(comments) == 0 {
		return
	}
	if len(comments) > briefRecentComments {
		comments = comments[len(comments)-briefRecentComments:]
	}
	b.WriteString("## Recent comments (oldest first)\n\n")
	for _, comment := range comments {
		fmt.Fprintf(b, "**%s** (%s, %s):\n%s\n\n", comment.Actor.Name, comment.Actor.Kind, comment.At.UTC().Format(timeLayout), comment.Body)
	}
}

// timeLayout is how the brief prints times: precise enough, easy to read.
const timeLayout = "2006-01-02 15:04 UTC"
