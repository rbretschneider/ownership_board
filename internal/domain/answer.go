package domain

import "time"

// Answer is a human's reply to a question.
type Answer struct {
	Choice     *int
	Text       string
	IsDeferred bool
	// IsBulk is set when the answer came from "Accept recommendations": the
	// person delegated rather than chose.
	IsBulk bool
	By     string
	At     time.Time
}
