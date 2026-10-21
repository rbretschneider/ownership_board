// Package clock supplies the current time behind an interface, so code that
// depends on time can be tested with a fixed one.
package clock

import "time"

// Clock returns the current time.
type Clock interface {
	Now() time.Time
}

// System is the real wall clock, in UTC.
type System struct{}

// Now implements Clock.
func (System) Now() time.Time {
	return time.Now().UTC()
}
