package auth

import (
	"errors"
	"fmt"
	"time"
)

// ErrUnauthorized means the credentials are missing, wrong, or expired. The
// message is deliberately vague so it reveals nothing to a guesser.
var ErrUnauthorized = errors.New("not signed in or the credentials are invalid")

// ErrNotLinked means an SSO identity signed in that no account has linked.
var ErrNotLinked = errors.New("no account is linked to this SSO identity")

// ThrottledError means too many failed sign-ins came from one address.
type ThrottledError struct {
	RetryAfter time.Duration
}

func (e *ThrottledError) Error() string {
	return fmt.Sprintf("too many failed sign-ins; try again in %s", e.RetryAfter.Round(time.Second))
}
