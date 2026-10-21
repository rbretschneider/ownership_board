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
}

// change applies fn to ticket id and returns the ticket as it now stands.
func (w ticketWriter) change(ctx context.Context, id int64, fn ticketChange) (TicketDetail, error) {
	var detail TicketDetail
	var events []Event
	err := w.store.Write(ctx, func(repos Repos) error {
		ticket, err := repos.Tickets().Get(ctx, id)
		if err != nil {
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
	summary := TicketSummary{Ticket: ticket, OpenQuestions: countOpen(questions)}
	return TicketDetail{TicketSummary: summary, Questions: questions, Timeline: timeline}, nil
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
