// Package clocktest provides a controllable clock for tests.
package clocktest

import (
	"sync"
	"time"
)

// Clock is a fixed clock that moves only when told to. It is safe for
// concurrent use.
type Clock struct {
	mu  sync.Mutex
	now time.Time
}

// New returns a clock set to a fixed instant.
func New() *Clock {
	return &Clock{now: time.Date(2026, 1, 2, 9, 0, 0, 0, time.UTC)}
}

// Now implements clock.Clock.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward.
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
