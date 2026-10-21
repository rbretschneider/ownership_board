package httpapi

import (
	"fmt"
	"sync"
	"time"
)

// Agent write limits: a runaway or hijacked agent loop cannot flood the board.
const (
	agentWritesPerWindow = 120
	agentWriteWindow     = time.Minute
)

// writeLimiter counts write requests per agent token in fixed one-minute
// windows. It is safe for concurrent use.
type writeLimiter struct {
	now func() time.Time

	mu      sync.Mutex
	windows map[int64]writeWindow
}

type writeWindow struct {
	start time.Time
	count int
}

// rateLimitedError means an agent token sent too many writes.
type rateLimitedError struct {
	retryAfter time.Duration
}

func (e *rateLimitedError) Error() string {
	return fmt.Sprintf("too many write requests from this agent token; wait %s", e.retryAfter.Round(time.Second))
}

func newWriteLimiter(now func() time.Time) *writeLimiter {
	return &writeLimiter{now: now, windows: map[int64]writeWindow{}}
}

// allow records one write for tokenID, or returns why it is refused.
func (l *writeLimiter) allow(tokenID int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	window := l.windows[tokenID]
	if now.Sub(window.start) >= agentWriteWindow {
		l.forgetExpired(now)
		window = writeWindow{start: now}
	}
	if window.count >= agentWritesPerWindow {
		return &rateLimitedError{retryAfter: window.start.Add(agentWriteWindow).Sub(now)}
	}
	window.count++
	l.windows[tokenID] = window
	return nil
}

// forgetExpired drops finished windows; callers hold mu.
func (l *writeLimiter) forgetExpired(now time.Time) {
	for id, window := range l.windows {
		if now.Sub(window.start) >= agentWriteWindow {
			delete(l.windows, id)
		}
	}
}
