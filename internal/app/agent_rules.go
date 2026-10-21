package app

import (
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// requireLeaseForAgent lets an agent change a ticket's record only while it
// holds the ticket's lease, so an agent working one ticket cannot rewrite
// another's history. People are not restricted.
func requireLeaseForAgent(ticket domain.Ticket, actor domain.Actor, now time.Time) error {
	if !actor.IsAgent() {
		return nil
	}
	if !ticket.IsLeased(now) || ticket.Lease.Holder != actor.Name {
		return domain.Forbiddenf("claim ticket %d first: an agent may only change a ticket it holds the lease on", ticket.ID)
	}
	return nil
}

// requireLeaseOnEither lets an agent relate two tickets when it holds the
// lease on at least one of them.
func requireLeaseOnEither(first, second domain.Ticket, actor domain.Actor, now time.Time) error {
	if requireLeaseForAgent(first, actor, now) == nil || requireLeaseForAgent(second, actor, now) == nil {
		return nil
	}
	return domain.Forbiddenf("claim ticket %d or %d before linking them", first.ID, second.ID)
}

// rejectHiddenFromAgent refuses invisible characters in text an agent wrote.
func rejectHiddenFromAgent(actor domain.Actor, fields map[string]string) error {
	if actor.IsHuman() {
		return nil
	}
	for field, value := range fields {
		if err := domain.RejectHiddenText(field, value); err != nil {
			return err
		}
	}
	return nil
}
