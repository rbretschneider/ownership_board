package reposource

import (
	"errors"
	"net/http"
)

// ProviderError is a failure talking to a hosting provider. Its message is
// fixed text written by the board, safe to show a person: it never contains
// response bytes, URLs or connection detail. It is not a board fault, so it
// should not raise an operator alert.
type ProviderError struct {
	Message string
	// Status is the provider's HTTP status, when it answered with one.
	Status int
}

func (e *ProviderError) Error() string {
	return e.Message
}

func providerError(message string) error {
	return &ProviderError{Message: message}
}

// IsProviderError reports whether err came from a provider call.
func IsProviderError(err error) bool {
	var providerErr *ProviderError
	return errors.As(err, &providerErr)
}

// isNotFound reports whether the provider answered 404.
func isNotFound(err error) bool {
	var providerErr *ProviderError
	return errors.As(err, &providerErr) && providerErr.Status == http.StatusNotFound
}
