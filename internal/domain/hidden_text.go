package domain

import (
	"strings"
	"unicode"
)

// RejectHiddenText refuses text containing characters a person cannot see:
// Unicode format characters (zero-width, bidi controls, tag characters) and
// control characters other than newline and tab. It is applied to text agents
// write, because such characters can hide instructions from the owner while
// the next agent still reads them. Text people write is not checked, so emoji
// built with zero-width joiners keep working.
func RejectHiddenText(field, value string) error {
	for _, r := range value {
		if r == '\n' || r == '\t' {
			continue
		}
		if unicode.Is(unicode.Cf, r) || unicode.IsControl(r) {
			return Invalidf("%s contains invisible or control characters (U+%04X); remove them", field, r)
		}
	}
	return nil
}

// StripHiddenText removes the characters RejectHiddenText refuses. It is for
// text from outside the board, such as a provider's repo description, which
// cannot be sent back for correction.
func StripHiddenText(value string) string {
	return strings.Map(func(r rune) rune {
		if r != '\n' && r != '\t' && (unicode.Is(unicode.Cf, r) || unicode.IsControl(r)) {
			return -1
		}
		return r
	}, value)
}
