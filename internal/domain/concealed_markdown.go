package domain

import (
	"regexp"
	"strings"
)

var (
	// rawHTMLRe matches an HTML tag, comment or declaration start.
	rawHTMLRe = regexp.MustCompile(`<[A-Za-z!/?]`)
	// referenceDefinitionRe matches a markdown link reference definition,
	// which renders to nothing.
	referenceDefinitionRe = regexp.MustCompile(`^ {0,3}\[[^\]]+\]:`)
	inlineCodeRe          = regexp.MustCompile("`+[^`]*`+")
)

// RejectConcealedMarkdown refuses markdown that renders differently from what
// it says: raw HTML (which can hide text with style or <details>), HTML
// comments and link reference definitions (which render to nothing). The owner
// approves what the board shows and the agent builds from the raw text, so the
// two must be the same. Code blocks and inline code are exempt: they render as
// written.
func RejectConcealedMarkdown(field, text string) error {
	isInFence := false
	fence := ""
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimLeft(line, " ")
		if marker := fenceMarker(trimmed); marker != "" && (!isInFence || strings.HasPrefix(trimmed, fence)) {
			isInFence, fence = !isInFence, marker
			continue
		}
		if isInFence || strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
			continue
		}
		prose := inlineCodeRe.ReplaceAllString(line, "")
		if rawHTMLRe.MatchString(prose) {
			return Invalidf("%s contains raw HTML or an HTML comment; write plain markdown, or put code in backticks", field)
		}
		if referenceDefinitionRe.MatchString(prose) {
			return Invalidf("%s contains a link reference definition, which the board does not show; write links inline", field)
		}
	}
	return nil
}

func fenceMarker(line string) string {
	for _, marker := range []string{"```", "~~~"} {
		if strings.HasPrefix(line, marker) {
			return marker
		}
	}
	return ""
}
