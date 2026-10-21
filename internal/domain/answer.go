package domain

import "time"

// Answer is a human's reply to a question.
type Answer struct {
	Choice     *int
	Text       string
	IsDeferred bool
	By         string
	At         time.Time
}
