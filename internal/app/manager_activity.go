package app

import (
	"context"
	"regexp"
	"slices"
	"sync"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// managerActivityTTL ends a "working" report the loop never closed, such as
// after the manager's container restarted mid-session.
const managerActivityTTL = 3 * time.Hour

// maxActivityTickets bounds the tickets one session may name.
const maxActivityTickets = 20

var activityHostPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

// ManagerActivityInput is what the manager loop reports around a session.
type ManagerActivityInput struct {
	State     string
	Kind      string
	TicketIDs []int64
	Host      string
	StartedAt *time.Time
}

// managerActivity keeps the manager's latest report in memory: it is live
// state, and a restart rightly forgets it.
type managerActivity struct {
	mu     sync.Mutex
	latest domain.ManagerActivity
}

// ReportManagerActivity records that the manager started or ended a session.
// Only the board manager token reports it.
func (s *BoardService) ReportManagerActivity(_ context.Context, actor domain.Actor, input ManagerActivityInput) (domain.ManagerActivity, error) {
	if !actor.IsBoardManager() {
		return domain.ManagerActivity{}, domain.Forbiddenf("only the board manager token reports manager activity")
	}
	now := s.clock.Now()
	activity := domain.ManagerActivity{State: domain.ManagerActivityState(input.State), UpdatedAt: now}
	switch activity.State {
	case domain.ManagerIdle:
	case domain.ManagerWorking:
		activity.Kind = domain.ManagerActivityKind(input.Kind)
		switch activity.Kind {
		case domain.ManagerActivityFlag, domain.ManagerActivityReview, domain.ManagerActivityRepair:
		default:
			return domain.ManagerActivity{}, domain.Invalidf("kind must be flag, review or repair")
		}
		if len(input.TicketIDs) > maxActivityTickets {
			return domain.ManagerActivity{}, domain.Invalidf("at most %d ticket_ids", maxActivityTickets)
		}
		for _, id := range input.TicketIDs {
			if id <= 0 {
				return domain.ManagerActivity{}, domain.Invalidf("ticket_ids must be positive")
			}
			if !slices.Contains(activity.TicketIDs, id) {
				activity.TicketIDs = append(activity.TicketIDs, id)
			}
		}
		if input.Host != "" && !activityHostPattern.MatchString(input.Host) {
			return domain.ManagerActivity{}, domain.Invalidf("host must be 1-64 letters, digits, dots, underscores or hyphens")
		}
		activity.Host = input.Host
		activity.StartedAt = now
		if input.StartedAt != nil && !input.StartedAt.After(now.Add(time.Minute)) && now.Sub(*input.StartedAt) < managerActivityTTL {
			activity.StartedAt = *input.StartedAt
		}
	default:
		return domain.ManagerActivity{}, domain.Invalidf("state must be working or idle")
	}
	s.activity.mu.Lock()
	s.activity.latest = activity
	s.activity.mu.Unlock()
	return activity, nil
}

// managerActivityNow is the latest report, idle when there is none or a
// working report has gone stale.
func (s *BoardService) managerActivityNow(now time.Time) domain.ManagerActivity {
	s.activity.mu.Lock()
	latest := s.activity.latest
	s.activity.mu.Unlock()
	if latest.State != domain.ManagerWorking || now.Sub(latest.UpdatedAt) > managerActivityTTL {
		return domain.ManagerActivity{State: domain.ManagerIdle, UpdatedAt: latest.UpdatedAt}
	}
	return latest
}
