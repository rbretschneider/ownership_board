package app

import (
	"fmt"

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

// DefaultNotificationRules notify when the ball lands with the human. Every
// message is fixed board text: nothing an agent wrote reaches a phone's lock
// screen, and no notification offers an action beyond opening the ticket.
func DefaultNotificationRules() []NotificationRule {
	return []NotificationRule{
		ownerRule{matches: isType(domain.TimelineQuestions), what: "needs answers", message: "The agent has questions before it can continue.", priority: PriorityHigh, tag: "question"},
		ownerRule{matches: isStageEntry(domain.StageSpecReview), what: "spec to approve", message: "A spec is ready for your approval.", priority: PriorityHigh, tag: "memo"},
		ownerRule{matches: isStageEntry(domain.StageDesignReview), what: "design to approve", message: "A design is ready for your approval.", priority: PriorityHigh, tag: "triangular_ruler"},
		ownerRule{matches: isType(domain.TimelineBlocked), what: "blocked", message: "Work stopped and needs your decision.", priority: PriorityHigh, tag: "warning"},
		ownerRule{matches: isStageEntry(domain.StagePR), what: "ready for review", message: "A pull request is ready for your review.", priority: PriorityDefault, tag: "eyes"},
		ownerRule{matches: isStageEntry(domain.StageDone), what: "merged", message: "The pull request was merged.", priority: PriorityLow, tag: "white_check_mark"},
	}
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

// ownerRule sends one fixed message when its event matches.
type ownerRule struct {
	matches  func(Event) bool
	what     string
	message  string
	priority Priority
	tag      string
}

func (r ownerRule) Notification(event Event, links Links) (Notification, bool) {
	if !r.matches(event) {
		return Notification{}, false
	}
	return Notification{
		Title: fmt.Sprintf("#%d %s", event.TicketID, r.what), Message: r.message, Priority: r.priority,
		ClickURL: links.Ticket(event.TicketID), Tags: []string{r.tag},
	}, true
}

func isType(kind domain.TimelineType) func(Event) bool {
	return func(event Event) bool { return event.Type == kind }
}

func isStageEntry(stage domain.Stage) func(Event) bool {
	return func(event Event) bool { return event.Type == domain.TimelineStage && event.Stage == stage }
}
