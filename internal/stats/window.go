// Package stats computes the product owner's delivery metrics from tickets
// and their recorded history. It is pure: callers load the data, and the same
// input always gives the same report.
package stats

import (
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// Window limits and defaults.
const (
	DefaultWindowDays = 30
	MaxWindowDays     = 365
	// maxOffsetMinutes is the widest real UTC offset, UTC+14.
	maxOffsetMinutes = 14 * 60
	dayFormat        = "2006-01-02"
)

// recentPeriod is what the summary strip calls "recent".
const recentPeriod = 24 * time.Hour

// Window is the period a report covers: whole local days, ending now.
type Window struct {
	Now      time.Time
	Days     int
	Location *time.Location
}

// NewWindow validates a window of days local days ending at now, in a zone
// offsetMinutes east of UTC (the browser's offset, so days match the owner's).
func NewWindow(now time.Time, days, offsetMinutes int) (Window, error) {
	if days < 1 || days > MaxWindowDays {
		return Window{}, domain.Invalidf("days must be between 1 and %d", MaxWindowDays)
	}
	if offsetMinutes < -maxOffsetMinutes || offsetMinutes > maxOffsetMinutes {
		return Window{}, domain.Invalidf("tz_offset must be between %d and %d minutes", -maxOffsetMinutes, maxOffsetMinutes)
	}
	return Window{Now: now, Days: days, Location: time.FixedZone("", offsetMinutes*60)}, nil
}

// Start is local midnight on the window's first day.
func (w Window) Start() time.Time {
	local := w.Now.In(w.Location)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, w.Location)
	return today.AddDate(0, 0, 1-w.Days)
}

// Contains reports whether at falls within the window.
func (w Window) Contains(at time.Time) bool {
	return !at.Before(w.Start()) && !at.After(w.Now)
}

// IsRecent reports whether at falls within the last 24 hours.
func (w Window) IsRecent(at time.Time) bool {
	return !at.Before(w.Now.Add(-recentPeriod)) && !at.After(w.Now)
}

// Day is the local calendar day of at, as YYYY-MM-DD.
func (w Window) Day(at time.Time) string {
	return at.In(w.Location).Format(dayFormat)
}

// DayList lists every local day in the window, oldest first.
func (w Window) DayList() []string {
	days := make([]string, w.Days)
	start := w.Start()
	for i := range days {
		days[i] = start.AddDate(0, 0, i).Format(dayFormat)
	}
	return days
}
