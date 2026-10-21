package secretbox

import (
	"fmt"
	"io"
	"log/slog"
)

// redacted is what a Secret shows instead of its value.
const redacted = "[redacted]"

// Secret holds a credential in memory. It prints, marshals and logs as
// "[redacted]", so passing one to fmt, encoding/json or slog by accident
// cannot leak it. Reveal returns the value for the one place that needs it.
type Secret struct {
	value string
}

// NewSecret wraps a value.
func NewSecret(value string) Secret {
	return Secret{value: value}
}

// Reveal returns the value. Call it only where the credential is sent.
func (s Secret) Reveal() string {
	return s.value
}

// IsEmpty reports whether there is no value.
func (s Secret) IsEmpty() bool {
	return s.value == ""
}

// String implements fmt.Stringer.
func (s Secret) String() string {
	return redacted
}

// GoString implements fmt.GoStringer, which %#v uses.
func (s Secret) GoString() string {
	return redacted
}

// Format implements fmt.Formatter, so every verb (%d, %t, %x included)
// prints the placeholder rather than the value.
func (s Secret) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, redacted)
}

// MarshalJSON implements json.Marshaler.
func (s Secret) MarshalJSON() ([]byte, error) {
	return []byte(`"` + redacted + `"`), nil
}

// LogValue implements slog.LogValuer.
func (s Secret) LogValue() slog.Value {
	return slog.StringValue(redacted)
}
