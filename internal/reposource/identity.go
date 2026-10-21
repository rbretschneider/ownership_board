package reposource

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Limits on what a provider may tell the board about an account. The name is
// stored, shown in Settings and sent in an ntfy push, so it must be short and
// printable whatever a hostile server answers.
const (
	maxIdentityBytes     = 64 << 10
	maxAccountNameLength = 100
)

// cleanAccountName returns the name, or "" when it is empty, too long, or
// contains control or formatting characters.
func cleanAccountName(name string) string {
	value := strings.TrimSpace(name)
	if value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > maxAccountNameLength {
		return ""
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ""
		}
	}
	return value
}

// isPrintableASCII reports whether every byte is a visible ASCII character,
// which every real provider token is.
func isPrintableASCII(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] < '!' || value[i] > '~' {
			return false
		}
	}
	return true
}
