package domain

import (
	"errors"
	"fmt"
)

// ErrorKind classifies a domain failure so adapters can map it to a response.
type ErrorKind int

// The error kinds.
const (
	KindInvalid ErrorKind = iota + 1
	KindNotFound
	KindConflict
	KindForbidden
)

// Error is a failure the caller caused and can correct.
type Error struct {
	Kind    ErrorKind
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

// Invalidf reports bad input.
func Invalidf(format string, args ...any) error {
	return &Error{Kind: KindInvalid, Message: fmt.Sprintf(format, args...)}
}

// NotFoundf reports a missing entity.
func NotFoundf(format string, args ...any) error {
	return &Error{Kind: KindNotFound, Message: fmt.Sprintf(format, args...)}
}

// Conflictf reports a request that clashes with current state.
func Conflictf(format string, args ...any) error {
	return &Error{Kind: KindConflict, Message: fmt.Sprintf(format, args...)}
}

// Forbiddenf reports an actor that may not perform the action.
func Forbiddenf(format string, args ...any) error {
	return &Error{Kind: KindForbidden, Message: fmt.Sprintf(format, args...)}
}

// KindOf returns the domain error kind wrapped in err, or zero when err is not
// a domain error.
func KindOf(err error) ErrorKind {
	var domainErr *Error
	if errors.As(err, &domainErr) {
		return domainErr.Kind
	}
	return 0
}
