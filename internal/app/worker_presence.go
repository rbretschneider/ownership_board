package app

import (
	"sync"
	"time"
)

// maxPresenceEntries bounds the presence registry. Worker identities are
// credential-scoped, so a misbehaving client could otherwise grow it forever.
const maxPresenceEntries = 256

// WorkerPresence remembers when each worker identity last called the board.
// It lives in memory: after a restart every polling worker reappears within
// one poll interval, so nothing needs persisting.
type WorkerPresence struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

// NewWorkerPresence returns an empty registry.
func NewWorkerPresence() *WorkerPresence {
	return &WorkerPresence{seen: map[string]time.Time{}}
}

// Mark records that the worker identity called the board at at. A nil
// registry ignores the call, so services built without one keep working.
func (p *WorkerPresence) Mark(name string, at time.Time) {
	if p == nil || name == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, known := p.seen[name]; !known && len(p.seen) >= maxPresenceEntries {
		oldest, oldestAt := "", time.Time{}
		for candidate, when := range p.seen {
			if oldest == "" || when.Before(oldestAt) {
				oldest, oldestAt = candidate, when
			}
		}
		delete(p.seen, oldest)
	}
	if at.After(p.seen[name]) {
		p.seen[name] = at
	}
}

// Snapshot copies the registry.
func (p *WorkerPresence) Snapshot() map[string]time.Time {
	out := map[string]time.Time{}
	if p == nil {
		return out
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for name, at := range p.seen {
		out[name] = at
	}
	return out
}
