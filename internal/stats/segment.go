package stats

import (
	"cmp"
	"slices"
	"strings"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// Timeline data keys the report reads.
const (
	dataModel  = "model"
	dataAction = "action"
	// workerPrefix starts the identity of a worker sharing an agent token:
	// "@worker/<token id>/<worker id>".
	workerPrefix = "@worker/"
)

// segmentBuilder accumulates one segment's metrics.
type segmentBuilder struct {
	window     Window
	segment    Segment
	lead       []float64
	cycle      []float64
	stageHours map[domain.Stage][]float64
	daily      map[string]int
	wip        map[domain.Stage]int
	models     map[string]map[int64]bool
	claims     map[string]int
	agents     map[string]int
}

func newSegmentBuilder(w Window) *segmentBuilder {
	return &segmentBuilder{
		window: w, stageHours: map[domain.Stage][]float64{}, daily: map[string]int{},
		wip: map[domain.Stage]int{}, models: map[string]map[int64]bool{},
		claims: map[string]int{}, agents: map[string]int{},
	}
}

// addTicket counts one ticket's history and current state.
func (b *segmentBuilder) addTicket(h history, openQuestions int) {
	b.addFlow(h)
	b.addMerge(h)
	b.addCurrent(h.Ticket, openQuestions)
	for _, span := range h.Spans() {
		if b.window.Contains(span.End) {
			b.stageHours[span.Stage] = append(b.stageHours[span.Stage], span.Hours)
		}
	}
}

// addEntry counts a non-stage timeline entry: a claim, a reported model or a
// rejected checkpoint.
func (b *segmentBuilder) addEntry(entry domain.TimelineEntry) {
	if !b.window.Contains(entry.At) {
		return
	}
	switch entry.Type {
	case domain.TimelineClaimed:
		b.addClaim(entry)
	case domain.TimelineEdited:
		b.addModel(entry)
	case domain.TimelineChangesRequested:
		b.segment.ChangesRequested++
	}
}

func (b *segmentBuilder) build() Segment {
	segment := b.segment
	segment.LeadTime = distributionOf(b.lead)
	segment.CycleTime = distributionOf(b.cycle)
	segment.Daily = b.dailyCounts()
	segment.StageTimes = b.stageTimes()
	segment.WIP = b.wipCounts()
	segment.Models = sortedCounts(distinctCounts(b.models))
	segment.Claims = sortedCounts(b.claims)
	segment.Agents = sortedCounts(b.agents)
	return segment
}

func (b *segmentBuilder) addFlow(h history) {
	if b.window.Contains(h.Ticket.CreatedAt) {
		b.segment.Created++
	}
	if opened, isOpened := h.ReviewOpenedAt(); isOpened && b.window.Contains(opened) {
		b.segment.PRsOpened++
	}
	if repairs := h.Repairs(b.window); repairs > 0 {
		b.segment.RepairRounds += repairs
		b.segment.RepairedTickets++
	}
}

func (b *segmentBuilder) addMerge(h history) {
	merged, isMerged := h.MergedAt()
	// The last 24 hours may reach back past a one-day window's midnight.
	if isMerged && b.window.IsRecent(merged) {
		b.segment.MergedRecent++
	}
	if !isMerged || !b.window.Contains(merged) {
		return
	}
	b.segment.Merged++
	b.daily[b.window.Day(merged)]++
	b.lead = append(b.lead, hoursBetween(h.Ticket.CreatedAt, merged))
	if started, isStarted := h.StartedAt(); isStarted && !started.After(merged) {
		b.cycle = append(b.cycle, hoursBetween(started, merged))
	}
}

func (b *segmentBuilder) addCurrent(ticket domain.Ticket, openQuestions int) {
	if ticket.Stage.IsCompleted() {
		return
	}
	b.wip[ticket.Stage]++
	if ticket.IsBlocked() {
		b.segment.Blocked++
	}
	if ticket.NeedsOwner(openQuestions) {
		b.segment.NeedsOwner++
	}
}

func (b *segmentBuilder) addClaim(entry domain.TimelineEntry) {
	if action, _ := entry.Data[dataAction].(string); action != "" {
		b.claims[action]++
	}
	b.agents[workerName(entry.Actor.Name)]++
}

func (b *segmentBuilder) addModel(entry domain.TimelineEntry) {
	model, _ := entry.Data[dataModel].(string)
	if model == "" {
		return
	}
	if b.models[model] == nil {
		b.models[model] = map[int64]bool{}
	}
	b.models[model][entry.TicketID] = true
}

func (b *segmentBuilder) dailyCounts() []DayCount {
	days := b.window.DayList()
	counts := make([]DayCount, len(days))
	for i, day := range days {
		counts[i] = DayCount{Date: day, Merged: b.daily[day]}
	}
	return counts
}

func (b *segmentBuilder) stageTimes() []StageTime {
	var times []StageTime
	for _, info := range domain.Stages() {
		if !info.ID.IsCompleted() {
			times = append(times, StageTime{Stage: info.ID, Distribution: distributionOf(b.stageHours[info.ID])})
		}
	}
	return times
}

func (b *segmentBuilder) wipCounts() []StageCount {
	var counts []StageCount
	for _, info := range domain.Stages() {
		if !info.ID.IsCompleted() {
			counts = append(counts, StageCount{Stage: info.ID, Lane: info.Lane, Count: b.wip[info.ID]})
		}
	}
	return counts
}

// workerName drops the token prefix from a shared-token worker identity, so
// the same worker is one name whichever token it used.
func workerName(actor string) string {
	rest, isWorker := strings.CutPrefix(actor, workerPrefix)
	if !isWorker {
		return actor
	}
	if _, name, hasName := strings.Cut(rest, "/"); hasName {
		return name
	}
	return actor
}

func distinctCounts(sets map[string]map[int64]bool) map[string]int {
	counts := make(map[string]int, len(sets))
	for name, set := range sets {
		counts[name] = len(set)
	}
	return counts
}

// sortedCounts lists tallies, largest first, then by name.
func sortedCounts(counts map[string]int) []NamedCount {
	out := make([]NamedCount, 0, len(counts))
	for name, count := range counts {
		out = append(out, NamedCount{Name: name, Count: count})
	}
	slices.SortFunc(out, func(a, b NamedCount) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Name, b.Name))
	})
	return out
}
