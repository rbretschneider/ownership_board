package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

// MaxManagerHeadlineLength bounds the one-line headline the Team view shows.
const MaxManagerHeadlineLength = 100

// headlinePrefix starts the line a manager session writes first in its
// summary: "HEADLINE: <one plain sentence>".
const headlinePrefix = "HEADLINE:"

// ManagerHeadline is a session's one-line headline: its "HEADLINE:" line when
// the summary has one, else the summary's first sentence, at most
// MaxManagerHeadlineLength characters.
func ManagerHeadline(summary string) string {
	for _, line := range strings.Split(summary, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#*-> "))
		if rest, ok := cutPrefixFold(line, headlinePrefix); ok {
			if headline := strings.TrimSpace(strings.Trim(strings.TrimSpace(rest), "*_`")); headline != "" {
				return truncateRunes(headline, MaxManagerHeadlineLength)
			}
		}
	}
	return truncateRunes(firstSentence(summary), MaxManagerHeadlineLength)
}

// WithManagerHeadline puts headline as the summary's first line, unless the
// summary already carries a HEADLINE line.
func WithManagerHeadline(summary, headline string) string {
	headline = strings.Join(strings.Fields(headline), " ")
	if headline == "" || HasManagerHeadline(summary) {
		return summary
	}
	line := headlinePrefix + " " + truncateRunes(headline, MaxManagerHeadlineLength)
	if strings.TrimSpace(summary) == "" {
		return line
	}
	return line + "\n" + summary
}

// HasManagerHeadline reports whether the summary carries a HEADLINE line: the
// structured format the runbook asks for since 2026-11-16.
func HasManagerHeadline(summary string) bool {
	for _, line := range strings.Split(summary, "\n") {
		if _, ok := cutPrefixFold(strings.TrimLeft(strings.TrimSpace(line), "#*-> "), headlinePrefix); ok {
			return true
		}
	}
	return false
}

func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) < len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
		return "", false
	}
	return s[len(prefix):], true
}

// firstSentence is the summary's first meaningful line up to its first
// sentence end, skipping markdown headings and block labels.
func firstSentence(summary string) string {
	for _, line := range strings.Split(summary, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#*-> "))
		line = strings.Trim(line, "*_`")
		if line == "" || strings.HasSuffix(line, ":") && len(line) < 40 || strings.EqualFold(line, "MANAGER SUMMARY") {
			continue
		}
		for i, r := range line {
			if (r == '.' || r == '!' || r == '?') && (i+1 == len(line) || line[i+1] == ' ') {
				return line[:i+1]
			}
		}
		return line
	}
	return ""
}

func truncateRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:limit-1])) + "…"
}

// ManagerActivityState is whether the manager is in a session right now.
type ManagerActivityState string

// Manager activity states.
const (
	ManagerWorking ManagerActivityState = "working"
	ManagerIdle    ManagerActivityState = "idle"
)

// ManagerActivityKind is why the manager started its session.
type ManagerActivityKind string

// Manager session kinds.
const (
	ManagerActivityFlag   ManagerActivityKind = "flag"
	ManagerActivityReview ManagerActivityKind = "review"
	ManagerActivityRepair ManagerActivityKind = "repair"
)

// ManagerActivity is what the manager is acting on, as its loop reports it.
type ManagerActivity struct {
	State     ManagerActivityState
	Kind      ManagerActivityKind
	TicketIDs []int64
	Host      string
	StartedAt time.Time
	UpdatedAt time.Time
}
