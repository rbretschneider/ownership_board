package httpapi

import (
	"github.com/rbretschneider/ownership_board/internal/auth"
	"net/netip"
	"sync"
	"time"
)

type pairWindow struct {
	start    time.Time
	attempts int
}
type buildHostPairLimiter struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[string]pairWindow
}

func newBuildHostPairLimiter(now func() time.Time) *buildHostPairLimiter {
	return &buildHostPairLimiter{now: now, entries: map[string]pairWindow{}}
}
func (l *buildHostPairLimiter) allow(ip string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if addr, e := netip.ParseAddr(ip); e == nil {
		ip = addr.Unmap().String()
	}
	now := l.now()
	for k, v := range l.entries {
		if !now.Before(v.start.Add(time.Minute)) {
			delete(l.entries, k)
		}
	}
	v, ok := l.entries[ip]
	if !ok {
		if len(l.entries) >= 4096 {
			return &auth.ThrottledError{RetryAfter: time.Minute}
		}
		v.start = now
	}
	if v.attempts >= 5 {
		return &auth.ThrottledError{RetryAfter: v.start.Add(time.Minute).Sub(now)}
	}
	v.attempts++
	l.entries[ip] = v
	return nil
}
