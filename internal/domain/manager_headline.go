package domain

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// lineMarkers are the markdown markers a summary line may start with: heading
// hashes followed by a space, list bullets and quotes. A ticket reference such
// as "#64" is not a marker and stays.
var lineMarkers = regexp.MustCompile(`^(?:#{1,6}\s+|[-*+>]\s+)+`)

// legacyEntry is the pre-2026-11-16 summary line "#64 <fingerprints>: why: ... |
// action: ... | expect: ...". Its headline is the ticket and the action.
var legacyEntry = regexp.MustCompile(`^#(\d+)\b[^|]*?\bwhy:.*?\|\s*action:\s*([^|]+)`)

// plainLine is a summary line without its leading markdown markers and emphasis.
func plainLine(line string) string {
	line = lineMarkers.ReplaceAllString(strings.TrimSpace(line), "")
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "*_`"))
}

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
		if rest, ok := cutPrefixFold(plainLine(line), headlinePrefix); ok {
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
		if _, ok := cutPrefixFold(plainLine(line), headlinePrefix); ok {
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
		line = plainLine(line)
		if line == "" || strings.HasSuffix(line, ":") && len(line) < 40 || strings.EqualFold(line, "MANAGER SUMMARY") || strings.EqualFold(line, "Board review") {
			continue
		}
		if m := legacyEntry.FindStringSubmatch(line); m != nil {
			return "#" + m[1] + ": " + strings.TrimSpace(m[2])
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
