package app

import (
	"fmt"
	"strings"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// NotificationRule turns an event into a notification when the event matters
// to the human. Add a rule to notify on something new; the policy is closed
// for modification.
type NotificationRule interface {
	Notification(event Event, links Links) (Notification, bool)
}

// Links builds URLs back into the board.
type Links interface {
	Ticket(id int64) string
}

// NotificationPolicy decides which events reach the human.
type NotificationPolicy struct {
	rules []NotificationRule
	links Links
}

// NewNotificationPolicy builds a policy from rules, checked in order.
func NewNotificationPolicy(links Links, rules ...NotificationRule) NotificationPolicy {
	return NotificationPolicy{rules: rules, links: links}
}

// DefaultNotificationRules notify when the ball lands with the human.
func DefaultNotificationRules() []NotificationRule {
	return []NotificationRule{questionsAsked{}, readyForReview{}, reachedDone{}}
}

// Evaluate returns the notification for event, if any. A person's own actions
// never notify them.
func (p NotificationPolicy) Evaluate(event Event) (Notification, bool) {
	if event.Actor.IsHuman() {
		return Notification{}, false
	}
	for _, rule := range p.rules {
		if notification, isMatch := rule.Notification(event, p.links); isMatch {
			return notification, true
		}
	}
	return Notification{}, false
}

type questionsAsked struct{}

func (questionsAsked) Notification(event Event, links Links) (Notification, bool) {
	if event.Type != domain.TimelineQuestions {
		return Notification{}, false
	}
	message := firstNonEmpty(event.Detail, "The agent has questions before it can continue.")
	return Notification{
		Title: ticketTitle(event, "needs answers"), Message: message, Priority: PriorityHigh,
		ClickURL: links.Ticket(event.TicketID), Tags: []string{"question"},
	}, true
}

type readyForReview struct{}

func (readyForReview) Notification(event Event, links Links) (Notification, bool) {
	if !isMoveInto(event, domain.LaneReview) {
		return Notification{}, false
	}
	notification := Notification{
		Title: ticketTitle(event, "ready for review"), Message: event.Title, Priority: PriorityDefault,
		ClickURL: links.Ticket(event.TicketID), Tags: []string{"eyes"},
	}
	if event.PRURL != "" {
		notification.Actions = []NotificationAction{{Label: "Open PR", URL: event.PRURL}}
	}
	return notification, true
}

type reachedDone struct{}

func (reachedDone) Notification(event Event, links Links) (Notification, bool) {
	if !isMoveInto(event, domain.LaneDone) {
		return Notification{}, false
	}
	return Notification{
		Title: ticketTitle(event, "done"), Message: event.Title, Priority: PriorityLow,
		ClickURL: links.Ticket(event.TicketID), Tags: []string{"white_check_mark"},
	}, true
}

func isMoveInto(event Event, lane domain.Lane) bool {
	return event.Type == domain.TimelineMoved && event.Lane == lane
}

func ticketTitle(event Event, what string) string {
	return fmt.Sprintf("#%d %s", event.TicketID, what)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
