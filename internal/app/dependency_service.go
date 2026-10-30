package app

import (
	"context"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// A depends_on link (A depends_on B) keeps A from being claimed until B is
// merged. The rules below keep that gate trustworthy: only a person, the
// agent holding A's lease, or the agent that filed A may add one; only a
// person or the agent that added it may remove one; both tickets are in the
// same repo; and the links never form a cycle.

// addDependencies links a ticket being created to every ticket in ids, in the
// caller's transaction, so a refusal leaves no ticket behind. It returns the
// lines for the new ticket's own timeline, for the caller to record, and the
// events of the lines it recorded on the other tickets.
func (s *BoardService) addDependencies(ctx context.Context, repos Repos, actor domain.Actor, ticket domain.Ticket, ids []int64) ([]domain.TimelineEntry, []Event, error) {
	var entries []domain.TimelineEntry
	var events []Event
	for _, id := range ids {
		prerequisite, err := repos.Tickets().Get(ctx, id)
		if domain.KindOf(err) == domain.KindNotFound {
			return nil, nil, domain.Invalidf("depends_on: ticket %d does not exist", id)
		}
		if err != nil {
			return nil, nil, err
		}
		entry, recorded, err := s.insertDependency(ctx, repos, actor, ticket, prerequisite)
		if err != nil {
			return nil, nil, err
		}
		entries = append(entries, entry)
		events = append(events, recorded...)
	}
	return entries, events, nil
}

// linkDependencyInTx adds "ticket depends_on otherID" for link_tickets. The
// permission check runs before the other ticket is even loaded, so a caller
// that may not add the link learns nothing about the other ticket.
func (s *BoardService) linkDependencyInTx(ctx context.Context, repos Repos, actor domain.Actor, ticket domain.Ticket, otherID int64) (TicketDetail, []Event, error) {
	if err := requireDependencyWriter(ctx, repos, ticket, actor, s.clock.Now()); err != nil {
		return TicketDetail{}, nil, err
	}
	prerequisite, err := repos.Tickets().Get(ctx, otherID)
	if err != nil {
		return TicketDetail{}, nil, err
	}
	entry, otherEvents, err := s.insertDependency(ctx, repos, actor, ticket, prerequisite)
	if err != nil {
		return TicketDetail{}, nil, err
	}
	detail, events, err := s.writer.apply(ctx, repos, &ticket, func(context.Context, Repos, *domain.Ticket) ([]domain.TimelineEntry, error) {
		ticket.UpdatedAt = entry.At
		return []domain.TimelineEntry{entry}, nil
	})
	return detail, append(events, otherEvents...), err
}

// insertDependency checks and stores "dependant depends_on prerequisite". It
// records the incoming line on the prerequisite and returns the outgoing line
// for the dependant, which the caller records with the dependant's other
// entries. Every check reads inside the caller's write transaction, so two
// concurrent links cannot together close a cycle.
func (s *BoardService) insertDependency(ctx context.Context, repos Repos, actor domain.Actor, dependant, prerequisite domain.Ticket) (domain.TimelineEntry, []Event, error) {
	if err := requireSameRepo(dependant, prerequisite); err != nil {
		return domain.TimelineEntry{}, nil, err
	}
	now := s.clock.Now()
	relation, err := domain.NewRelation(dependant.ID, prerequisite.ID, domain.RelationDependsOn, actor, now)
	if err != nil {
		return domain.TimelineEntry{}, nil, err
	}
	edges, err := repos.Relations().DependencyEdges(ctx)
	if err != nil {
		return domain.TimelineEntry{}, nil, err
	}
	if len(edges[dependant.ID]) >= domain.MaxDependencies {
		return domain.TimelineEntry{}, nil, domain.Conflictf("ticket %d already depends on %d tickets", dependant.ID, domain.MaxDependencies)
	}
	if cycle := domain.DependencyCycle(edges, dependant.ID, prerequisite.ID); cycle != nil {
		return domain.TimelineEntry{}, nil, domain.Conflictf("depends_on would make a cycle: %s", domain.FormatCyclePath(cycle))
	}
	if err := s.insertRelation(ctx, repos, relation); err != nil {
		return domain.TimelineEntry{}, nil, err
	}
	incoming := domain.NewEntry(prerequisite.ID, domain.TimelineRelation, actor, now).
		With("kind", string(domain.RelationDependsOn)).With("ticket_id", dependant.ID).With("direction", "incoming")
	events, err := record(ctx, repos, prerequisite, []domain.TimelineEntry{incoming})
	if err != nil {
		return domain.TimelineEntry{}, nil, err
	}
	outgoing := domain.NewEntry(dependant.ID, domain.TimelineRelation, actor, now).
		With("kind", string(domain.RelationDependsOn)).With("ticket_id", prerequisite.ID).With("direction", "outgoing")
	return outgoing, events, nil
}

// requireSameRepo keeps dependencies within one repo. The message names only
// the ticket id, never the other repo.
func requireSameRepo(dependant, prerequisite domain.Ticket) error {
	if dependant.RepoID == 0 {
		return domain.Invalidf("depends_on: ticket %d has no repo; dependencies are same repo only", dependant.ID)
	}
	if prerequisite.RepoID != dependant.RepoID {
		return domain.Invalidf("depends_on: ticket %d is in another repo; dependencies are same repo only", prerequisite.ID)
	}
	return nil
}

// requireDependencyWriter lets a person, the agent holding the dependant's
// lease, or the agent that filed the dependant add a dependency to it. A
// lease on the prerequisite alone is not enough: an agent must not be able
// to park tickets it has nothing to do with.
func requireDependencyWriter(ctx context.Context, repos Repos, dependant domain.Ticket, actor domain.Actor, now time.Time) error {
	if !actor.IsAgent() || requireLeaseForAgent(dependant, actor, now) == nil {
		return nil
	}
	isCreator, err := isCreator(ctx, repos, dependant, actor)
	if err != nil || isCreator {
		return err
	}
	return domain.Forbiddenf("claim ticket %d first: only a person, the agent holding its lease or the agent that filed it can add its dependencies", dependant.ID)
}

// isCreator reports whether the agent filed the ticket. Tickets record their
// creator by name, so this relies on agent token names being unique and
// never reused (revoking a token keeps its row and name), and on a token
// never sharing a person's name; the created entry must also be an agent's,
// so a person with the same display name never counts.
func isCreator(ctx context.Context, repos Repos, ticket domain.Ticket, actor domain.Actor) (bool, error) {
	if !actor.IsAgent() || ticket.CreatedBy != actor.Name {
		return false, nil
	}
	timeline, err := repos.Timeline().ForTicket(ctx, ticket.ID)
	if err != nil {
		return false, err
	}
	for _, entry := range timeline {
		if entry.Type == domain.TimelineCreated {
			return entry.Actor.IsAgent() && entry.Actor.Name == actor.Name, nil
		}
	}
	return false, nil
}

// mayUnlink lets a person remove any link, and an agent only a depends_on
// link its own token added, from either end and with no lease needed.
func mayUnlink(actor domain.Actor, relation domain.Relation) error {
	if actor.IsHuman() {
		return nil
	}
	isOwnDependency := relation.Kind == domain.RelationDependsOn &&
		relation.AddedBy.Kind == domain.ActorAgent && relation.AddedBy.ID == actor.ID
	if actor.IsAgent() && isOwnDependency {
		return nil
	}
	return domain.Forbiddenf("only a person, or the agent that added it, can remove this link")
}

// requireDependenciesFollowRepo refuses a repo change that would leave a
// dependency between two repos, or a dependency on a ticket with no repo.
func requireDependenciesFollowRepo(ctx context.Context, repos Repos, ticket domain.Ticket) error {
	relations, err := repos.Relations().ForTicket(ctx, ticket.ID)
	if err != nil {
		return err
	}
	for _, relation := range relations {
		if relation.Kind != domain.RelationDependsOn {
			continue
		}
		if ticket.RepoID == 0 {
			return domain.Conflictf("ticket %d has dependencies; remove them before clearing its repo", ticket.ID)
		}
		otherID := relation.ToID
		if otherID == ticket.ID {
			otherID = relation.FromID
		}
		other, err := repos.Tickets().Get(ctx, otherID)
		if err != nil {
			return err
		}
		if other.RepoID != ticket.RepoID {
			return domain.Conflictf("ticket %d has a dependency with ticket %d in its current repo; remove it before changing the repo", ticket.ID, other.ID)
		}
	}
	return nil
}

// releaseDependants runs when merged has just reached Merged. Each ticket
// that depended on it and now waits for nothing gets a line saying so,
// attributed to whoever merged it; its event also wakes waiting claims.
func releaseDependants(ctx context.Context, repos Repos, merged domain.Ticket, mergedBy domain.Actor, now time.Time) ([]Event, error) {
	relations, err := repos.Relations().ForTicket(ctx, merged.ID)
	if err != nil {
		return nil, err
	}
	var events []Event
	for _, relation := range relations {
		if relation.Kind != domain.RelationDependsOn || relation.ToID != merged.ID {
			continue
		}
		dependant, err := repos.Tickets().Get(ctx, relation.FromID)
		if err != nil {
			return nil, err
		}
		if dependant.Stage == domain.StageDone {
			continue
		}
		waiting, err := repos.Relations().WaitingOn(ctx, dependant.ID)
		if err != nil {
			return nil, err
		}
		if len(waiting) > 0 {
			continue
		}
		entry := domain.NewEntry(dependant.ID, domain.TimelineDependenciesMet, mergedBy, now).
			With("ticket_id", merged.ID).With("merged_by_kind", string(mergedBy.Kind))
		recorded, err := record(ctx, repos, dependant, []domain.TimelineEntry{entry})
		if err != nil {
			return nil, err
		}
		events = append(events, recorded...)
	}
	return events, nil
}

// mergedBy is the actor of the stage entry that moved a ticket to Merged, or
// the board itself when there is none.
func mergedBy(entries []domain.TimelineEntry) domain.Actor {
	for _, entry := range entries {
		if entry.Type == domain.TimelineStage && entry.Data["to"] == string(domain.StageDone) {
			return entry.Actor
		}
	}
	return domain.SystemActor
}
