package auth

import (
	"sync"
	"time"

	"github.com/rbretschneider/ownership_board/internal/clock"
)

// Throttle settings: a few free attempts, then exponential backoff.
const (
	throttleFreeAttempts = 5
	throttleBaseDelay    = time.Second
	throttleMaxDelay     = 15 * time.Minute
	// throttleForgetAfter drops an address's record after a quiet spell, which
	// also bounds the map's size.
	throttleForgetAfter = time.Hour
)

// Throttle slows repeated failed sign-ins per client address. It is keyed on
// the address, not the account, so a guesser cannot lock a person out of their
// own account. It is safe for concurrent use.
type Throttle struct {
	mu       sync.Mutex
	clock    clock.Clock
	failures map[string]failureRecord
}

type failureRecord struct {
	count int
	last  time.Time
}

// NewThrottle builds an empty throttle.
func NewThrottle(clk clock.Clock) *Throttle {
	return &Throttle{clock: clk, failures: map[string]failureRecord{}}
}

// Check returns how long key must still wait, or zero when it may try now.
func (t *Throttle) Check(key string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	record, isKnown := t.failures[key]
	if !isKnown || record.count < throttleFreeAttempts {
		return 0
	}
	wait := record.last.Add(backoff(record.count)).Sub(t.clock.Now())
	return max(wait, 0)
}

// Fail records a failed attempt for key.
func (t *Throttle) Fail(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.clock.Now()
	t.forgetStale(now)
	record := t.failures[key]
	t.failures[key] = failureRecord{count: record.count + 1, last: now}
}

// Succeed clears key's record.
func (t *Throttle) Succeed(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.failures, key)
}

// forgetStale must be called with mu held.
func (t *Throttle) forgetStale(now time.Time) {
	for key, record := range t.failures {
		if now.Sub(record.last) > throttleForgetAfter {
			delete(t.failures, key)
		}
	}
}

func backoff(count int) time.Duration {
	exponent := count - throttleFreeAttempts
	if exponent > 20 {
		return throttleMaxDelay
	}
	return min(throttleBaseDelay<<exponent, throttleMaxDelay)
}
