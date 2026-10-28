package app

import (
	"context"
	"slices"
	"strings"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// SaveTextInput is a spec or design an agent or person saves.
type SaveTextInput struct {
	Text string
	// IsSubmit hands the ticket on once the text is saved. Agents only.
	IsSubmit bool
}

// ReviewInput is a person's answer at a checkpoint.
type ReviewInput struct {
	Checkpoint string
	// Revision is the revision the person was shown.
	Revision int
	Comment  string
}

// SaveSpec stores a spec. The agent holding the lease writes it while refining
// and may submit it; a person may rewrite it at any time, as a new revision.
func (s *BoardService) SaveSpec(ctx context.Context, actor domain.Actor, id int64, input SaveTextInput) (TicketDetail, error) {
	return s.saveText(ctx, actor, id, input, domain.TimelineSpec, func(ticket *domain.Ticket, gates domain.Gates, open int) (domain.Stage, error) {
		if actor.IsHuman() {
			return "", ticket.EditSpecAsOwner(input.Text, s.clock.Now())
		}
		return ticket.SaveSpec(input.Text, input.IsSubmit, gates, open, s.clock.Now())
	})
}

// SaveDesign stores a design, with the same rules as SaveSpec in the design
// stage.
func (s *BoardService) SaveDesign(ctx context.Context, actor domain.Actor, id int64, input SaveTextInput) (TicketDetail, error) {
	return s.saveText(ctx, actor, id, input, domain.TimelineDesign, func(ticket *domain.Ticket, gates domain.Gates, open int) (domain.Stage, error) {
		if actor.IsHuman() {
			return "", ticket.EditDesignAsOwner(input.Text, s.clock.Now())
		}
		return ticket.SaveDesign(input.Text, input.IsSubmit, gates, open, s.clock.Now())
	})
}

type textSave func(ticket *domain.Ticket, gates domain.Gates, openQuestions int) (domain.Stage, error)

func (s *BoardService) saveText(ctx context.Context, actor domain.Actor, id int64, input SaveTextInput, kind domain.TimelineType, save textSave) (TicketDetail, error) {
	if actor.IsHuman() && input.IsSubmit {
		return TicketDetail{}, domain.Invalidf("people approve instead of submitting")
	}
	if err := rejectHiddenFromAgent(actor, map[string]string{string(kind): input.Text}); err != nil {
		return TicketDetail{}, err
	}
	if actor.IsAgent() {
		if err := domain.RejectConcealedMarkdown(string(kind), input.Text); err != nil {
			return TicketDetail{}, err
		}
	}
	return s.writer.change(ctx, actor, id, func(ctx context.Context, repos Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		gates, open, err := gatesAndOpen(ctx, repos, *ticket)
		if err != nil {
			return nil, err
		}
		next, err := save(ticket, gates, open)
		if err != nil {
			return nil, err
		}
		revision := ticket.SpecRevision
		if kind == domain.TimelineDesign {
			revision = ticket.DesignRevision
		}
		saved := domain.NewEntry(ticket.ID, kind, actor, ticket.UpdatedAt).With("revision", revision).With("submitted", next != "")
		if next == "" {
			return []domain.TimelineEntry{saved}, nil
		}
		moved, err := changeStage(ctx, repos, ticket, next, actor, s.clock)
		return append([]domain.TimelineEntry{saved}, moved...), err
	})
}

// Approve records the owner's approval of the revision they were shown and
// moves the ticket past the checkpoint.
func (s *BoardService) Approve(ctx context.Context, actor domain.Actor, id int64, input ReviewInput) (TicketDetail, error) {
	checkpoint, err := reviewCheckpoint(actor, input)
	if err != nil {
		return TicketDetail{}, err
	}
	return s.writer.change(ctx, actor, id, func(ctx context.Context, repos Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		next, err := ticket.Approve(checkpoint, input.Revision)
		if err != nil {
			return nil, err
		}
		now := s.clock.Now()
		decision := domain.DecisionFromCheckpoint(ticket.ID, checkpoint, input.Revision, actor, now)
		if err := repos.Decisions().Insert(ctx, &decision); err != nil {
			return nil, err
		}
		approved := domain.NewEntry(ticket.ID, domain.TimelineApproved, actor, now).
			With("checkpoint", string(checkpoint)).With("revision", input.Revision)
		moved, err := changeStage(ctx, repos, ticket, next, actor, s.clock)
		return append([]domain.TimelineEntry{approved}, moved...), err
	})
}

// RequestChanges sends a reviewed revision back to the agent with the owner's
// comment.
func (s *BoardService) RequestChanges(ctx context.Context, actor domain.Actor, id int64, input ReviewInput) (TicketDetail, error) {
	checkpoint, err := reviewCheckpoint(actor, input)
	if err != nil {
		return TicketDetail{}, err
	}
	comment, err := domain.RequireText("comment", input.Comment, domain.MaxCommentLength)
	if err != nil {
		return TicketDetail{}, err
	}
	return s.writer.change(ctx, actor, id, func(ctx context.Context, repos Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		next, err := ticket.RequestChanges(checkpoint, input.Revision)
		if err != nil {
			return nil, err
		}
		requested := domain.NewEntry(ticket.ID, domain.TimelineChangesRequested, actor, s.clock.Now()).
			WithBody(comment).With("checkpoint", string(checkpoint)).With("revision", input.Revision)
		moved, err := changeStage(ctx, repos, ticket, next, actor, s.clock)
		return append([]domain.TimelineEntry{requested}, moved...), err
	})
}

// Block stops agent work on a ticket until a person decides. The agent gives
// up its lease with it.
func (s *BoardService) Block(ctx context.Context, actor domain.Actor, id int64, reason string) (TicketDetail, error) {
	if err := rejectHiddenFromAgent(actor, map[string]string{"reason": reason}); err != nil {
		return TicketDetail{}, err
	}
	return s.writer.change(ctx, actor, id, func(_ context.Context, _ Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		if err := ticket.Block(reason, s.clock.Now()); err != nil {
			return nil, err
		}
		entry := domain.NewEntry(ticket.ID, domain.TimelineBlocked, actor, ticket.UpdatedAt).WithBody(ticket.BlockedReason)
		return []domain.TimelineEntry{entry}, nil
	})
}

// Unblock lets agents pick a blocked ticket up again, with an optional note
// for them.
func (s *BoardService) Unblock(ctx context.Context, actor domain.Actor, id int64, comment string) (TicketDetail, error) {
	if !actor.IsHuman() {
		return TicketDetail{}, domain.Forbiddenf("only a person can unblock a ticket")
	}
	note, err := domain.LimitText("comment", strings.TrimSpace(comment), domain.MaxCommentLength)
	if err != nil {
		return TicketDetail{}, err
	}
	return s.writer.change(ctx, actor, id, func(_ context.Context, _ Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		if err := ticket.Unblock(s.clock.Now()); err != nil {
			return nil, err
		}
		entry := domain.NewEntry(ticket.ID, domain.TimelineUnblocked, actor, ticket.UpdatedAt).WithBody(note)
		return []domain.TimelineEntry{entry}, nil
	})
}

func reviewCheckpoint(actor domain.Actor, input ReviewInput) (domain.Checkpoint, error) {
	if !actor.IsHuman() {
		return "", domain.Forbiddenf("only a person can resolve a checkpoint")
	}
	return domain.ParseCheckpoint(input.Checkpoint)
}

// gatesAndOpen reads, inside the caller's transaction, the checkpoints that
// apply to ticket and how many of its questions are open. When an agent chose
// the ticket's repo, both checkpoints apply until a person sets the repo: an
// agent must not pick its way past the owner by pointing the ticket at a repo
// whose checkpoints are off.
func gatesAndOpen(ctx context.Context, repos Repos, ticket domain.Ticket) (domain.Gates, int, error) {
	repo, err := repoOf(ctx, repos, ticket.RepoID)
	if err != nil {
		return domain.Gates{}, 0, err
	}
	questions, err := repos.Questions().ForTicket(ctx, ticket.ID)
	if err != nil {
		return domain.Gates{}, 0, err
	}
	timeline, err := repos.Timeline().ForTicket(ctx, ticket.ID)
	if err != nil {
		return domain.Gates{}, 0, err
	}
	gates := domain.GatesFor(repo)
	if repoChosenByAgent(timeline) {
		gates = domain.GatesFor(nil)
	}
	// An autonomous ticket's repo was agreed with the owner, so the agent
	// that filed it chose it in the owner's name.
	return ticket.Gates(gates), countOpen(questions), nil
}

// repoChosenByAgent reports whether the latest repo choice, at creation or in
// an edit, was an agent's.
func repoChosenByAgent(timeline []domain.TimelineEntry) bool {
	isAgent := false
	for _, entry := range timeline {
		switch {
		case entry.Type == domain.TimelineCreated:
			isAgent = entry.Actor.IsAgent()
		case entry.Type == domain.TimelineEdited && editedRepo(entry.Data["fields"]):
			isAgent = entry.Actor.IsAgent()
		}
	}
	return isAgent
}

// editedRepo reads an edit entry's field list as saved ([]string) or as
// loaded from the database ([]any).
func editedRepo(fields any) bool {
	switch list := fields.(type) {
	case []string:
		return slices.Contains(list, "repo")
	case []any:
		return slices.Contains(list, any("repo"))
	}
	return false
}

// changeStage moves a ticket to stage and returns the history entries for it.
// Within a lane the ticket keeps its place; into another lane it goes on top.
func changeStage(ctx context.Context, repos Repos, ticket *domain.Ticket, stage domain.Stage, actor domain.Actor, clock Clock) ([]domain.TimelineEntry, error) {
	rank := ticket.Rank
	if stage.Lane() != ticket.Lane {
		var err error
		if rank, err = rankFor(ctx, repos.Tickets(), stage.Lane(), ticket.ID, atTop); err != nil {
			return nil, err
		}
	}
	from, fromLane := ticket.Stage, ticket.Lane
	isLaneChange := ticket.MoveToStage(stage, rank, clock.Now())
	return stageEntries(*ticket, from, fromLane, actor, isLaneChange), nil
}

// stageEntries records a stage change and, when the lane changed too, the
// move between lanes.
func stageEntries(ticket domain.Ticket, from domain.Stage, fromLane domain.Lane, actor domain.Actor, isLaneChange bool) []domain.TimelineEntry {
	var entries []domain.TimelineEntry
	if from != ticket.Stage {
		entries = append(entries, domain.NewEntry(ticket.ID, domain.TimelineStage, actor, ticket.UpdatedAt).
			With("from", string(from)).With("to", string(ticket.Stage)))
	}
	return append(entries, moveEntry(ticket, fromLane, actor, isLaneChange)...)
}
