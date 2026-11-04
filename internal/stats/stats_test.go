package stats_test

import (
	"errors"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
	"github.com/rbretschneider/ownership_board/internal/stats"
)

// now is a Tuesday afternoon in UTC.
var now = time.Date(2026, 3, 10, 15, 0, 0, 0, time.UTC)

// board builds report input in the vocabulary of the workflow.
type board struct {
	t     *testing.T
	input stats.Input
}

func newBoard(t *testing.T) *board {
	return &board{t: t, input: stats.Input{OpenQuestions: map[int64]int{}}}
}

// ticket files a ticket created ago before now, in its starting stage.
func (b *board) ticket(id int64, isAutonomous bool, ago time.Duration) *ticketSteps {
	stage := domain.StageRefine
	if isAutonomous {
		stage = domain.StageReady
	}
	b.input.Tickets = append(b.input.Tickets, domain.Ticket{
		ID: id, RepoID: 1, IsAutonomous: isAutonomous, CreatedAt: now.Add(-ago), Stage: stage, Lane: stage.Lane(),
	})
	return &ticketSteps{board: b, index: len(b.input.Tickets) - 1}
}

func (b *board) entry(ticketID int64, kind domain.TimelineType, actor string, ago time.Duration, data map[string]any) {
	b.input.Timeline = append(b.input.Timeline, domain.TimelineEntry{
		TicketID: ticketID, Type: kind, Actor: domain.Actor{Kind: domain.ActorAgent, Name: actor}, Data: data, At: now.Add(-ago),
	})
}

func (b *board) report(days int) stats.Report {
	b.t.Helper()
	return b.reportIn(days, 0)
}

func (b *board) reportIn(days, offsetMinutes int) stats.Report {
	b.t.Helper()
	window, err := stats.NewWindow(now, days, offsetMinutes)
	if err != nil {
		b.t.Fatalf("window: %v", err)
	}
	return stats.Compute(b.input, window)
}

// ticketSteps records a ticket's stage moves.
type ticketSteps struct {
	board *board
	index int
}

// to moves the ticket to stage ago before now.
func (s *ticketSteps) to(stage domain.Stage, ago time.Duration) *ticketSteps {
	ticket := &s.board.input.Tickets[s.index]
	s.board.entry(ticket.ID, domain.TimelineStage, "agent", ago, map[string]any{"from": string(ticket.Stage), "to": string(stage)})
	ticket.Stage, ticket.Lane = stage, stage.Lane()
	return s
}

func (s *ticketSteps) blocked() *ticketSteps {
	s.board.input.Tickets[s.index].BlockedReason = "needs a decision"
	return s
}

func (s *ticketSteps) inRepo(id int64) *ticketSteps {
	s.board.input.Tickets[s.index].RepoID = id
	return s
}

const hour = time.Hour
const day = 24 * time.Hour

func expectEqual[T comparable](t *testing.T, name string, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func TestMergesSplitAutonomousFromOwnerTickets(t *testing.T) {
	b := newBoard(t)
	b.ticket(1, true, 3*day).to(domain.StageDesign, 2*day).to(domain.StageBuild, day).to(domain.StagePR, 10*hour).to(domain.StageDone, 2*hour)
	b.ticket(2, false, 5*day).to(domain.StageDone, 3*day)
	b.ticket(3, false, 90*day).to(domain.StageDone, 60*day)

	report := b.report(30)

	expectEqual(t, "autonomous merged in 24h", report.Autonomous.MergedRecent, 1)
	expectEqual(t, "owner merged in 24h", report.Owner.MergedRecent, 0)
	expectEqual(t, "owner merged in window", report.Owner.Merged, 1)
	expectEqual(t, "all merged in window", report.All.Merged, 2)
	expectEqual(t, "autonomous PRs opened", report.Autonomous.PRsOpened, 1)
}

func TestLast24HoursCountEvenBeforeAOneDayWindowsMidnight(t *testing.T) {
	b := newBoard(t)
	b.ticket(1, false, 2*day).to(domain.StageDone, 20*hour)

	segment := b.report(1).Owner

	expectEqual(t, "merged in 24h", segment.MergedRecent, 1)
	expectEqual(t, "merged today", segment.Merged, 0)
}

func TestLeadTimeRunsFromCreatedAndCycleTimeFromFirstInProgress(t *testing.T) {
	b := newBoard(t)
	b.ticket(1, true, 50*hour).to(domain.StageDesign, 40*hour).to(domain.StageBuild, 30*hour).to(domain.StagePR, 20*hour).to(domain.StageDone, 10*hour)
	b.ticket(2, true, 30*hour).to(domain.StageDesign, 25*hour).to(domain.StagePR, 15*hour).to(domain.StageDone, 5*hour)

	segment := b.report(7).Autonomous

	expectEqual(t, "lead count", segment.LeadTime.Count, 2)
	expectEqual(t, "lead median", segment.LeadTime.MedianHours, 32.5)
	expectEqual(t, "lead p90", segment.LeadTime.P90Hours, 40.0)
	expectEqual(t, "cycle median", segment.CycleTime.MedianHours, 25.0)
}

func TestTicketDraggedStraightToDoneHasLeadTimeButNoCycleTime(t *testing.T) {
	b := newBoard(t)
	b.ticket(1, false, 10*hour).to(domain.StageDone, 4*hour)

	segment := b.report(7).Owner

	expectEqual(t, "lead count", segment.LeadTime.Count, 1)
	expectEqual(t, "cycle count", segment.CycleTime.Count, 0)
}

func TestStageTimesMeasureFinishedStaysEndingInTheWindow(t *testing.T) {
	b := newBoard(t)
	b.ticket(1, false, 10*hour).to(domain.StageSpecReview, 8*hour).to(domain.StageReady, 5*hour).to(domain.StageDesign, 4*hour)

	times := map[domain.Stage]stats.StageTime{}
	for _, stage := range b.report(7).Owner.StageTimes {
		times[stage.Stage] = stage
	}

	expectEqual(t, "refine hours", times[domain.StageRefine].MedianHours, 2.0)
	expectEqual(t, "spec review hours", times[domain.StageSpecReview].MedianHours, 3.0)
	expectEqual(t, "ready hours", times[domain.StageReady].MedianHours, 1.0)
	// Design is still under way, so it has no finished stay yet.
	expectEqual(t, "design count", times[domain.StageDesign].Count, 0)
}

func TestRepairRoundsCountReturnsFromReviewToBuild(t *testing.T) {
	b := newBoard(t)
	b.ticket(1, true, 3*day).to(domain.StageDesign, 2*day).to(domain.StageBuild, 40*hour).
		to(domain.StagePR, 30*hour).to(domain.StageBuild, 20*hour).to(domain.StagePR, 10*hour).to(domain.StageBuild, 5*hour)
	b.ticket(2, true, 3*day).to(domain.StageDesign, 2*day).to(domain.StageBuild, 40*hour).to(domain.StagePR, 30*hour)

	segment := b.report(7).Autonomous

	expectEqual(t, "repair rounds", segment.RepairRounds, 2)
	expectEqual(t, "repaired tickets", segment.RepairedTickets, 1)
	// Re-entering review after a repair is not a new pull request.
	expectEqual(t, "PRs opened", segment.PRsOpened, 2)
}

func TestCurrentStateCountsOpenWorkBlocksAndOwnerTurns(t *testing.T) {
	b := newBoard(t)
	b.ticket(1, false, day).to(domain.StageSpecReview, hour)
	b.ticket(2, false, day).blocked()
	b.ticket(3, false, day)
	b.input.OpenQuestions[3] = 2
	b.ticket(4, false, day).to(domain.StageDone, hour)

	segment := b.report(7).Owner
	wip := map[domain.Stage]int{}
	for _, count := range segment.WIP {
		wip[count.Stage] = count.Count
	}

	expectEqual(t, "refine WIP", wip[domain.StageRefine], 2)
	expectEqual(t, "spec review WIP", wip[domain.StageSpecReview], 1)
	expectEqual(t, "blocked", segment.Blocked, 1)
	expectEqual(t, "needs owner", segment.NeedsOwner, 3)
}

func TestAgentMixCountsClaimsAndDistinctTicketsPerModel(t *testing.T) {
	b := newBoard(t)
	b.ticket(1, true, day)
	b.ticket(2, true, day)
	b.entry(1, domain.TimelineClaimed, "@worker/7/codex-1", hour, map[string]any{"action": "design"})
	b.entry(1, domain.TimelineClaimed, "@worker/7/codex-1", hour, map[string]any{"action": "implement"})
	b.entry(2, domain.TimelineClaimed, "loop-a", hour, map[string]any{"action": "design"})
	b.entry(1, domain.TimelineEdited, "@worker/7/codex-1", hour, map[string]any{"model": "gpt-5"})
	b.entry(1, domain.TimelineEdited, "@worker/7/codex-1", hour, map[string]any{"model": "gpt-5"})
	b.entry(2, domain.TimelineEdited, "loop-a", hour, map[string]any{"model": "claude-opus"})
	b.entry(2, domain.TimelineEdited, "loop-a", 40*day, map[string]any{"model": "old-model"})

	segment := b.report(30).Autonomous

	expectEqual(t, "top action", segment.Claims[0], stats.NamedCount{Name: "design", Count: 2})
	expectEqual(t, "top agent", segment.Agents[0], stats.NamedCount{Name: "codex-1", Count: 2})
	expectEqual(t, "model count", len(segment.Models), 2)
	expectEqual(t, "first model", segment.Models[0], stats.NamedCount{Name: "claude-opus", Count: 1})
	expectEqual(t, "second model", segment.Models[1], stats.NamedCount{Name: "gpt-5", Count: 1})
}

func TestChangesRequestedCountsOwnerRejections(t *testing.T) {
	b := newBoard(t)
	b.ticket(1, false, day)
	b.entry(1, domain.TimelineChangesRequested, "owner", hour, map[string]any{})

	expectEqual(t, "changes requested", b.report(7).Owner.ChangesRequested, 1)
}

func TestDailyThroughputUsesTheOwnersLocalDays(t *testing.T) {
	b := newBoard(t)
	// 16:00 UTC the previous day is 11:00 there in UTC-5; 02:00 UTC today is
	// still 21:00 yesterday there.
	b.ticket(1, false, 2*day).to(domain.StageDone, 23*hour)
	b.ticket(2, false, 2*day).to(domain.StageDone, 13*hour)

	daily := b.reportIn(2, -300).All.Daily

	expectEqual(t, "days", len(daily), 2)
	expectEqual(t, "yesterday", daily[0], stats.DayCount{Date: "2026-03-09", Merged: 2})
	expectEqual(t, "today", daily[1], stats.DayCount{Date: "2026-03-10", Merged: 0})
}

func TestReposAreBrokenDownBusiestFirst(t *testing.T) {
	b := newBoard(t)
	b.input.Repos = []domain.Repo{{ID: 1, FullName: "acme/api"}, {ID: 2, FullName: "acme/web"}}
	b.ticket(1, true, 2*day).inRepo(2).to(domain.StageDone, hour)
	b.ticket(2, false, 2*day).inRepo(2).to(domain.StageDone, hour)
	b.ticket(3, false, 2*day).inRepo(1)
	b.ticket(4, false, 2*day).inRepo(0)

	repos := b.report(7).Repos

	expectEqual(t, "repos", len(repos), 3)
	expectEqual(t, "busiest", repos[0].Name, "acme/web")
	expectEqual(t, "autonomous merged", repos[0].Autonomous.Merged, 1)
	expectEqual(t, "owner merged", repos[0].Owner.Merged, 1)
	// Ties fall back to name order.
	expectEqual(t, "no repo", repos[1].Name, "No repo")
	expectEqual(t, "open elsewhere", repos[2].Owner.Open, 1)
}

func TestWindowRejectsOutOfRangeInput(t *testing.T) {
	cases := []struct {
		name          string
		days, offsets int
	}{
		{"zero days", 0, 0},
		{"over a year", stats.MaxWindowDays + 1, 0},
		{"offset past UTC+14", 7, 15 * 60},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := stats.NewWindow(now, c.days, c.offsets)
			var invalid *domain.Error
			if !errors.As(err, &invalid) {
				t.Fatalf("err = %v, want a domain validation error", err)
			}
		})
	}
}

func TestWindowStartsAtLocalMidnight(t *testing.T) {
	window, err := stats.NewWindow(now, 7, 60)
	if err != nil {
		t.Fatal(err)
	}
	expectEqual(t, "start", window.Start().UTC(), time.Date(2026, 3, 3, 23, 0, 0, 0, time.UTC))
}
