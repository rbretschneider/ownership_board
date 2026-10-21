package app

import (
	"context"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// maxWorkLookupResults caps how many tickets a commit or PR lookup returns.
const maxWorkLookupResults = 20

// AddWorkLink records a PR or commit for a ticket. Adding one that is already
// there changes nothing.
func (s *BoardService) AddWorkLink(ctx context.Context, actor domain.Actor, id int64, draft domain.WorkLinkDraft) (TicketDetail, error) {
	if err := rejectHiddenFromAgent(actor, map[string]string{"url": draft.URL, "sha": draft.SHA}); err != nil {
		return TicketDetail{}, err
	}
	return s.writer.change(ctx, actor, id, func(ctx context.Context, repos Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		if err := requireLeaseForAgent(*ticket, actor, s.clock.Now()); err != nil {
			return nil, err
		}
		link, isNew, err := s.newWorkLink(ctx, repos, *ticket, draft, actor)
		if err != nil || !isNew {
			return nil, err
		}
		if err := repos.WorkLinks().Insert(ctx, &link); err != nil {
			return nil, err
		}
		ticket.UpdatedAt = link.AddedAt
		// The first PR linked becomes the ticket's PR, so the card and the
		// review notification have it without a separate update.
		if link.Kind == domain.WorkLinkPR && ticket.PRURL == "" {
			ticket.PRURL = link.URL
		}
		entry := domain.NewEntry(ticket.ID, domain.TimelineWorkLink, actor, link.AddedAt).
			With("kind", string(link.Kind)).With("url", link.URL).With("sha", link.SHA)
		return []domain.TimelineEntry{entry}, nil
	})
}

// RemoveWorkLink deletes a work link. Only people remove links.
func (s *BoardService) RemoveWorkLink(ctx context.Context, actor domain.Actor, id, linkID int64) (TicketDetail, error) {
	if !actor.IsHuman() {
		return TicketDetail{}, domain.Forbiddenf("only a person can remove work links")
	}
	return s.writer.change(ctx, actor, id, func(ctx context.Context, repos Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		link, err := repos.WorkLinks().Delete(ctx, ticket.ID, linkID)
		if err != nil {
			return nil, err
		}
		ticket.UpdatedAt = s.clock.Now()
		entry := domain.NewEntry(ticket.ID, domain.TimelineWorkLink, actor, ticket.UpdatedAt).
			With("kind", string(link.Kind)).With("removed", true)
		return []domain.TimelineEntry{entry}, nil
	})
}

// RecordDecision adds a decision, optionally replacing an earlier one.
func (s *BoardService) RecordDecision(ctx context.Context, actor domain.Actor, id int64, draft domain.DecisionDraft) (TicketDetail, error) {
	if err := rejectHiddenFromAgent(actor, decisionText(draft)); err != nil {
		return TicketDetail{}, err
	}
	return s.writer.change(ctx, actor, id, func(ctx context.Context, repos Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		now := s.clock.Now()
		if err := requireLeaseForAgent(*ticket, actor, now); err != nil {
			return nil, err
		}
		decision, err := domain.NewDecision(draft, ticket.ID, actor, now)
		if err != nil {
			return nil, err
		}
		if err := s.checkDecisionRoom(ctx, repos, ticket.ID, actor); err != nil {
			return nil, err
		}
		if err := s.insertDecision(ctx, repos, &decision, actor); err != nil {
			return nil, err
		}
		ticket.UpdatedAt = now
		entry := domain.NewEntry(ticket.ID, domain.TimelineDecision, actor, now).
			With("decision_id", decision.ID).With("weight", string(decision.Weight)).WithBody(decision.Summary)
		if decision.SupersedesID != nil {
			entry = entry.With("supersedes_id", *decision.SupersedesID)
		}
		return []domain.TimelineEntry{entry}, nil
	})
}

// RemoveDecision turns a decision into a content-free tombstone and, if it
// had replaced an earlier one, makes that one active again. Only people remove
// decisions.
func (s *BoardService) RemoveDecision(ctx context.Context, actor domain.Actor, id, decisionID int64) (TicketDetail, error) {
	if !actor.IsHuman() {
		return TicketDetail{}, domain.Forbiddenf("only a person can remove decisions")
	}
	return s.writer.change(ctx, actor, id, func(ctx context.Context, repos Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		decision, err := repos.Decisions().Get(ctx, ticket.ID, decisionID)
		if err != nil {
			return nil, err
		}
		if decision.IsRemoved {
			return nil, domain.Conflictf("decision %d is already removed", decisionID)
		}
		if err := repos.Decisions().Tombstone(ctx, decisionID); err != nil {
			return nil, err
		}
		if err := rejoinChain(ctx, repos.Decisions(), decision); err != nil {
			return nil, err
		}
		ticket.UpdatedAt = s.clock.Now()
		entry := domain.NewEntry(ticket.ID, domain.TimelineDecision, actor, ticket.UpdatedAt).
			With("decision_id", decisionID).With("removed", true)
		return []domain.TimelineEntry{entry}, nil
	})
}

// LinkTickets relates ticket id to another ticket. An agent needs the lease on
// one of the two.
func (s *BoardService) LinkTickets(ctx context.Context, actor domain.Actor, id int64, kind string, otherID int64) (TicketDetail, error) {
	relationKind, err := domain.ParseRelationKind(kind)
	if err != nil {
		return TicketDetail{}, err
	}
	var events []Event
	var detail TicketDetail
	err = s.store.Write(ctx, func(repos Repos) error {
		var err error
		detail, events, err = s.linkInTx(ctx, repos, actor, id, relationKind, otherID)
		return err
	})
	if err != nil {
		return TicketDetail{}, err
	}
	s.writer.sink.Publish(ctx, events)
	return detail, nil
}

// UnlinkTickets removes a relation in either direction. Only people unlink.
func (s *BoardService) UnlinkTickets(ctx context.Context, actor domain.Actor, id int64, kind string, otherID int64) (TicketDetail, error) {
	if !actor.IsHuman() {
		return TicketDetail{}, domain.Forbiddenf("only a person can remove ticket links")
	}
	relationKind, err := domain.ParseRelationKind(kind)
	if err != nil {
		return TicketDetail{}, err
	}
	return s.writer.change(ctx, actor, id, func(ctx context.Context, repos Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		relation, err := findRelation(ctx, repos, ticket.ID, otherID, relationKind)
		if err != nil {
			return nil, err
		}
		if err := repos.Relations().Delete(ctx, relation); err != nil {
			return nil, err
		}
		ticket.UpdatedAt = s.clock.Now()
		entry := domain.NewEntry(ticket.ID, domain.TimelineRelation, actor, ticket.UpdatedAt).
			With("kind", string(relationKind)).With("ticket_id", otherID).With("removed", true)
		return []domain.TimelineEntry{entry}, nil
	})
}

// FindTicketsForWork returns the tickets a commit or PR was recorded on. It
// answers only from recorded work links, never from commit trailers, which
// anyone can write in a public repo.
func (s *BoardService) FindTicketsForWork(ctx context.Context, sha, prURL string) ([]TicketSummary, error) {
	shaPrefix, normalizedURL, err := workLookupKeys(sha, prURL)
	if err != nil {
		return nil, err
	}
	var found []TicketSummary
	err = s.store.Read(ctx, func(repos Repos) error {
		ids, err := repos.WorkLinks().TicketsFor(ctx, shaPrefix, normalizedURL, maxWorkLookupResults)
		if err != nil {
			return err
		}
		for _, ticketID := range ids {
			ticket, err := repos.Tickets().Get(ctx, ticketID)
			if err != nil {
				return err
			}
			found = append(found, TicketSummary{Ticket: ticket})
		}
		return nil
	})
	return found, err
}

func (s *BoardService) linkInTx(ctx context.Context, repos Repos, actor domain.Actor, id int64, kind domain.RelationKind, otherID int64) (TicketDetail, []Event, error) {
	ticket, err := repos.Tickets().Get(ctx, id)
	if err != nil {
		return TicketDetail{}, nil, err
	}
	other, err := repos.Tickets().Get(ctx, otherID)
	if err != nil {
		return TicketDetail{}, nil, err
	}
	if err := requireLeaseOnEither(ticket, other, actor, s.clock.Now()); err != nil {
		return TicketDetail{}, nil, err
	}
	relation, err := domain.NewRelation(ticket.ID, other.ID, kind, actor, s.clock.Now())
	if err != nil {
		return TicketDetail{}, nil, err
	}
	if err := s.insertRelation(ctx, repos, relation); err != nil {
		return TicketDetail{}, nil, err
	}
	// The other ticket's history and live view learn about the link too.
	incoming := domain.NewEntry(other.ID, domain.TimelineRelation, actor, relation.AddedAt).
		With("kind", string(kind)).With("ticket_id", ticket.ID).With("direction", "incoming")
	otherEvents, err := record(ctx, repos, other, []domain.TimelineEntry{incoming})
	if err != nil {
		return TicketDetail{}, nil, err
	}
	detail, events, err := s.writer.apply(ctx, repos, &ticket, func(context.Context, Repos, *domain.Ticket) ([]domain.TimelineEntry, error) {
		ticket.UpdatedAt = relation.AddedAt
		entry := domain.NewEntry(ticket.ID, domain.TimelineRelation, actor, relation.AddedAt).
			With("kind", string(kind)).With("ticket_id", other.ID).With("direction", "outgoing")
		return []domain.TimelineEntry{entry}, nil
	})
	return detail, append(events, otherEvents...), err
}

func (s *BoardService) insertRelation(ctx context.Context, repos Repos, relation domain.Relation) error {
	outgoing, err := repos.Relations().CountOutgoing(ctx, relation.FromID)
	if err != nil {
		return err
	}
	if outgoing >= domain.MaxOutgoingRelations {
		return domain.Conflictf("ticket %d already links to %d tickets", relation.FromID, domain.MaxOutgoingRelations)
	}
	return repos.Relations().Insert(ctx, relation)
}

// newWorkLink validates a link and reports false when the ticket already has
// it.
func (s *BoardService) newWorkLink(ctx context.Context, repos Repos, ticket domain.Ticket, draft domain.WorkLinkDraft, actor domain.Actor) (domain.WorkLink, bool, error) {
	repo, err := repoOf(ctx, repos, ticket.RepoID)
	if err != nil {
		return domain.WorkLink{}, false, err
	}
	link, err := domain.NewWorkLink(draft, ticket, repo, actor, s.clock.Now())
	if err != nil {
		return domain.WorkLink{}, false, err
	}
	existing, err := repos.WorkLinks().ForTicket(ctx, ticket.ID)
	if err != nil || hasWorkLink(existing, link) {
		return domain.WorkLink{}, false, err
	}
	if !actor.IsHuman() {
		count, err := repos.WorkLinks().CountByAgents(ctx, ticket.ID)
		if err != nil {
			return domain.WorkLink{}, false, err
		}
		if count >= domain.MaxAgentWorkLinksPerTicket {
			return domain.WorkLink{}, false, domain.Conflictf("ticket %d already has %d agent work links", ticket.ID, count)
		}
	}
	return link, true, nil
}

func (s *BoardService) checkDecisionRoom(ctx context.Context, repos Repos, ticketID int64, actor domain.Actor) error {
	if actor.IsHuman() {
		return nil
	}
	count, err := repos.Decisions().CountByAgents(ctx, ticketID)
	if err != nil {
		return err
	}
	if count >= domain.MaxAgentDecisionsPerTicket {
		return domain.Conflictf("ticket %d already has %d agent decisions", ticketID, count)
	}
	return nil
}

// insertDecision saves a decision and links it to the one it replaces, in the
// caller's transaction.
func (s *BoardService) insertDecision(ctx context.Context, repos Repos, decision *domain.Decision, actor domain.Actor) error {
	if decision.SupersedesID == nil {
		return repos.Decisions().Insert(ctx, decision)
	}
	previous, err := repos.Decisions().Get(ctx, decision.TicketID, *decision.SupersedesID)
	if err != nil {
		return err
	}
	if err := previous.CanBeSupersededBy(actor); err != nil {
		return err
	}
	if err := repos.Decisions().Insert(ctx, decision); err != nil {
		return err
	}
	return repos.Decisions().SetSupersededBy(ctx, previous.ID, &decision.ID)
}

// recordAnswerDecisions logs each newly answered question as a decision. It
// never fails on content: the domain truncates rather than rejects.
func recordAnswerDecisions(ctx context.Context, repos Repos, answered []domain.Question, actor domain.Actor, now time.Time) error {
	for _, question := range answered {
		decision := domain.DecisionFromQuestion(question, actor, now)
		if err := repos.Decisions().Insert(ctx, &decision); err != nil {
			return err
		}
	}
	return nil
}

func findRelation(ctx context.Context, repos Repos, ticketID, otherID int64, kind domain.RelationKind) (domain.Relation, error) {
	relations, err := repos.Relations().ForTicket(ctx, ticketID)
	if err != nil {
		return domain.Relation{}, err
	}
	for _, relation := range relations {
		isPair := (relation.FromID == ticketID && relation.ToID == otherID) || (relation.FromID == otherID && relation.ToID == ticketID)
		if isPair && relation.Kind == kind {
			return relation, nil
		}
	}
	return domain.Relation{}, domain.NotFoundf("ticket %d has no %s link with ticket %d", ticketID, kind, otherID)
}

func hasWorkLink(existing []domain.WorkLink, link domain.WorkLink) bool {
	for _, candidate := range existing {
		if candidate.Kind != link.Kind {
			continue
		}
		if (link.Kind == domain.WorkLinkCommit && candidate.SHA == link.SHA) || (link.Kind == domain.WorkLinkPR && candidate.URL == link.URL) {
			return true
		}
	}
	return false
}

func workLookupKeys(sha, prURL string) (string, string, error) {
	if (sha == "") == (prURL == "") {
		return "", "", domain.Invalidf("give exactly one of sha or url")
	}
	if sha != "" {
		prefix, err := domain.ParseSHA(sha)
		return prefix, "", err
	}
	normalized, err := domain.NormalizeLinkURL(prURL)
	return "", normalized, err
}

func decisionText(draft domain.DecisionDraft) map[string]string {
	fields := map[string]string{"summary": draft.Summary, "rationale": draft.Rationale}
	for _, alternative := range draft.Alternatives {
		fields["alternative"] += alternative
	}
	return fields
}

// CreateRelated files a ticket already linked to another, such as a bug found
// while working #42 ("follow_up_of" #42). An agent needs the lease on the
// related ticket, so it can only file follow-ups for work it holds.
func (s *BoardService) CreateRelated(ctx context.Context, actor domain.Actor, draft domain.TicketDraft, kind string, relatedID int64) (TicketDetail, error) {
	if _, err := domain.ParseRelationKind(kind); err != nil {
		return TicketDetail{}, err
	}
	related, err := s.Ticket(ctx, relatedID)
	if err != nil {
		return TicketDetail{}, err
	}
	if err := requireLeaseForAgent(related.Ticket, actor, s.clock.Now()); err != nil {
		return TicketDetail{}, err
	}
	created, err := s.Create(ctx, actor, draft)
	if err != nil {
		return TicketDetail{}, err
	}
	return s.LinkTickets(ctx, actor, created.Ticket.ID, kind, relatedID)
}

// rejoinChain closes the gap a removed decision leaves in a replacement chain.
// Removing the newest decision makes its predecessor active again; removing
// one in the middle (B in A <- B <- C) links A and C directly, so only C stays
// active.
func rejoinChain(ctx context.Context, decisions DecisionRepository, removed domain.Decision) error {
	previous, next := removed.SupersedesID, removed.SupersededByID
	if previous != nil {
		if err := decisions.SetSupersededBy(ctx, *previous, next); err != nil {
			return err
		}
	}
	if next != nil {
		return decisions.SetSupersedes(ctx, *next, previous)
	}
	return nil
}
