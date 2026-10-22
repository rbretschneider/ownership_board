package app

import (
	"context"
	"strings"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// PullRequestState is what an agent found when it checked a ticket's PR.
type PullRequestState string

// The states an agent may report.
const (
	PullRequestMerged PullRequestState = "merged"
	PullRequestClosed PullRequestState = "closed"
	PullRequestOpen   PullRequestState = "open"
)

// ReportPullRequest lets an agent close the loop on a ticket in review: a
// merged PR moves it to Merged, a PR closed without merging sends it back to
// building, blocked, so a person decides what happens next. It is the one
// agent write that needs no lease, because review tickets are the owner's and
// cannot be claimed; it is narrow on purpose: it only ever acts on a ticket in
// review that has a PR, and only moves it along those two edges. An open PR
// changes nothing.
func (s *BoardService) ReportPullRequest(ctx context.Context, actor domain.Actor, id int64, state PullRequestState, note string) (TicketDetail, error) {
	isChange, err := checkReportedState(actor, state)
	if err != nil {
		return TicketDetail{}, err
	}
	text, err := domain.LimitText("note", strings.TrimSpace(note), domain.MaxCommentLength)
	if err != nil {
		return TicketDetail{}, err
	}
	if err := rejectHiddenFromAgent(actor, map[string]string{"note": text}); err != nil {
		return TicketDetail{}, err
	}
	var detail TicketDetail
	var events []Event
	err = s.store.Write(ctx, func(repos Repos) error {
		ticket, err := repos.Tickets().Get(ctx, id)
		if err != nil {
			return err
		}
		if ticket.Stage != domain.StagePR || ticket.PRURL == "" {
			return domain.Conflictf("ticket %d is not in review with a pull request", id)
		}
		if !isChange {
			detail, err = loadDetail(ctx, repos, ticket)
			return err
		}
		detail, events, err = s.writer.apply(ctx, repos, &ticket, s.reviewOutcome(actor, state, text))
		return err
	})
	if err != nil {
		return TicketDetail{}, err
	}
	s.writer.sink.Publish(ctx, events)
	return detail, nil
}

func (s *BoardService) reviewOutcome(actor domain.Actor, state PullRequestState, note string) ticketChange {
	return func(ctx context.Context, repos Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		body := "Pull request " + string(state) + "."
		if note != "" {
			body += "\n\n" + note
		}
		comment := domain.NewEntry(ticket.ID, domain.TimelineComment, actor, s.clock.Now()).WithBody(body)
		if state == PullRequestMerged {
			moved, err := changeStage(ctx, repos, ticket, domain.StageDone, actor, s.clock)
			return append([]domain.TimelineEntry{comment}, moved...), err
		}
		rank, err := rankFor(ctx, repos.Tickets(), domain.LaneInProgress, ticket.ID, atTop)
		if err != nil {
			return nil, err
		}
		from, fromLane := ticket.Stage, ticket.Lane
		ticket.PullRequestClosed(rank, s.clock.Now())
		entries := append([]domain.TimelineEntry{comment}, stageEntries(*ticket, from, fromLane, actor, true)...)
		blocked := domain.NewEntry(ticket.ID, domain.TimelineBlocked, domain.SystemActor, ticket.UpdatedAt).WithBody(ticket.BlockedReason)
		return append(entries, blocked), nil
	}
}

// checkReportedState validates what the agent found; false means nothing
// changes.
func checkReportedState(actor domain.Actor, state PullRequestState) (bool, error) {
	if !actor.IsAgent() {
		return false, domain.Forbiddenf("people move review tickets by dragging them")
	}
	switch state {
	case PullRequestMerged, PullRequestClosed:
		return true, nil
	case PullRequestOpen:
		return false, nil
	default:
		return false, domain.Invalidf("state must be merged, closed or open")
	}
}
