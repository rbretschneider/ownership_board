package reposource

import (
	"bytes"
	"io"
	"regexp"
	"strings"
)

// failureLine marks a log line that reports a failure: a failed test, a
// failed assertion, an error annotation or an unresolved import.
var failureLine = regexp.MustCompile(`(?i)(\*\*\*failed|\bFAILED\b|##\[error\]|\berror:|test failed|unresolved import|\bpanic:|assertion)`)

// Context kept around a failure line: the lines before it name the test, the
// lines after it carry the assertion's expansion and messages.
const (
	failureLinesBefore = 2
	failureLinesAfter  = 6
)

// failureExcerpt is one failure line with its context, and the offset in the
// raw log where its first line starts.
type failureExcerpt struct {
	offset int64
	text   string
}

// failureCollector watches a log stream for failure lines and keeps the last
// of them, with context, within a byte budget. A job's tail often holds only
// cleanup noise (2026-11-20: makercad's nightly ended in artifact-quota
// errors, and the failed test sat 40 KB earlier), so the reader needs the
// failures from further up as well.
type failureCollector struct {
	budget    int
	offset    int64
	partial   []byte
	before    []failureExcerpt
	open      *strings.Builder
	openAt    int64
	openAfter int
	kept      []failureExcerpt
	keptBytes int
}

func newFailureCollector(budget int) *failureCollector {
	return &failureCollector{budget: budget}
}

// Write implements io.Writer.
func (c *failureCollector) Write(p []byte) (int, error) {
	data := append(c.partial, p...)
	start := 0
	for {
		newline := bytes.IndexByte(data[start:], '\n')
		if newline < 0 {
			break
		}
		c.line(data[start:start+newline], c.offset)
		c.offset += int64(newline + 1)
		start += newline + 1
	}
	c.partial = append([]byte(nil), data[start:]...)
	return len(p), nil
}

func (c *failureCollector) line(raw []byte, offset int64) {
	text := cleanLine(raw)
	if c.open != nil {
		c.open.WriteString("\n" + text)
		c.openAfter--
		if failureLine.MatchString(text) {
			c.openAfter = failureLinesAfter
		}
		if c.openAfter <= 0 {
			c.close()
		}
		return
	}
	if failureLine.MatchString(text) {
		c.open = &strings.Builder{}
		c.openAt = offset
		if len(c.before) > 0 {
			c.openAt = c.before[0].offset
		}
		for _, previous := range c.before {
			c.open.WriteString(previous.text + "\n")
		}
		c.open.WriteString(text)
		c.openAfter = failureLinesAfter
		c.before = nil
		return
	}
	c.before = append(c.before, failureExcerpt{offset: offset, text: text})
	if len(c.before) > failureLinesBefore {
		c.before = c.before[1:]
	}
}

func (c *failureCollector) close() {
	excerpt := failureExcerpt{offset: c.openAt, text: c.open.String()}
	c.open = nil
	if len(excerpt.text) > c.budget {
		excerpt.text = excerpt.text[:c.budget]
	}
	c.kept = append(c.kept, excerpt)
	c.keptBytes += len(excerpt.text)
	for c.keptBytes > c.budget && len(c.kept) > 0 {
		c.keptBytes -= len(c.kept[0].text)
		c.kept = c.kept[1:]
	}
}

// excerptsBefore returns the kept excerpts that start before the raw offset end, in
// log order, separated by a marker line.
func (c *failureCollector) excerptsBefore(end int64) string {
	if c.open != nil {
		c.close()
	}
	var parts []string
	for _, excerpt := range c.kept {
		if excerpt.offset < end {
			parts = append(parts, excerpt.text)
		}
	}
	return strings.ToValidUTF8(strings.Join(parts, "\n...\n"), "")
}

// cleanLine strips colour codes and the timestamp from one log line.
func cleanLine(raw []byte) string {
	text := ansiEscape.ReplaceAllString(strings.TrimRight(string(raw), "\r"), "")
	return logTimestamp.ReplaceAllString(text, "")
}

// Headers that split a log read into its failures and its end.
const (
	failuresHeader = "Failure lines from earlier in the log:\n"
	tailHeader     = "\n\nEnd of the log:\n"
)

// readFailuresAndTail reads a job log and returns its last maxBytes, cleaned,
// preceded by the failure lines that appear before that tail. The whole
// answer stays within maxBytes: the failures get at most half of it.
func readFailuresAndTail(reader io.Reader, maxBytes int) (string, error) {
	collector := newFailureCollector(maxBytes / 2)
	raw, err := readTail(io.TeeReader(reader, collector), maxBytes)
	if err != nil {
		return "", err
	}
	// The tail always gets at least half the budget, so failures that start
	// in the log's last maxBytes/2 bytes show up there instead.
	total := collector.offset + int64(len(collector.partial))
	earlier := collector.excerptsBefore(total - int64(maxBytes/2))
	if earlier == "" {
		return cleanLog(raw, maxBytes), nil
	}
	tailBudget := maxBytes - len(failuresHeader) - len(earlier) - len(tailHeader)
	return failuresHeader + earlier + tailHeader + cleanLog(raw, max(tailBudget, 0)), nil
}
