package domain

import (
	"math"
	"strings"
	"time"
)

// UsageReport is the token usage of one model call made while working a
// ticket. CallKey identifies the call, so reporting it again changes nothing.
type UsageReport struct {
	TicketID         int64
	CallKey          string
	Provider         string
	Model            string
	InputTokens      int64
	CacheWriteTokens int64
	CacheReadTokens  int64
	OutputTokens     int64
	CostUSD          float64
	ReportedBy       string
	ReportedAt       time.Time
}

// maxCallTokens bounds one count of one call; anything larger is a bad report.
const maxCallTokens = 10_000_000_000

// Validate checks a report before it is stored.
func (r UsageReport) Validate() error {
	if r.CallKey == "" || len(r.CallKey) > 200 || strings.TrimSpace(r.CallKey) != r.CallKey {
		return Invalidf("call_key must be 1 to 200 characters with no surrounding spaces")
	}
	if r.Provider != "claude" && r.Provider != "codex" {
		return Invalidf("provider must be claude or codex")
	}
	if len(r.Model) > 100 {
		return Invalidf("model must be at most 100 characters")
	}
	for _, n := range []int64{r.InputTokens, r.CacheWriteTokens, r.CacheReadTokens, r.OutputTokens} {
		if n < 0 || n > maxCallTokens {
			return Invalidf("token counts must be between 0 and %d", int64(maxCallTokens))
		}
	}
	if math.IsNaN(r.CostUSD) || r.CostUSD < 0 || r.CostUSD > 100_000 {
		return Invalidf("cost_usd must be between 0 and 100000")
	}
	return nil
}

// TokenUsage is the token usage reported for a ticket, summed over its calls.
type TokenUsage struct {
	Calls            int
	InputTokens      int64
	CacheWriteTokens int64
	CacheReadTokens  int64
	OutputTokens     int64
	CostUSD          float64
}

// cacheReadWeight is how much a cached input token counts against usage
// compared with a new one; both providers bill cache reads at about a tenth.
const cacheReadWeight = 10

// Weighted is the usage in new-token terms: new input, cache writes and output
// count in full, cache reads at a tenth. It compares Claude calls, which report
// cache reads, with Codex calls, whose totals already leave them out.
func (u TokenUsage) Weighted() int64 {
	return u.InputTokens + u.CacheWriteTokens + u.OutputTokens + u.CacheReadTokens/cacheReadWeight
}
