package app

import (
	"context"
	"strings"

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
	return &BoardService{store: store, clock: clock, writer: ticketWriter{store: store, sink: sink}}
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
// first served until someone drags it into a different order.
func (s *BoardService) Create(ctx context.Context, actor domain.Actor, draft domain.TicketDraft) (TicketDetail, error) {
	ticket, err := domain.NewTicket(draft, actor, s.clock.Now())
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
		entry := domain.NewEntry(ticket.ID, domain.TimelineCreated, actor, ticket.CreatedAt)
		if events, err = record(ctx, repos, ticket, []domain.TimelineEntry{entry}); err != nil {
			return err
		}
		detail, err = loadDetail(ctx, repos, ticket)
		return err
	})
	if err != nil {
		return TicketDetail{}, err
	}
	s.writer.sink.Publish(ctx, events)
	return detail, nil
}

// Edit changes a ticket's descriptive fields.
func (s *BoardService) Edit(ctx context.Context, actor domain.Actor, id int64, edit domain.TicketEdit) (TicketDetail, error) {
	return s.writer.change(ctx, id, func(ctx context.Context, repos Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		if edit.RepoID != nil {
			if err := requireRepo(ctx, repos, *edit.RepoID); err != nil {
				return nil, err
			}
		}
		changed, err := ticket.Apply(edit, s.clock.Now())
		if err != nil || len(changed) == 0 {
			return nil, err
		}
		entry := domain.NewEntry(ticket.ID, domain.TimelineEdited, actor, ticket.UpdatedAt).With("fields", changed)
		return []domain.TimelineEntry{entry}, nil
	})
}

// Move puts a ticket in a lane at a position.
func (s *BoardService) Move(ctx context.Context, actor domain.Actor, id int64, input MoveInput) (TicketDetail, error) {
	lane, err := domain.ParseLane(input.Lane)
	if err != nil {
		return TicketDetail{}, err
	}
	return s.writer.change(ctx, id, func(ctx context.Context, repos Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		if isUnrefinedHandoff(*ticket, lane, actor) {
			return nil, domain.Invalidf("ticket %d has no repo; set repo_id or ask which repo before moving it to %s", ticket.ID, lane)
		}
		return moveTicket(ctx, repos, ticket, lane, placement{AfterID: input.AfterID}, actor, s.clock)
	})
}

// Comment adds a note to a ticket's timeline.
func (s *BoardService) Comment(ctx context.Context, actor domain.Actor, id int64, body string) (TicketDetail, error) {
	text, err := domain.RequireText("body", body, domain.MaxCommentLength)
	if err != nil {
		return TicketDetail{}, err
	}
	return s.writer.change(ctx, id, func(_ context.Context, _ Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		ticket.UpdatedAt = s.clock.Now()
		entry := domain.NewEntry(ticket.ID, domain.TimelineComment, actor, ticket.UpdatedAt).WithBody(text)
		return []domain.TimelineEntry{entry}, nil
	})
}

// SetSpec stores the refined specification an agent wrote.
func (s *BoardService) SetSpec(ctx context.Context, actor domain.Actor, id int64, spec string) (TicketDetail, error) {
	return s.writer.change(ctx, id, func(_ context.Context, _ Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		if err := ticket.SetSpec(spec, s.clock.Now()); err != nil {
			return nil, err
		}
		entry := domain.NewEntry(ticket.ID, domain.TimelineSpec, actor, ticket.UpdatedAt)
		return []domain.TimelineEntry{entry}, nil
	})
}

// Ask posts a round of questions and moves the ticket to Pending Answers, which
// hands the ball to the human.
func (s *BoardService) Ask(ctx context.Context, actor domain.Actor, id int64, input AskInput) (TicketDetail, error) {
	summary, err := domain.LimitText("summary", strings.TrimSpace(input.Summary), domain.MaxCommentLength)
	if err != nil {
		return TicketDetail{}, err
	}
	if len(input.Questions) == 0 || len(input.Questions) > domain.MaxQuestionsPerAsk {
		return TicketDetail{}, domain.Invalidf("ask between 1 and %d questions", domain.MaxQuestionsPerAsk)
	}
	return s.writer.change(ctx, id, func(ctx context.Context, repos Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		round, err := s.insertRound(ctx, repos, ticket.ID, input.Questions, actor)
		if err != nil {
			return nil, err
		}
		moved, err := moveTicket(ctx, repos, ticket, domain.LaneAnswers, atTop, actor, s.clock)
		if err != nil {
			return nil, err
		}
		asked := domain.NewEntry(ticket.ID, domain.TimelineQuestions, actor, ticket.UpdatedAt).
			WithBody(summary).With("round", round).With("count", len(input.Questions))
		return append([]domain.TimelineEntry{asked}, moved...), nil
	})
}

// Answer records a person's answers. When nothing is left open the ticket goes
// back to New, so the agent refines again with the answers in hand.
func (s *BoardService) Answer(ctx context.Context, actor domain.Actor, id int64, answers []domain.AnswerDraft) (TicketDetail, error) {
	if !actor.IsHuman() {
		return TicketDetail{}, domain.Forbiddenf("only a person can answer questions")
	}
	if len(answers) == 0 {
		return TicketDetail{}, domain.Invalidf("answers are required")
	}
	return s.writer.change(ctx, id, func(ctx context.Context, repos Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		round, remaining, err := s.applyAnswers(ctx, repos, ticket.ID, answers, actor)
		if err != nil {
			return nil, err
		}
		ticket.UpdatedAt = s.clock.Now()
		answered := domain.NewEntry(ticket.ID, domain.TimelineAnswers, actor, ticket.UpdatedAt).
			With("round", round).With("count", len(answers))
		entries := []domain.TimelineEntry{answered}
		if remaining > 0 || ticket.Lane != domain.LaneAnswers {
			return entries, nil
		}
		moved, err := moveTicket(ctx, repos, ticket, domain.LaneNew, atTop, domain.SystemActor, s.clock)
		return append(entries, moved...), err
	})
}

// Delete removes a ticket and its history.
func (s *BoardService) Delete(ctx context.Context, actor domain.Actor, id int64) error {
	if !actor.IsHuman() {
		return domain.Forbiddenf("only a person can delete tickets")
	}
	var ticket domain.Ticket
	err := s.store.Write(ctx, func(repos Repos) error {
		var err error
		if ticket, err = repos.Tickets().Get(ctx, id); err != nil {
			return err
		}
		return repos.Tickets().Delete(ctx, id)
	})
	if err != nil {
		return err
	}
	s.writer.sink.Publish(ctx, []Event{{Type: domain.TimelineDeleted, TicketID: id, Title: ticket.Title, Lane: ticket.Lane, Actor: actor}})
	return nil
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

// applyAnswers saves each answer and returns the latest round answered and how
// many questions remain open.
func (s *BoardService) applyAnswers(ctx context.Context, repos Repos, ticketID int64, answers []domain.AnswerDraft, actor domain.Actor) (int, int, error) {
	questions, err := repos.Questions().ForTicket(ctx, ticketID)
	if err != nil {
		return 0, 0, err
	}
	byID := make(map[int64]*domain.Question, len(questions))
	for i := range questions {
		byID[questions[i].ID] = &questions[i]
	}
	round := 0
	for _, answer := range answers {
		question, isKnown := byID[answer.QuestionID]
		if !isKnown {
			return 0, 0, domain.Invalidf("question %d does not belong to ticket %d", answer.QuestionID, ticketID)
		}
		if err := question.ApplyAnswer(answer, actor, s.clock.Now()); err != nil {
			return 0, 0, err
		}
		if err := repos.Questions().SaveAnswer(ctx, *question); err != nil {
			return 0, 0, err
		}
		round = max(round, question.Round)
	}
	return round, countOpen(questions), nil
}

// moveTicket places a ticket and returns the history entry for the move.
func moveTicket(ctx context.Context, repos Repos, ticket *domain.Ticket, lane domain.Lane, place placement, actor domain.Actor, clock Clock) ([]domain.TimelineEntry, error) {
	rank, err := rankFor(ctx, repos.Tickets(), lane, ticket.ID, place)
	if err != nil {
		return nil, err
	}
	from := ticket.Lane
	isLaneChange := ticket.MoveTo(lane, rank, clock.Now())
	return moveEntry(*ticket, from, actor, isLaneChange), nil
}

// isUnrefinedHandoff catches an agent queuing work it cannot build: nothing
// leaves refinement for the build lanes without a repo. People may still drag
// anything anywhere.
func isUnrefinedHandoff(ticket domain.Ticket, lane domain.Lane, actor domain.Actor) bool {
	isBuildLane := lane == domain.LaneReady || lane == domain.LaneInProgress
	return actor.IsAgent() && isBuildLane && ticket.RepoID == 0
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
	summaries := make([]TicketSummary, 0, len(tickets))
	for _, ticket := range tickets {
		summaries = append(summaries, TicketSummary{Ticket: ticket, OpenQuestions: openCounts[ticket.ID]})
	}
	return Board{Lanes: domain.Lanes(), Repos: codeRepos, Tickets: summaries}, nil
}
