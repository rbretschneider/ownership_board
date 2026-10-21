package app

import (
	"context"
	"log/slog"
	"time"
)

// notifyTimeout bounds one push so a slow ntfy server never piles up work.
const notifyTimeout = 10 * time.Second

// NotifyingSink sends a notification for each event the policy selects. It
// pushes in the background so a slow notifier never delays a request.
type NotifyingSink struct {
	policy   NotificationPolicy
	notifier Notifier
	logger   *slog.Logger
}

// NewNotifyingSink wires the sink to its dependencies.
func NewNotifyingSink(policy NotificationPolicy, notifier Notifier, logger *slog.Logger) *NotifyingSink {
	return &NotifyingSink{policy: policy, notifier: notifier, logger: logger}
}

// Publish implements EventSink.
func (s *NotifyingSink) Publish(_ context.Context, events []Event) {
	for _, event := range events {
		notification, isWanted := s.policy.Evaluate(event)
		if isWanted {
			go s.send(event.TicketID, notification)
		}
	}
}

func (s *NotifyingSink) send(ticketID int64, notification Notification) {
	ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
	defer cancel()
	if err := s.notifier.Notify(ctx, notification); err != nil {
		s.logger.Warn("notification failed", "ticket", ticketID, "title", notification.Title, "error", err)
	}
}

// FanoutSink publishes every event to each of its sinks in order.
type FanoutSink []EventSink

// Publish implements EventSink.
func (f FanoutSink) Publish(ctx context.Context, events []Event) {
	if len(events) == 0 {
		return
	}
	for _, sink := range f {
		sink.Publish(ctx, events)
	}
}
