package connections

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"

	"github.com/rbretschneider/ownership_board/internal/clock"
	"github.com/rbretschneider/ownership_board/internal/domain"
	"github.com/rbretschneider/ownership_board/internal/reposource"
)

// Device-flow limits.
const (
	maxActiveFlows   = 8
	startCooldown    = 5 * time.Second
	flowIDBytes      = 32
	confirmLifetime  = 10 * time.Minute
	errFlowNotFound  = "that sign-in does not exist or has finished; start again"
	errTooManyFlows  = "too many GitHub sign-ins are in progress; try again in a few minutes"
	errStartedRecent = "a sign-in was just started; wait a few seconds"
)

type flowState int

const (
	flowWaiting flowState = iota
	flowAwaitingConfirmation
)

// flow is one device sign-in, held only in memory. The device code never
// leaves the server.
type flow struct {
	id        string
	userID    int64
	device    reposource.DeviceCode
	interval  time.Duration
	nextPoll  time.Time
	expiresAt time.Time
	isPolling bool
	state     flowState
	tokens    reposource.TokenSet
	user      reposource.GitHubUser
}

// flows holds pending device sign-ins. It is safe for concurrent use; every
// field below mu is guarded by it, and no network call happens while it is
// held.
type flows struct {
	clock clock.Clock

	mu        sync.Mutex
	byID      map[string]*flow
	lastStart map[int64]time.Time
}

func newFlows(clk clock.Clock) *flows {
	return &flows{clock: clk, byID: map[string]*flow{}, lastStart: map[int64]time.Time{}}
}

// reserve checks a person may start a flow now and records the attempt. It
// cancels that person's earlier flow, since only one may be active.
func (f *flows) reserve(userID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.clock.Now()
	f.sweep(now)
	if last, hasStarted := f.lastStart[userID]; hasStarted && now.Sub(last) < startCooldown {
		return domain.Conflictf(errStartedRecent)
	}
	f.removeUser(userID)
	if len(f.byID) >= maxActiveFlows {
		return domain.Conflictf(errTooManyFlows)
	}
	f.lastStart[userID] = now
	return nil
}

// add stores a started flow and returns its id.
func (f *flows) add(userID int64, device reposource.DeviceCode) (*flow, error) {
	id, err := newFlowID()
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removeUser(userID)
	started := &flow{
		id: id, userID: userID, device: device, interval: device.Interval,
		nextPoll: f.clock.Now().Add(device.Interval), expiresAt: device.ExpiresAt,
	}
	f.byID[id] = started
	return started, nil
}

// beginPoll claims the right to call GitHub for a flow. It reports false when
// the interval has not passed or another poll is in flight; the caller then
// answers "pending" without calling GitHub.
func (f *flows) beginPoll(userID int64, id string) (flow, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	current, err := f.owned(userID, id)
	if err != nil {
		return flow{}, false, err
	}
	now := f.clock.Now()
	if current.state != flowWaiting || current.isPolling || now.Before(current.nextPoll) {
		return *current, false, nil
	}
	current.isPolling = true
	return *current, true, nil
}

// endPoll records a poll's outcome. A terminal outcome deletes the flow.
func (f *flows) endPoll(id string, result reposource.PollResult, user reposource.GitHubUser) {
	f.mu.Lock()
	defer f.mu.Unlock()
	current, isKnown := f.byID[id]
	if !isKnown {
		return
	}
	current.isPolling = false
	current.interval = result.Interval
	current.nextPoll = f.clock.Now().Add(result.Interval)
	switch result.Outcome {
	case reposource.PollConnected:
		current.state, current.tokens, current.user = flowAwaitingConfirmation, result.Tokens, user
		current.expiresAt = f.clock.Now().Add(confirmLifetime)
	case reposource.PollExpired, reposource.PollDenied:
		delete(f.byID, id)
	}
}

// abortPoll releases a poll that failed without an answer from GitHub.
func (f *flows) abortPoll(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if current, isKnown := f.byID[id]; isKnown {
		current.isPolling = false
		current.nextPoll = f.clock.Now().Add(current.interval)
	}
}

// take removes and returns a flow that is waiting for confirmation.
func (f *flows) take(userID int64, id string) (flow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	current, err := f.owned(userID, id)
	if err != nil {
		return flow{}, err
	}
	if current.state != flowAwaitingConfirmation {
		return flow{}, domain.Conflictf("GitHub has not confirmed this sign-in yet")
	}
	delete(f.byID, id)
	return *current, nil
}

// cancel removes a person's flow. Unknown ids are ignored.
func (f *flows) cancel(userID int64, id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if current, err := f.owned(userID, id); err == nil {
		delete(f.byID, current.id)
	}
}

// owned returns the flow when it exists, has not expired and belongs to
// userID. Unknown and foreign ids get the same error, so ids cannot be probed.
// Callers hold mu.
func (f *flows) owned(userID int64, id string) (*flow, error) {
	current, isKnown := f.byID[id]
	if !isKnown || current.userID != userID || !f.clock.Now().Before(current.expiresAt) {
		return nil, domain.NotFoundf(errFlowNotFound)
	}
	return current, nil
}

// sweep drops expired flows. Callers hold mu.
func (f *flows) sweep(now time.Time) {
	for id, current := range f.byID {
		if !now.Before(current.expiresAt) {
			delete(f.byID, id)
		}
	}
	for userID, at := range f.lastStart {
		if now.Sub(at) > startCooldown {
			delete(f.lastStart, userID)
		}
	}
}

// removeUser drops a person's flows. Callers hold mu.
func (f *flows) removeUser(userID int64) {
	for id, current := range f.byID {
		if current.userID == userID {
			delete(f.byID, id)
		}
	}
}

func newFlowID() (string, error) {
	raw := make([]byte, flowIDBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
