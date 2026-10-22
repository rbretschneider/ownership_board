package app

import "github.com/rbretschneider/ownership_board/internal/domain"

// Event announces a committed change to one ticket.
type Event struct {
	Cursor   int64
	Type     domain.TimelineType
	TicketID int64
	Title    string
	Lane     domain.Lane
	Stage    domain.Stage
	PRURL    string
	Actor    domain.Actor
	// Detail is a short human-readable line, such as a round summary.
	Detail string
}

func eventFor(ticket domain.Ticket, entry domain.TimelineEntry) Event {
	return Event{
		Cursor: entry.ID, Type: entry.Type, TicketID: ticket.ID, Title: ticket.Title,
		Lane: ticket.Lane, Stage: ticket.Stage, PRURL: ticket.PRURL, Actor: entry.Actor, Detail: entry.Body,
	}
}
