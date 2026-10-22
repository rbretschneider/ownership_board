package domain

import (
	"strings"
	"unicode/utf8"
)

// Size limits keep a runaway agent from flooding the board.
const (
	MaxTitleLength      = 200
	MaxBodyLength       = 20_000
	MaxSpecLength       = 20_000
	MaxCommentLength    = 20_000
	MaxPromptLength     = 1_000
	MaxOptionLength     = 200
	MaxOptionsPerPrompt = 8
	MaxQuestionsPerAsk  = 10
	MaxAnswerLength     = 5_000
	MaxURLLength        = 500
	MaxBranchLength     = 200
)

// RequireText trims value and fails when it is empty or longer than max runes.
func RequireText(field, value string, max int) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", Invalidf("%s is required", field)
	}
	return LimitText(field, trimmed, max)
}

// LimitText fails when value is longer than max runes. Empty is allowed.
func LimitText(field, value string, max int) (string, error) {
	if utf8.RuneCountInString(value) > max {
		return "", Invalidf("%s must be at most %d characters", field, max)
	}
	return value, nil
}
