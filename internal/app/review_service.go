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
// merged PR moves it to Done, a PR closed without merging sends it back to In
// Progress. It is the one agent write that needs no lease, because review
// tickets are the owner's and cannot be claimed; it is narrow on purpose: it
// only ever acts on a ticket in review that has a PR, and only moves it along
// those two edges. An open PR changes nothing.
func (s *BoardService) ReportPullRequest(ctx context.Context, actor domain.Actor, id int64, state PullRequestState, note string) (TicketDetail, error) {
	target, err := reviewTarget(actor, state)
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
		if ticket.Lane != domain.LaneReview || ticket.PRURL == "" {
			return domain.Conflictf("ticket %d is not in review with a pull request", id)
		}
		if target == "" {
			detail, err = loadDetail(ctx, repos, ticket)
			return err
		}
		detail, events, err = s.writer.apply(ctx, repos, &ticket, s.reviewOutcome(actor, state, target, text))
		return err
	})
	if err != nil {
		return TicketDetail{}, err
	}
	s.writer.sink.Publish(ctx, events)
	return detail, nil
}

func (s *BoardService) reviewOutcome(actor domain.Actor, state PullRequestState, target domain.Lane, note string) ticketChange {
	return func(ctx context.Context, repos Repos, ticket *domain.Ticket) ([]domain.TimelineEntry, error) {
		moved, err := moveTicket(ctx, repos, ticket, target, atTop, actor, s.clock)
		if err != nil {
			return nil, err
		}
		body := "Pull request " + string(state) + "."
		if note != "" {
			body += "\n\n" + note
		}
		comment := domain.NewEntry(ticket.ID, domain.TimelineComment, actor, ticket.UpdatedAt).WithBody(body)
		return append([]domain.TimelineEntry{comment}, moved...), nil
	}
}

// reviewTarget maps a reported state to the lane it leads to; "" means stay.
func reviewTarget(actor domain.Actor, state PullRequestState) (domain.Lane, error) {
	if !actor.IsAgent() {
		return "", domain.Forbiddenf("people move review tickets by dragging them")
	}
	switch state {
	case PullRequestMerged:
		return domain.LaneDone, nil
	case PullRequestClosed:
		return domain.LaneInProgress, nil
	case PullRequestOpen:
		return "", nil
	default:
		return "", domain.Invalidf("state must be merged, closed or open")
	}
}
