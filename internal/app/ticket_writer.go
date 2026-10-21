package app

import (
	"context"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// ticketChange mutates a loaded ticket inside a transaction and returns the
// timeline entries describing what it did.
type ticketChange func(ctx context.Context, repos Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error)

// ticketWriter is the shared write path for every use case that changes a
// ticket: load, mutate, save, record history, then announce after commit.
type ticketWriter struct {
	store Store
	sink  EventSink
	clock Clock
}

// change applies fn to ticket id and returns the ticket as it now stands. An
// agent may change only a ticket it holds the lease on: this is the one place
// every ticket write passes through, so no use case can forget the check.
func (w ticketWriter) change(ctx context.Context, actor domain.Actor, id int64, fn ticketChange) (TicketDetail, error) {
	var detail TicketDetail
	var events []Event
	err := w.store.Write(ctx, func(repos Repos) error {
		ticket, err := repos.Tickets().Get(ctx, id)
		if err != nil {
			return err
		}
		if err := requireLeaseForAgent(ticket, actor, w.clock.Now()); err != nil {
			return err
		}
		detail, events, err = w.apply(ctx, repos, &ticket, fn)
		return err
	})
	if err != nil {
		return TicketDetail{}, err
	}
	w.sink.Publish(ctx, events)
	return detail, nil
}

// apply runs fn on an already loaded ticket within the caller's transaction.
// The caller publishes the returned events once the transaction commits.
func (w ticketWriter) apply(ctx context.Context, repos Repos, ticket *domain.Ticket, fn ticketChange) (TicketDetail, []Event, error) {
	entries, err := fn(ctx, repos, ticket)
	if err != nil {
		return TicketDetail{}, nil, err
	}
	if err := repos.Tickets().Update(ctx, *ticket); err != nil {
		return TicketDetail{}, nil, err
	}
	events, err := record(ctx, repos, *ticket, entries)
	if err != nil {
		return TicketDetail{}, nil, err
	}
	detail, err := loadDetail(ctx, repos, *ticket)
	return detail, events, err
}

// record saves timeline entries and returns the matching events.
func record(ctx context.Context, repos Repos, ticket domain.Ticket, entries []domain.TimelineEntry) ([]Event, error) {
	events := make([]Event, 0, len(entries))
	for i := range entries {
		if err := repos.Timeline().Append(ctx, &entries[i]); err != nil {
			return nil, err
		}
		events = append(events, eventFor(ticket, entries[i]))
	}
	return events, nil
}

// loadDetail reads the questions and timeline that belong with a ticket.
func loadDetail(ctx context.Context, repos Repos, ticket domain.Ticket) (TicketDetail, error) {
	questions, err := repos.Questions().ForTicket(ctx, ticket.ID)
	if err != nil {
		return TicketDetail{}, err
	}
	timeline, err := repos.Timeline().ForTicket(ctx, ticket.ID)
	if err != nil {
		return TicketDetail{}, err
	}
	detail := TicketDetail{
		TicketSummary: TicketSummary{Ticket: ticket, OpenQuestions: countOpen(questions)},
		Questions:     questions, Timeline: timeline,
	}
	if detail.Repo, err = repoOf(ctx, repos, ticket.RepoID); err != nil {
		return TicketDetail{}, err
	}
	return detail, loadTrace(ctx, repos, &detail)
}

// loadTrace adds work links, decisions and related tickets to a detail.
func loadTrace(ctx context.Context, repos Repos, detail *TicketDetail) error {
	id := detail.Ticket.ID
	var err error
	if detail.WorkLinks, err = repos.WorkLinks().ForTicket(ctx, id); err != nil {
		return err
	}
	if detail.Decisions, err = repos.Decisions().ForTicket(ctx, id); err != nil {
		return err
	}
	relations, err := repos.Relations().ForTicket(ctx, id)
	if err != nil {
		return err
	}
	for _, relation := range relations {
		related, err := relatedTicket(ctx, repos, id, relation)
		if err != nil {
			return err
		}
		detail.Related = append(detail.Related, related)
	}
	return nil
}

func relatedTicket(ctx context.Context, repos Repos, ticketID int64, relation domain.Relation) (RelatedTicket, error) {
	isOutgoing := relation.FromID == ticketID
	otherID := relation.FromID
	if isOutgoing {
		otherID = relation.ToID
	}
	other, err := repos.Tickets().Get(ctx, otherID)
	if err != nil {
		return RelatedTicket{}, err
	}
	otherRepo, err := repoOf(ctx, repos, other.RepoID)
	ref := TicketRef{ID: other.ID, Title: other.Title, Lane: other.Lane, Repo: otherRepo}
	return RelatedTicket{Relation: relation, IsOutgoing: isOutgoing, Other: ref}, err
}

// repoOf returns the repo with id, or nil for "no repo".
func repoOf(ctx context.Context, repos Repos, repoID int64) (*domain.Repo, error) {
	if repoID == 0 {
		return nil, nil
	}
	repo, err := repos.CodeRepos().Get(ctx, repoID)
	if err != nil {
		return nil, err
	}
	return &repo, nil
}

// moveEntry records a lane change, or nothing when the lane did not change.
func moveEntry(ticket domain.Ticket, from domain.Lane, actor domain.Actor, isLaneChange bool) []domain.TimelineEntry {
	if !isLaneChange {
		return nil
	}
	entry := domain.NewEntry(ticket.ID, domain.TimelineMoved, actor, ticket.UpdatedAt).
		With("from", string(from)).
		With("to", string(ticket.Lane))
	return []domain.TimelineEntry{entry}
}

func countOpen(questions []domain.Question) int {
	open := 0
	for i := range questions {
		if questions[i].IsOpen() {
			open++
		}
	}
	return open
}
