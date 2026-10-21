package logging

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/clock"
)

// Alert settings.
const (
	alertDedupWindow = 5 * time.Minute
	alertTimeout     = 10 * time.Second
	alertService     = "ownership_board"
)

// AlertHandler pushes error-level log records to the operator, sending the
// same message at most once per dedup window. It is safe for concurrent use.
type AlertHandler struct {
	notifier app.Notifier
	clock    clock.Clock

	mu       sync.Mutex
	lastSent map[string]time.Time
}

// NewAlertHandler builds the alerter.
func NewAlertHandler(notifier app.Notifier, clk clock.Clock) *AlertHandler {
	return &AlertHandler{notifier: notifier, clock: clk, lastSent: map[string]time.Time{}}
}

// Wrap returns a handler that logs through next and alerts on errors.
func (a *AlertHandler) Wrap(next slog.Handler) slog.Handler {
	return alertingHandler{next: next, alerts: a}
}

func (a *AlertHandler) alert(record slog.Record) {
	if !a.isDue(record.Message) {
		return
	}
	notification := app.Notification{
		Title: alertService + " error", Message: record.Message, Priority: app.PriorityHigh, Tags: []string{"rotating_light"},
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), alertTimeout)
		defer cancel()
		// A failed alert cannot be logged as an error without alerting again,
		// and the record it describes is already in the log.
		_ = a.notifier.Notify(ctx, notification)
	}()
}

func (a *AlertHandler) isDue(message string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.clock.Now()
	for key, sent := range a.lastSent {
		if now.Sub(sent) > alertDedupWindow {
			delete(a.lastSent, key)
		}
	}
	if _, isRecent := a.lastSent[message]; isRecent {
		return false
	}
	a.lastSent[message] = now
	return true
}

type alertingHandler struct {
	next   slog.Handler
	alerts *AlertHandler
}

func (h alertingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h alertingHandler) Handle(ctx context.Context, record slog.Record) error {
	if record.Level >= slog.LevelError {
		h.alerts.alert(record)
	}
	return h.next.Handle(ctx, record)
}

func (h alertingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return alertingHandler{next: h.next.WithAttrs(attrs), alerts: h.alerts}
}

func (h alertingHandler) WithGroup(name string) slog.Handler {
	return alertingHandler{next: h.next.WithGroup(name), alerts: h.alerts}
}
