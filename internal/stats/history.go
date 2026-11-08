package stats

import (
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// stageChange is one recorded stage move of a ticket.
type stageChange struct {
	From domain.Stage
	To   domain.Stage
	At   time.Time
}

// stageSpan is a finished stay in one stage.
type stageSpan struct {
	Stage domain.Stage
	Hours float64
	End   time.Time
}

// history is a ticket with its stage moves, oldest first.
type history struct {
	Ticket  domain.Ticket
	Changes []stageChange
}

// histories pairs every ticket with its stage entries. Entries of tickets
// that no longer exist are ignored.
func histories(tickets []domain.Ticket, timeline []domain.TimelineEntry) []history {
	byID := make(map[int64]int, len(tickets))
	out := make([]history, len(tickets))
	for i, ticket := range tickets {
		byID[ticket.ID] = i
		out[i] = history{Ticket: ticket}
	}
	for _, entry := range timeline {
		index, isKnown := byID[entry.TicketID]
		if !isKnown || entry.Type != domain.TimelineStage {
			continue
		}
		change := stageChange{From: stageField(entry, "from"), To: stageField(entry, "to"), At: entry.At}
		out[index].Changes = append(out[index].Changes, change)
	}
	return out
}

// MergedAt is when the ticket last moved from active work to a completed stage.
func (h history) MergedAt() (time.Time, bool) {
	for i := len(h.Changes) - 1; i >= 0; i-- {
		if !h.Changes[i].From.IsCompleted() && h.Changes[i].To.IsCompleted() {
			return h.Changes[i].At, true
		}
	}
	return time.Time{}, false
}

// StartedAt is when the ticket first entered an in-progress stage, which is
// when agents began delivering it rather than shaping it.
func (h history) StartedAt() (time.Time, bool) {
	return h.firstInto(func(s domain.Stage) bool { return s.Lane() == domain.LaneInProgress })
}

// ReviewOpenedAt is when the ticket first reached review, which is when its
// pull request was opened.
func (h history) ReviewOpenedAt() (time.Time, bool) {
	return h.firstInto(func(s domain.Stage) bool { return s == domain.StagePR })
}

// Repairs counts returns from review to build within the window: a failed
// check, conflict or requested change the agent had to fix.
func (h history) Repairs(w Window) int {
	count := 0
	for _, change := range h.Changes {
		if change.From == domain.StagePR && change.To == domain.StageBuild && w.Contains(change.At) {
			count++
		}
	}
	return count
}

// Spans lists every finished stay in a stage. The first stage is the one the
// first move left, so autonomous tickets (which start ready) are right.
func (h history) Spans() []stageSpan {
	if len(h.Changes) == 0 {
		return nil
	}
	spans := make([]stageSpan, 0, len(h.Changes))
	start := h.Ticket.CreatedAt
	for _, change := range h.Changes {
		spans = append(spans, stageSpan{Stage: change.From, Hours: hoursBetween(start, change.At), End: change.At})
		start = change.At
	}
	return spans
}

func (h history) firstInto(matches func(domain.Stage) bool) (time.Time, bool) {
	for _, change := range h.Changes {
		if matches(change.To) {
			return change.At, true
		}
	}
	return time.Time{}, false
}

func stageField(entry domain.TimelineEntry, key string) domain.Stage {
	value, _ := entry.Data[key].(string)
	return domain.Stage(value)
}

func hoursBetween(from, to time.Time) float64 {
	return to.Sub(from).Hours()
}
