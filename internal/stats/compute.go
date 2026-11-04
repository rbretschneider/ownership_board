package stats

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// noRepoName labels tickets that have no repo yet.
const noRepoName = "No repo"

// Input is the board data a report is computed from.
type Input struct {
	Tickets []domain.Ticket
	// OpenQuestions maps ticket id to its unanswered questions.
	OpenQuestions map[int64]int
	// Timeline holds the entries of the types in TimelineTypes, for every
	// ticket.
	Timeline []domain.TimelineEntry
	Repos    []domain.Repo
}

// TimelineTypes are the history entries a report reads.
var TimelineTypes = []domain.TimelineType{
	domain.TimelineStage, domain.TimelineClaimed, domain.TimelineEdited, domain.TimelineChangesRequested,
}

// Compute builds the report for the window.
func Compute(input Input, w Window) Report {
	all, autonomous, owner := newSegmentBuilder(w), newSegmentBuilder(w), newSegmentBuilder(w)
	pick := func(ticket domain.Ticket) *segmentBuilder {
		if ticket.IsAutonomous {
			return autonomous
		}
		return owner
	}
	tickets := histories(input.Tickets, input.Timeline)
	byID := make(map[int64]domain.Ticket, len(tickets))
	for _, h := range tickets {
		byID[h.Ticket.ID] = h.Ticket
		all.addTicket(h, input.OpenQuestions[h.Ticket.ID])
		pick(h.Ticket).addTicket(h, input.OpenQuestions[h.Ticket.ID])
	}
	for _, entry := range input.Timeline {
		if ticket, isKnown := byID[entry.TicketID]; isKnown {
			all.addEntry(entry)
			pick(ticket).addEntry(entry)
		}
	}
	return Report{
		GeneratedAt: w.Now, WindowDays: w.Days, Since: w.Start(),
		All: all.build(), Autonomous: autonomous.build(), Owner: owner.build(),
		Repos: repoBreakdowns(tickets, input.Repos, w),
	}
}

// repoBreakdowns lists every repo with merged, opened or open work, busiest
// first.
func repoBreakdowns(tickets []history, repos []domain.Repo, w Window) []RepoBreakdown {
	byRepo := map[int64]*RepoBreakdown{}
	for _, h := range tickets {
		breakdown, isSeen := byRepo[h.Ticket.RepoID]
		if !isSeen {
			breakdown = &RepoBreakdown{RepoID: h.Ticket.RepoID, Name: repoName(h.Ticket.RepoID, repos)}
			byRepo[h.Ticket.RepoID] = breakdown
		}
		counts := &breakdown.Owner
		if h.Ticket.IsAutonomous {
			counts = &breakdown.Autonomous
		}
		counts.add(h, w)
	}
	out := make([]RepoBreakdown, 0, len(byRepo))
	for _, breakdown := range byRepo {
		if breakdown.hasActivity() {
			out = append(out, *breakdown)
		}
	}
	slices.SortFunc(out, func(a, b RepoBreakdown) int {
		return cmp.Or(cmp.Compare(b.total(), a.total()), cmp.Compare(a.Name, b.Name))
	})
	return out
}

func (c *RepoCounts) add(h history, w Window) {
	if merged, isMerged := h.MergedAt(); isMerged && w.Contains(merged) {
		c.Merged++
	}
	if opened, isOpened := h.ReviewOpenedAt(); isOpened && w.Contains(opened) {
		c.PRsOpened++
	}
	if h.Ticket.Stage != domain.StageDone {
		c.Open++
	}
}

func (r RepoBreakdown) total() int {
	return r.Autonomous.Merged + r.Owner.Merged
}

func (r RepoBreakdown) hasActivity() bool {
	return r.Autonomous != (RepoCounts{}) || r.Owner != (RepoCounts{})
}

func repoName(id int64, repos []domain.Repo) string {
	if id == 0 {
		return noRepoName
	}
	for _, repo := range repos {
		if repo.ID == id {
			return repo.FullName
		}
	}
	return fmt.Sprintf("Repo #%d", id)
}
