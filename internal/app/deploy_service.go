package app

import (
	"context"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// DeployOutcome describes the result of an agent's deployment attempt.
type DeployOutcome string

const (
	DeployDeployed DeployOutcome = "deployed"
	DeployFailed   DeployOutcome = "failed"
)

// RequestDeploy queues deployment only on a person's explicit request.
func (s *BoardService) RequestDeploy(ctx context.Context, actor domain.Actor, id int64) (TicketDetail, error) {
	if !actor.IsHuman() {
		return TicketDetail{}, domain.Forbiddenf("only a person can request deployment")
	}
	return s.writer.change(ctx, actor, id, func(ctx context.Context, repos Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		repo, err := repoOf(ctx, repos, ticket.RepoID)
		if err != nil {
			return nil, err
		}
		if err := ticket.CheckDeployRequest(repo); err != nil {
			return nil, err
		}
		decision, err := domain.NewDecision(domain.DecisionDraft{Summary: "Deploy requested by " + actor.Name}, id, actor, s.clock.Now())
		if err != nil {
			return nil, err
		}
		if err := repos.Decisions().Insert(ctx, &decision); err != nil {
			return nil, err
		}
		entry := domain.NewEntry(id, domain.TimelineDecision, actor, decision.At).With("decision_id", decision.ID).With("weight", string(decision.Weight)).WithBody(decision.Summary)
		moved, err := changeStage(ctx, repos, ticket, domain.StageDeploying, actor, s.clock)
		return append([]domain.TimelineEntry{entry}, moved...), err
	})
}

// ReportDeploy completes a leased deployment with the worker's runtime evidence.
func (s *BoardService) ReportDeploy(ctx context.Context, actor domain.Actor, id int64, outcome DeployOutcome, summary string) (TicketDetail, error) {
	if !actor.IsAgent() {
		return TicketDetail{}, domain.Forbiddenf("only an agent can report deployment")
	}
	if outcome != DeployDeployed && outcome != DeployFailed {
		return TicketDetail{}, domain.Invalidf("outcome must be deployed or failed")
	}
	text, err := deploySummary(actor, summary)
	if err != nil {
		return TicketDetail{}, err
	}
	return s.writer.change(ctx, actor, id, func(ctx context.Context, repos Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		if ticket.Stage != domain.StageDeploying || ticket.IsBlocked() {
			return nil, domain.Conflictf("ticket %d is not awaiting a deployment report", id)
		}
		if outcome == DeployFailed {
			return s.failDeploy(ctx, repos, ticket, actor, text)
		}
		comment := domain.NewEntry(id, domain.TimelineComment, actor, s.clock.Now()).WithBody(text)
		moved, err := changeStage(ctx, repos, ticket, domain.StageDeployed, actor, s.clock)
		return append([]domain.TimelineEntry{comment}, moved...), err
	})
}

func deploySummary(actor domain.Actor, summary string) (string, error) {
	if err := rejectHiddenFromAgent(actor, map[string]string{"summary": summary}); err != nil {
		return "", err
	}
	return domain.RequireText("summary", summary, domain.MaxBlockedReasonLength)
}

// failDeploy is transaction-local so reports and ordinary blocking stay atomic.
func (s *BoardService) failDeploy(ctx context.Context, repos Repos, ticket *domain.Ticket, actor domain.Actor, summary string) ([]domain.TimelineEntry, error) {
	text, err := deploySummary(actor, summary)
	if err != nil {
		return nil, err
	}
	if err := ticket.Block(text, s.clock.Now()); err != nil {
		return nil, err
	}
	entries := []domain.TimelineEntry{
		domain.NewEntry(ticket.ID, domain.TimelineComment, actor, s.clock.Now()).WithBody(text),
		domain.NewEntry(ticket.ID, domain.TimelineBlocked, actor, s.clock.Now()).WithBody(ticket.BlockedReason),
	}
	moved, err := changeStage(ctx, repos, ticket, domain.StageDone, actor, s.clock)
	return append(entries, moved...), err
}
