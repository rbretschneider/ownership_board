package app

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// BoardService holds the use cases people and agents share: reading the board
// and changing tickets.
type BoardService struct {
	store  Store
	clock  Clock
	writer ticketWriter
}

// MoveInput is a request to put a ticket in a lane.
type MoveInput struct {
	Lane    string
	AfterID *int64
}

// AskInput is a round of clarifying questions.
type AskInput struct {
	Summary   string
	Questions []domain.QuestionDraft
}

// NewBoardService wires the service to its dependencies.
func NewBoardService(store Store, clock Clock, sink EventSink) *BoardService {
	return &BoardService{store: store, clock: clock, writer: ticketWriter{store: store, sink: sink, clock: clock}}
}

// Board returns the lanes, repos and every ticket within scope.
func (s *BoardService) Board(ctx context.Context, scope RepoScope) (Board, error) {
	var board Board
	err := s.store.Read(ctx, func(repos Repos) error {
		var err error
		board, err = loadBoard(ctx, repos, scope.filter())
		return err
	})
	return board, err
}

// Ticket returns one ticket with its questions and history.
func (s *BoardService) Ticket(ctx context.Context, id int64) (TicketDetail, error) {
	var detail TicketDetail
	err := s.store.Read(ctx, func(repos Repos) error {
		ticket, err := repos.Tickets().Get(ctx, id)
		if err != nil {
			return err
		}
		detail, err = loadDetail(ctx, repos, ticket)
		return err
	})
	return detail, err
}

// Create adds a ticket at the bottom of its lane, so a lane is first come,
// first served until someone drags it into a different order. Any
// dependencies are added in the same transaction: if one is refused, no
// ticket is created.
func (s *BoardService) Create(ctx context.Context, actor domain.Actor, draft domain.TicketDraft) (TicketDetail, error) {
	if err := rejectHiddenFromAgent(actor, map[string]string{"title": draft.Title, "body": draft.Body}); err != nil {
		return TicketDetail{}, err
	}
	ticket, err := domain.NewTicket(draft, actor, s.clock.Now())
	if err != nil {
		return TicketDetail{}, err
	}
	dependsOn, err := parseDependencies(ticket, draft.DependsOn)
	if err != nil {
		return TicketDetail{}, err
	}
	var detail TicketDetail
	var events []Event
	err = s.store.Write(ctx, func(repos Repos) error {
		if err := requireRepo(ctx, repos, ticket.RepoID); err != nil {
			return err
		}
		if ticket.Rank, err = rankFor(ctx, repos.Tickets(), ticket.Lane, 0, atBottom); err != nil {
			return err
		}
		if err := repos.Tickets().Insert(ctx, &ticket); err != nil {
			return err
		}
		entries := []domain.TimelineEntry{domain.NewEntry(ticket.ID, domain.TimelineCreated, actor, ticket.CreatedAt)}
		detail, events, err = s.recordCreated(ctx, repos, actor, ticket, entries, dependsOn)
		return err
	})
	if err != nil {
		return TicketDetail{}, err
	}
	s.writer.sink.Publish(ctx, events)
	return detail, nil
}

// CreateAutonomous files a ticket whose spec the owner agreed in a chat with a
// delegate agent. It starts Ready with the spec approved, and the agreement
// is logged as a binding decision, so every brief carries it. The queue of
// autonomous tickets is unlimited; how many are worked at once is the
// in-flight cap's business, and how fast one token may file them is limited
// here.
func (s *BoardService) CreateAutonomous(ctx context.Context, actor domain.Actor, draft domain.AutonomousDraft) (TicketDetail, error) {
	if err := rejectHiddenFromAgent(actor, map[string]string{"title": draft.Title, "body": draft.Body, "spec": draft.Spec}); err != nil {
		return TicketDetail{}, err
	}
	if err := domain.RejectConcealedMarkdown("spec", draft.Spec); err != nil {
		return TicketDetail{}, err
	}
	ticket, err := domain.NewAutonomousTicket(draft, actor, s.clock.Now())
	if err != nil {
		return TicketDetail{}, err
	}
	dependsOn, err := parseDependencies(ticket, draft.DependsOn)
	if err != nil {
		return TicketDetail{}, err
	}
	var detail TicketDetail
	var events []Event
	err = s.store.Write(ctx, func(repos Repos) error {
		if err := requireFilingRoom(ctx, repos, actor, ticket.CreatedAt); err != nil {
			return err
		}
		if ticket.Rank, err = rankFor(ctx, repos.Tickets(), ticket.Lane, 0, atBottom); err != nil {
			return err
		}
		if err := repos.Tickets().Insert(ctx, &ticket); err != nil {
			return err
		}
		decision := domain.DecisionFromAgreedSpec(ticket.ID, ticket.SpecRevision, actor, ticket.CreatedAt)
		if err := repos.Decisions().Insert(ctx, &decision); err != nil {
			return err
		}
		entries := []domain.TimelineEntry{
			domain.NewEntry(ticket.ID, domain.TimelineCreated, actor, ticket.CreatedAt).With("autonomous", true),
			domain.NewEntry(ticket.ID, domain.TimelineSpec, actor, ticket.CreatedAt).With("revision", ticket.SpecRevision).With("submitted", true),
		}
		detail, events, err = s.recordCreated(ctx, repos, actor, ticket, entries, dependsOn)
		return err
	})
	if err != nil {
		return TicketDetail{}, err
	}
	s.writer.sink.Publish(ctx, events)
	return detail, nil
}

// recordCreated adds a new ticket's dependencies and records its first
// timeline entries, in the caller's transaction.
func (s *BoardService) recordCreated(ctx context.Context, repos Repos, actor domain.Actor, ticket domain.Ticket, entries []domain.TimelineEntry, dependsOn []int64) (TicketDetail, []Event, error) {
	dependencyEntries, otherEvents, err := s.addDependencies(ctx, repos, actor, ticket, dependsOn)
	if err != nil {
		return TicketDetail{}, nil, err
	}
	events, err := record(ctx, repos, ticket, append(entries, dependencyEntries...))
	if err != nil {
		return TicketDetail{}, nil, err
	}
	detail, err := loadDetail(ctx, repos, ticket)
	return detail, append(events, otherEvents...), err
}

// parseDependencies validates the ids a new ticket depends on. Dependencies
// are same repo only, so they need the ticket to have one.
func parseDependencies(ticket domain.Ticket, ids []int64) ([]int64, error) {
	dependsOn, err := domain.ParseDependencies(ids)
	if err != nil {
		return nil, err
	}
	if len(dependsOn) > 0 && ticket.RepoID == 0 {
		return nil, domain.Invalidf("depends_on needs repo_id: dependencies are same repo only")
	}
	return dependsOn, nil
}

// requireFilingRoom limits how many autonomous tickets one token files in an
// hour, so a leaked or looping delegate token cannot flood the queue or the
// owner's phone. Revoking the token stops it at once. Tickets record their
// creator by name, so the count is per token only because agent token names
// are unique and never reused, even after a token is revoked: the same
// invariant isCreator relies on. A new token never inherits or shares an old
// token's budget.
func requireFilingRoom(ctx context.Context, repos Repos, actor domain.Actor, now time.Time) error {
	filed, err := repos.Tickets().CountAutonomousFiledSince(ctx, actor.Name, now.Add(-time.Hour))
	if err != nil {
		return err
	}
	if filed >= domain.MaxAutonomousFilingsPerHour {
		return domain.Conflictf("this token filed %d autonomous tickets in the last hour, the most allowed; file the rest later", filed)
	}
	return nil
}

// Edit changes a ticket's descriptive fields.
func (s *BoardService) Edit(ctx context.Context, actor domain.Actor, id int64, edit domain.TicketEdit) (TicketDetail, error) {
	if err := rejectHiddenFromAgent(actor, editText(edit)); err != nil {
		return TicketDetail{}, err
	}
	return s.writer.change(ctx, actor, id, func(ctx context.Context, repos Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		if edit.RepoID != nil {
			if err := requireRepo(ctx, repos, *edit.RepoID); err != nil {
				return nil, err
			}
		}
		changed, err := ticket.Apply(edit, s.clock.Now())
		if err != nil || len(changed) == 0 {
			return nil, err
		}
		if actor.IsAgent() {
			if err := ticket.AgentMayEdit(changed); err != nil {
				return nil, err
			}
		}
		if slices.Contains(changed, "repo") {
			if err := requireDependenciesFollowRepo(ctx, repos, *ticket); err != nil {
				return nil, err
			}
		}
		if err := s.linkPRField(ctx, repos, *ticket, changed, actor); err != nil {
			return nil, err
		}
		if slices.Contains(changed, "autonomous") {
			decision := domain.DecisionFromAutonomySwitch(ticket.ID, ticket.IsAutonomous, actor, ticket.UpdatedAt)
			if err := repos.Decisions().Insert(ctx, &decision); err != nil {
				return nil, err
			}
		}
		entry := domain.NewEntry(ticket.ID, domain.TimelineEdited, actor, ticket.UpdatedAt).With("fields", changed)
		return []domain.TimelineEntry{entry}, nil
	})
}

// linkPRField also records a newly set pr_url as a work link, so agents that
// only set the field still leave a traceable link.
func (s *BoardService) linkPRField(ctx context.Context, repos Repos, ticket domain.Ticket, changed []string, actor domain.Actor) error {
	if ticket.PRURL == "" || !slices.Contains(changed, "pr_url") {
		return nil
	}
	if err := requireLeaseForAgent(ticket, actor, s.clock.Now()); err != nil {
		return err
	}
	link, isNew, err := s.newWorkLink(ctx, repos, ticket, domain.WorkLinkDraft{Kind: string(domain.WorkLinkPR), URL: ticket.PRURL}, actor)
	if err != nil || !isNew {
		return err
	}
	return repos.WorkLinks().Insert(ctx, &link)
}

func editText(edit domain.TicketEdit) map[string]string {
	fields := map[string]string{}
	for name, value := range map[string]*string{"title": edit.Title, "body": edit.Body, "branch": edit.Branch, "pr_url": edit.PRURL} {
		if value != nil {
			fields[name] = *value
		}
	}
	return fields
}

// Move puts a ticket in a lane at a position. The server picks the stage. An
// agent's only move is a finished build into Review; a person may drag
// anywhere, landing on the first stage whose checkpoints are met, and dragging
// to another lane clears a block because the person has decided.
func (s *BoardService) Move(ctx context.Context, actor domain.Actor, id int64, input MoveInput) (TicketDetail, error) {
	lane, err := domain.ParseLane(input.Lane)
	if err != nil {
		return TicketDetail{}, err
	}
	place := placement{AfterID: input.AfterID}
	return s.writer.change(ctx, actor, id, func(ctx context.Context, repos Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		gates, open, err := gatesAndOpen(ctx, repos, *ticket)
		if err != nil {
			return nil, err
		}
		if actor.IsAgent() {
			return s.submitForReview(ctx, repos, ticket, lane, open, actor)
		}
		rank, err := rankFor(ctx, repos.Tickets(), lane, ticket.ID, place)
		if err != nil {
			return nil, err
		}
		if lane == ticket.Lane {
			ticket.Reorder(rank, s.clock.Now())
			return nil, nil
		}
		ticket.ClearBlockForDrop()
		from, fromLane := ticket.Stage, ticket.Lane
		isLaneChange := ticket.MoveToStage(ticket.DropStage(lane, gates), rank, s.clock.Now())
		return stageEntries(*ticket, from, fromLane, actor, isLaneChange), nil
	})
}

func (s *BoardService) submitForReview(ctx context.Context, repos Repos, ticket *domain.Ticket, lane domain.Lane, open int, actor domain.Actor) ([]domain.TimelineEntry, error) {
	if lane != domain.LaneReview {
		return nil, domain.Forbiddenf("agents only move a finished build to review; use save_spec, save_design, ask_questions or mark_blocked")
	}
	next, err := ticket.SubmitForReview(open)
	if err != nil {
		return nil, err
	}
	return changeStage(ctx, repos, ticket, next, actor, s.clock)
}

// Comment adds a note to a ticket's timeline.
func (s *BoardService) Comment(ctx context.Context, actor domain.Actor, id int64, body string) (TicketDetail, error) {
	text, err := domain.RequireText("body", body, domain.MaxCommentLength)
	if err != nil {
		return TicketDetail{}, err
	}
	if err := rejectHiddenFromAgent(actor, map[string]string{"body": text}); err != nil {
		return TicketDetail{}, err
	}
	return s.writer.change(ctx, actor, id, func(_ context.Context, _ Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		ticket.UpdatedAt = s.clock.Now()
		entry := domain.NewEntry(ticket.ID, domain.TimelineComment, actor, ticket.UpdatedAt).WithBody(text)
		return []domain.TimelineEntry{entry}, nil
	})
}

// Ask posts a round of questions. The ticket stays where it is but the agent
// gives up its lease: the ball is with the owner until every question is
// answered.
func (s *BoardService) Ask(ctx context.Context, actor domain.Actor, id int64, input AskInput) (TicketDetail, error) {
	summary, err := domain.LimitText("summary", strings.TrimSpace(input.Summary), domain.MaxCommentLength)
	if err != nil {
		return TicketDetail{}, err
	}
	if len(input.Questions) == 0 || len(input.Questions) > domain.MaxQuestionsPerAsk {
		return TicketDetail{}, domain.Invalidf("ask between 1 and %d questions", domain.MaxQuestionsPerAsk)
	}
	if err := rejectHiddenFromAgent(actor, askText(input)); err != nil {
		return TicketDetail{}, err
	}
	return s.writer.change(ctx, actor, id, func(ctx context.Context, repos Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		if err := ticket.HandOffForQuestions(s.clock.Now()); err != nil {
			return nil, err
		}
		round, err := s.insertRound(ctx, repos, ticket.ID, input.Questions, actor)
		if err != nil {
			return nil, err
		}
		asked := domain.NewEntry(ticket.ID, domain.TimelineQuestions, actor, ticket.UpdatedAt).
			WithBody(summary).With("round", round).With("count", len(input.Questions))
		return []domain.TimelineEntry{asked}, nil
	})
}

// Answer records a person's answers. Once nothing is left open the ticket is
// claimable again in the stage it was asked from.
func (s *BoardService) Answer(ctx context.Context, actor domain.Actor, id int64, answers []domain.AnswerDraft) (TicketDetail, error) {
	if !actor.IsHuman() {
		return TicketDetail{}, domain.Forbiddenf("only a person can answer questions")
	}
	if len(answers) == 0 {
		return TicketDetail{}, domain.Invalidf("answers are required")
	}
	return s.writer.change(ctx, actor, id, func(ctx context.Context, repos Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		round, _, answeredQuestions, err := s.applyAnswers(ctx, repos, ticket.ID, answers, actor)
		if err != nil {
			return nil, err
		}
		ticket.UpdatedAt = s.clock.Now()
		if err := recordAnswerDecisions(ctx, repos, answeredQuestions, actor, ticket.UpdatedAt); err != nil {
			return nil, err
		}
		answered := domain.NewEntry(ticket.ID, domain.TimelineAnswers, actor, ticket.UpdatedAt).
			With("round", round).With("count", len(answers))
		return []domain.TimelineEntry{answered}, nil
	})
}

// Delete removes a ticket and its history.
func (s *BoardService) Delete(ctx context.Context, actor domain.Actor, id int64) error {
	if !actor.IsHuman() {
		return domain.Forbiddenf("only a person can delete tickets")
	}
	var ticket domain.Ticket
	var events []Event
	err := s.store.Write(ctx, func(repos Repos) error {
		var err error
		if ticket, err = repos.Tickets().Get(ctx, id); err != nil {
			return err
		}
		relations, err := repos.Relations().ForTicket(ctx, id)
		if err != nil {
			return err
		}
		if err := repos.Tickets().Delete(ctx, id); err != nil {
			return err
		}
		events, err = s.noteDeletedRelations(ctx, repos, ticket, relations, actor)
		return err
	})
	if err != nil {
		return err
	}
	deleted := Event{Type: domain.TimelineDeleted, TicketID: id, Title: ticket.Title, Lane: ticket.Lane, Actor: actor}
	s.writer.sink.Publish(ctx, append(events, deleted))
	return nil
}

// noteDeletedRelations leaves a line on every ticket that was linked to a
// deleted one, so the surviving side still knows the link existed.
func (s *BoardService) noteDeletedRelations(ctx context.Context, repos Repos, deleted domain.Ticket, relations []domain.Relation, actor domain.Actor) ([]Event, error) {
	var events []Event
	for _, relation := range relations {
		otherID := relation.FromID
		if otherID == deleted.ID {
			otherID = relation.ToID
		}
		other, err := repos.Tickets().Get(ctx, otherID)
		if err != nil {
			return nil, err
		}
		entry := domain.NewEntry(other.ID, domain.TimelineRelation, actor, s.clock.Now()).
			With("kind", string(relation.Kind)).With("ticket_id", deleted.ID).With("deleted", true)
		recorded, err := record(ctx, repos, other, []domain.TimelineEntry{entry})
		if err != nil {
			return nil, err
		}
		events = append(events, recorded...)
	}
	return events, nil
}

func (s *BoardService) insertRound(ctx context.Context, repos Repos, ticketID int64, drafts []domain.QuestionDraft, actor domain.Actor) (int, error) {
	latest, err := repos.Questions().LatestRound(ctx, ticketID)
	if err != nil {
		return 0, err
	}
	round := latest + 1
	now := s.clock.Now()
	for position, draft := range drafts {
		question, err := domain.NewQuestion(draft, round, position, actor, now)
		if err != nil {
			return 0, err
		}
		question.TicketID = ticketID
		if err := repos.Questions().Insert(ctx, &question); err != nil {
			return 0, err
		}
	}
	return round, nil
}

// applyAnswers saves each answer and returns the latest round answered, how
// many questions remain open, and the questions just answered.
func (s *BoardService) applyAnswers(ctx context.Context, repos Repos, ticketID int64, answers []domain.AnswerDraft, actor domain.Actor) (int, int, []domain.Question, error) {
	questions, err := repos.Questions().ForTicket(ctx, ticketID)
	if err != nil {
		return 0, 0, nil, err
	}
	byID := make(map[int64]*domain.Question, len(questions))
	for i := range questions {
		byID[questions[i].ID] = &questions[i]
	}
	round := 0
	answered := make([]domain.Question, 0, len(answers))
	for _, answer := range answers {
		question, isKnown := byID[answer.QuestionID]
		if !isKnown {
			return 0, 0, nil, domain.Invalidf("question %d does not belong to ticket %d", answer.QuestionID, ticketID)
		}
		if err := question.ApplyAnswer(answer, actor, s.clock.Now()); err != nil {
			return 0, 0, nil, err
		}
		if err := repos.Questions().SaveAnswer(ctx, *question); err != nil {
			return 0, 0, nil, err
		}
		round = max(round, question.Round)
		answered = append(answered, *question)
	}
	return round, countOpen(questions), answered, nil
}

func askText(input AskInput) map[string]string {
	fields := map[string]string{"summary": input.Summary}
	for _, question := range input.Questions {
		fields["question prompt"] += question.Prompt
		fields["option"] += strings.Join(question.Options, "")
	}
	return fields
}

// requireRepo checks a repo exists; zero means "no repo" and always passes.
func requireRepo(ctx context.Context, repos Repos, repoID int64) error {
	if repoID == 0 {
		return nil
	}
	_, err := repos.CodeRepos().Get(ctx, repoID)
	return err
}

func loadBoard(ctx context.Context, repos Repos, filter TicketFilter) (Board, error) {
	codeRepos, err := repos.CodeRepos().List(ctx)
	if err != nil {
		return Board{}, err
	}
	tickets, err := repos.Tickets().List(ctx, filter)
	if err != nil {
		return Board{}, err
	}
	openCounts, err := repos.Questions().OpenCounts(ctx)
	if err != nil {
		return Board{}, err
	}
	waiting, err := repos.Relations().Waiting(ctx)
	if err != nil {
		return Board{}, err
	}
	summaries := make([]TicketSummary, 0, len(tickets))
	for _, ticket := range tickets {
		summaries = append(summaries, TicketSummary{Ticket: ticket, OpenQuestions: openCounts[ticket.ID], WaitingOn: waiting[ticket.ID]})
	}
	return Board{Lanes: domain.Lanes(), Stages: domain.Stages(), Repos: codeRepos, Tickets: summaries}, nil
}
