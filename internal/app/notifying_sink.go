package app

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Notification timing.
const (
	// notifyTimeout bounds one push so a slow ntfy server never piles up work.
	notifyTimeout = 10 * time.Second
	// notifyDedupeWindow stops an agent looping on one ticket from flooding the
	// owner's phone with the same push.
	notifyDedupeWindow = 10 * time.Minute
)

// NotifyingSink sends a notification for each event the policy selects. It
// pushes in the background so a slow notifier never delays a request.
type NotifyingSink struct {
	policy   NotificationPolicy
	notifier Notifier
	logger   *slog.Logger
	clock    Clock

	mu       sync.Mutex
	lastSent map[string]time.Time
}

// NewNotifyingSink wires the sink to its dependencies.
func NewNotifyingSink(policy NotificationPolicy, notifier Notifier, logger *slog.Logger, clock Clock) *NotifyingSink {
	return &NotifyingSink{policy: policy, notifier: notifier, logger: logger, clock: clock, lastSent: map[string]time.Time{}}
}

// Publish implements EventSink.
func (s *NotifyingSink) Publish(_ context.Context, events []Event) {
	for _, event := range events {
		notification, isWanted := s.policy.Evaluate(event)
		if isWanted && s.isFirstInWindow(notification.Title) {
			go s.send(event.TicketID, notification)
		}
	}
}

// isFirstInWindow reports whether a push with this title (ticket and reason)
// has not gone out within the dedupe window, and records it if so.
func (s *NotifyingSink) isFirstInWindow(key string) bool {
	now := s.clock.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for sentKey, at := range s.lastSent {
		if now.Sub(at) >= notifyDedupeWindow {
			delete(s.lastSent, sentKey)
		}
	}
	if _, isRecent := s.lastSent[key]; isRecent {
		return false
	}
	s.lastSent[key] = now
	return true
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
