// Package broker fans committed board events out to live listeners: browser
// event streams and agents waiting for work.
package broker

import (
	"context"
	"sync"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
)

// subscriberBuffer is how many events a slow subscriber may fall behind before
// further events are dropped for it. Events are doorbells, not a log, so a
// dropped one only means the subscriber refetches a little later.
const subscriberBuffer = 64

// Broker is an in-memory publish/subscribe hub. It is safe for concurrent use.
// It implements app.EventSink and app.Doorbell.
type Broker struct {
	mu          sync.Mutex
	subscribers map[*Subscription]struct{}
}

// New builds an empty broker.
func New() *Broker {
	return &Broker{subscribers: map[*Subscription]struct{}{}}
}

// Subscription receives every event published after it was created.
type Subscription struct {
	events chan app.Event
	broker *Broker
	once   sync.Once
}

// Subscribe starts a subscription. Close it when done.
func (b *Broker) Subscribe() *Subscription {
	subscription := &Subscription{events: make(chan app.Event, subscriberBuffer), broker: b}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers[subscription] = struct{}{}
	return subscription
}

// Listen implements app.Doorbell.
func (b *Broker) Listen() app.Listener {
	return b.Subscribe()
}

// Publish implements app.EventSink. It never blocks.
func (b *Broker) Publish(_ context.Context, events []app.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for subscription := range b.subscribers {
		for _, event := range events {
			select {
			case subscription.events <- event:
			default:
			}
		}
	}
}

// Events is the channel events arrive on.
func (s *Subscription) Events() <-chan app.Event {
	return s.events
}

// Wait implements app.Listener.
func (s *Subscription) Wait(ctx context.Context, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-s.events:
		return true
	case <-timer.C:
		return false
	case <-ctx.Done():
		return false
	}
}

// Close ends the subscription. It is safe to call more than once.
func (s *Subscription) Close() {
	s.once.Do(func() {
		s.broker.mu.Lock()
		defer s.broker.mu.Unlock()
		delete(s.broker.subscribers, s)
	})
}
