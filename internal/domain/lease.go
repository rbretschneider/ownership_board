package domain

import "time"

// Lease marks a ticket as being worked by one agent until it expires.
type Lease struct {
	Holder    string    `json:"holder"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Lease durations, in minutes.
const (
	DefaultLeaseMinutes = 30
	MaxLeaseMinutes     = 240
)

// IsActive reports whether the lease still holds at now.
func (l *Lease) IsActive(now time.Time) bool {
	return l != nil && now.Before(l.ExpiresAt)
}

// LeaseDuration clamps a requested lease length, treating zero as the default.
func LeaseDuration(minutes int) (time.Duration, error) {
	if minutes == 0 {
		minutes = DefaultLeaseMinutes
	}
	if minutes < 1 || minutes > MaxLeaseMinutes {
		return 0, Invalidf("lease_minutes must be between 1 and %d", MaxLeaseMinutes)
	}
	return time.Duration(minutes) * time.Minute, nil
}
