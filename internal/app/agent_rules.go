package app

import (
	"context"
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

// maxHealthCheckFollowUps bounds agent-generated work from a single review.
const maxHealthCheckFollowUps = 3

// requireHealthCheckFollowUpRoom runs in the same write transaction as linking.
func requireHealthCheckFollowUpRoom(ctx context.Context, repos Repos, parent domain.Ticket, childID int64, actor domain.Actor) error {
	if !actor.IsAgent() || !parent.IsHealthCheck {
		return nil
	}
	relations, err := repos.Relations().ForTicket(ctx, parent.ID)
	if err != nil {
		return err
	}
	count := 0
	for _, relation := range relations {
		if relation.Kind != domain.RelationFollowUpOf || relation.ToID != parent.ID {
			continue
		}
		if relation.FromID == childID {
			// Let insertion retain the normal duplicate-link error.
			return nil
		}
		count++
	}
	if count >= maxHealthCheckFollowUps {
		return domain.Invalidf("health check %d allows at most %d follow-up tickets", parent.ID, maxHealthCheckFollowUps)
	}
	return nil
}
