package app

import (
	"context"
	"fmt"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// SplitResult is a split parent as it now stands and the children filed, in
// build order.
type SplitResult struct {
	Parent   TicketDetail
	ChildIDs []int64
}

// Split breaks a ticket that is too big for one half-day story into child
// stories, all in one transaction: if any child is refused, nothing changes.
// Each child carries its own spec, inherits the parent's repo and autonomous
// mode, links to the parent with split_from and waits for the children before
// it. A full split parks the parent as an epic that closes when every child is
// deployed; the epic's own unmet dependencies move to the first children. A
// narrowing split keeps the parent working on its narrowed core, and the
// children wait for it. Either way the parent's decision log records why and
// into what.
func (s *BoardService) Split(ctx context.Context, actor domain.Actor, id int64, draft domain.SplitDraft) (SplitResult, error) {
	if err := rejectHiddenSplitText(actor, draft); err != nil {
		return SplitResult{}, err
	}
	plan, err := domain.ParseSplit(draft)
	if err != nil {
		return SplitResult{}, err
	}
	var result SplitResult
	var events []Event
	err = s.store.Write(ctx, func(repos Repos) error {
		result, events, err = s.splitInTx(ctx, repos, actor, id, plan)
		return err
	})
	if err != nil {
		return SplitResult{}, err
	}
	s.writer.sink.Publish(ctx, events)
	return result, nil
}

func (s *BoardService) splitInTx(ctx context.Context, repos Repos, actor domain.Actor, id int64, plan domain.SplitPlan) (SplitResult, []Event, error) {
	now := s.clock.Now()
	parent, err := repos.Tickets().Get(ctx, id)
	if err != nil {
		return SplitResult{}, nil, err
	}
	if err := requireSplitter(ctx, repos, parent, actor, now); err != nil {
		return SplitResult{}, nil, err
	}
	if err := parent.RequireSplittable(plan); err != nil {
		return SplitResult{}, nil, err
	}
	repo, err := repoOf(ctx, repos, parent.RepoID)
	if err != nil {
		return SplitResult{}, nil, err
	}
	inherited, err := repos.Relations().WaitingOn(ctx, parent.ID)
	if err != nil {
		return SplitResult{}, nil, err
	}
	if !plan.IsNarrowing() {
		// Park the parent first, so it holds no in-flight slot while the
		// dependency checks for its children run.
		if err := repos.Tickets().Update(ctx, parkedAsEpic(parent, now)); err != nil {
			return SplitResult{}, nil, err
		}
	}
	var events []Event
	var parentEntries []domain.TimelineEntry
	childIDs := make([]int64, 0, len(plan.Children))
	for _, child := range plan.Children {
		dependsOn := splitDependencies(child, childIDs, parent.ID, inherited, plan.IsNarrowing())
		childID, entry, filed, err := s.insertSplitChild(ctx, repos, actor, parent, domain.GatesFor(repo), child.Draft, dependsOn)
		if err != nil {
			return SplitResult{}, nil, err
		}
		childIDs = append(childIDs, childID)
		parentEntries = append(parentEntries, entry)
		events = append(events, filed...)
	}
	decision := domain.DecisionFromSplit(parent, plan, childIDs, actor, now)
	if err := repos.Decisions().Insert(ctx, &decision); err != nil {
		return SplitResult{}, nil, err
	}
	parentEntries = append(parentEntries, domain.NewEntry(parent.ID, domain.TimelineDecision, actor, now).
		With("decision_id", decision.ID).With("weight", string(decision.Weight)).WithBody(decision.Summary))
	detail, parentEvents, err := s.writer.apply(ctx, repos, &parent, func(ctx context.Context, repos Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		changed, err := s.reshapeParent(ctx, repos, ticket, plan, actor)
		return append(changed, parentEntries...), err
	})
	if err != nil {
		return SplitResult{}, nil, err
	}
	return SplitResult{Parent: detail, ChildIDs: childIDs}, append(events, parentEvents...), nil
}

// reshapeParent parks a fully split parent as an epic, or narrows the spec
// of the parent that keeps building its core.
func (s *BoardService) reshapeParent(ctx context.Context, repos Repos, ticket *domain.Ticket, plan domain.SplitPlan, actor domain.Actor) ([]domain.TimelineEntry, error) {
	now := s.clock.Now()
	if plan.IsNarrowing() {
		if err := ticket.Narrow(plan.KeepSpec, now); err != nil {
			return nil, err
		}
		return []domain.TimelineEntry{domain.NewEntry(ticket.ID, domain.TimelineSpec, actor, now).
			With("revision", ticket.SpecRevision).With("split", true)}, nil
	}
	ticket.ClearBlockForDrop()
	return changeStage(ctx, repos, ticket, domain.StageEpic, actor, s.clock)
}

// parkedAsEpic is the parent as stored while its children are filed: in the
// epic stage, holding no slot. reshapeParent records the move properly.
func parkedAsEpic(parent domain.Ticket, now time.Time) domain.Ticket {
	parked := parent
	parked.MoveToStage(domain.StageEpic, parent.Rank, now)
	return parked
}

// splitDependencies lists what one child waits for: the earlier children it
// names, the parent when the parent keeps the core, and, for a first child
// of a full split, whatever the parent was still waiting for.
func splitDependencies(child domain.SplitChildPlan, earlierIDs []int64, parentID int64, inherited []int64, isNarrowing bool) []int64 {
	var dependsOn []int64
	for _, index := range child.After {
		dependsOn = append(dependsOn, earlierIDs[index])
	}
	switch {
	case isNarrowing:
		dependsOn = append(dependsOn, parentID)
	case len(child.After) == 0:
		dependsOn = append(dependsOn, inherited...)
	}
	return dependsOn
}

// insertSplitChild files one child, links it to its parent and returns its
// id, the line for the parent's timeline, and the events of what it recorded.
func (s *BoardService) insertSplitChild(ctx context.Context, repos Repos, actor domain.Actor, parent domain.Ticket, gates domain.Gates, draft domain.SplitChildDraft, dependsOn []int64) (int64, domain.TimelineEntry, []Event, error) {
	now := s.clock.Now()
	ticket, err := domain.NewSplitChild(draft, parent, gates, actor, now)
	if err != nil {
		return 0, domain.TimelineEntry{}, nil, err
	}
	ids, err := domain.ParseDependencies(dependsOn)
	if err != nil {
		return 0, domain.TimelineEntry{}, nil, err
	}
	if ticket.IsAutonomous {
		if err := requireFilingRoom(ctx, repos, actor, now); err != nil {
			return 0, domain.TimelineEntry{}, nil, err
		}
	}
	if ticket.Rank, err = rankFor(ctx, repos.Tickets(), ticket.Lane, 0, atBottom); err != nil {
		return 0, domain.TimelineEntry{}, nil, err
	}
	if err := repos.Tickets().Insert(ctx, &ticket); err != nil {
		return 0, domain.TimelineEntry{}, nil, err
	}
	if ticket.IsAutonomous {
		decision := domain.DecisionFromSplitAutonomy(ticket.ID, parent.ID, actor, now)
		if err := repos.Decisions().Insert(ctx, &decision); err != nil {
			return 0, domain.TimelineEntry{}, nil, err
		}
	}
	relation, err := domain.NewRelation(ticket.ID, parent.ID, domain.RelationSplitFrom, actor, now)
	if err != nil {
		return 0, domain.TimelineEntry{}, nil, err
	}
	if err := s.insertRelation(ctx, repos, relation); err != nil {
		return 0, domain.TimelineEntry{}, nil, err
	}
	entries := []domain.TimelineEntry{
		domain.NewEntry(ticket.ID, domain.TimelineCreated, actor, now).With("autonomous", ticket.IsAutonomous).With("split_from", parent.ID),
		domain.NewEntry(ticket.ID, domain.TimelineSpec, actor, now).With("revision", ticket.SpecRevision).With("submitted", true),
		relationEntry(ticket.ID, parent.ID, actor, now, "outgoing"),
	}
	detail, events, err := s.recordCreated(ctx, repos, actor, ticket, entries, ids)
	if err != nil {
		return 0, domain.TimelineEntry{}, nil, err
	}
	return detail.Ticket.ID, relationEntry(parent.ID, ticket.ID, actor, now, "incoming"), events, nil
}

// relationEntry is one end's timeline line for a split_from link.
func relationEntry(ticketID, otherID int64, actor domain.Actor, now time.Time, direction string) domain.TimelineEntry {
	return domain.NewEntry(ticketID, domain.TimelineRelation, actor, now).
		With("kind", string(domain.RelationSplitFrom)).With("ticket_id", otherID).With("direction", direction)
}

// requireSplitter lets a person, the agent holding the ticket's lease, the
// board manager on an autonomous ticket, or the agent that filed a ticket
// nobody has started yet split it. The last lets a delegate that filed an
// agreed piece of work break it down at once.
func requireSplitter(ctx context.Context, repos Repos, ticket domain.Ticket, actor domain.Actor, now time.Time) error {
	if requireLeaseForAgent(ticket, actor, now) == nil {
		return nil
	}
	if ticket.Stage == domain.StageReady && !ticket.IsLeased(now) {
		isCreator, err := isCreator(ctx, repos, ticket, actor)
		if err != nil || isCreator {
			return err
		}
	}
	return domain.Forbiddenf("claim ticket %d first: only a person, the agent holding its lease, the board manager or the agent that filed it can split it", ticket.ID)
}

// rejectHiddenSplitText refuses invisible characters and concealed markdown
// in everything an agent wrote for a split.
func rejectHiddenSplitText(actor domain.Actor, draft domain.SplitDraft) error {
	fields := map[string]string{"reason": draft.Reason, "keep_spec": draft.KeepSpec}
	for i, child := range draft.Children {
		fields[fmt.Sprintf("children[%d].title", i)] = child.Title
		fields[fmt.Sprintf("children[%d].body", i)] = child.Body
		fields[fmt.Sprintf("children[%d].spec", i)] = child.Spec
	}
	if err := rejectHiddenFromAgent(actor, fields); err != nil {
		return err
	}
	if actor.IsHuman() {
		return nil
	}
	for field, text := range fields {
		if err := domain.RejectConcealedMarkdown(field, text); err != nil {
			return err
		}
	}
	return nil
}

// closeFinishedEpic runs when child has just been deployed. If it was the last
// child of an epic still waiting for its children, the epic is deployed too,
// by the board. Closing goes through apply, so an epic that was itself a
// child closes its own parent in turn, and tickets that waited for the epic
// are released.
func (w ticketWriter) closeFinishedEpic(ctx context.Context, repos Repos, child domain.Ticket) ([]Event, error) {
	parentID, err := repos.Relations().SplitParentOf(ctx, child.ID)
	if err != nil || parentID == 0 {
		return nil, err
	}
	parent, err := repos.Tickets().Get(ctx, parentID)
	if err != nil || parent.Stage != domain.StageEpic {
		return nil, err
	}
	children, err := repos.Relations().SplitChildrenOf(ctx, parentID)
	if err != nil || !domain.EpicIsFinished(children) {
		return nil, err
	}
	_, events, err := w.apply(ctx, repos, &parent, func(ctx context.Context, repos Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		return changeStage(ctx, repos, ticket, domain.StageDeployed, domain.SystemActor, w.clock)
	})
	return events, err
}
