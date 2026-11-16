package app

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// teamRunnerWindow is how recently an agent must have reported a work model
// to appear as an idle runner on the Team view.
const teamRunnerWindow = 6 * time.Hour

// teamManagerSessions bounds the manager history the Team view carries.
const teamManagerSessions = 20

// awaitingWindowsSuffix marks a build ticket whose worker released it while
// its commit waits for, or runs on, a Windows build host.
const awaitingWindowsSuffix = " · awaiting Windows"

// RunnerState is what a runner is doing right now.
type RunnerState string

// Runner states, in the order the Team view lists them.
const (
	RunnerWorking RunnerState = "working"
	RunnerWaiting RunnerState = "waiting"
	RunnerIdle    RunnerState = "idle"
)

var runnerStateOrder = map[RunnerState]int{RunnerWorking: 0, RunnerWaiting: 1, RunnerIdle: 2}

// TeamRunner is one agent seen recently: holding a lease, waiting on a build
// host for a ticket it released, or idle after recent work.
type TeamRunner struct {
	Name  string
	State RunnerState
	// Model and Signal split the ticket's work model "model · signal".
	Model  string
	Signal string
	// TicketID is the ticket it holds or waits on; zero when idle.
	TicketID int64
	LastSeen time.Time
	// ProfileVersion is the role profile the runner loaded.
	// TODO: runners do not report it yet (overhaul plan section 2); empty until they do.
	ProfileVersion string
}

// TeamTicket is a ticket in flight with the runner that holds or waits on it.
type TeamTicket struct {
	TicketSummary
	Holder string
}

// TeamManager is the board manager's card: status, flags and history.
type TeamManager struct {
	Status    ManagerStatus
	OpenFlags int
	Sessions  []domain.ManagerSession
	// Activity is what the manager is acting on right now, as its loop reported it.
	Activity domain.ManagerActivity
}

// Team is the Team view: the manager, its runners, the tickets in flight and
// the build hosts that verify them.
type Team struct {
	ObservedAt time.Time
	Manager    TeamManager
	Runners    []TeamRunner
	Tickets    []TeamTicket
	Hosts      []BuildHostRunner
	QueuedByOS map[domain.BuildHostOS]int
}

// Team reads the whole team in one snapshot. People and agent tokens may read
// it, like the board.
func (s *BoardService) Team(ctx context.Context) (Team, error) {
	now := s.clock.Now()
	team := Team{ObservedAt: now}
	err := s.store.Read(ctx, func(repos Repos) error {
		settings, err := loadSettings(ctx, repos)
		if err != nil {
			return err
		}
		board, err := loadBoard(ctx, repos, RepoScope{}.filter())
		if err != nil {
			return err
		}
		today, err := repos.ManagerUsage().Day(ctx, now.UTC().Format("2006-01-02"))
		if err != nil {
			return err
		}
		flags, err := managerFlagsOf(ctx, repos, board.Tickets, settings, now)
		if err != nil {
			return err
		}
		sessions, err := repos.ManagerUsage().RecentSessions(ctx, teamManagerSessions)
		if err != nil {
			return err
		}
		team.Manager = TeamManager{
			Status:    ManagerStatus{Settings: settings, Today: today, RemainingUSD: float64(settings.ManagerDailyBudgetUSD) - today.CostUSD},
			OpenFlags: len(flags), Sessions: sessions, Activity: s.managerActivityNow(now),
		}
		signals, err := repos.Timeline().StageSignals(ctx)
		if err != nil {
			return err
		}
		team.Runners, team.Tickets = TeamRoster(board.Tickets, signals, now)
		team.Hosts, team.QueuedByOS, err = loadHostRunners(ctx, repos, true)
		return err
	})
	return team, err
}

// TeamRoster derives the runners and the tickets in flight from the board's
// tickets and its stage signals (ordered by ticket, then as recorded).
//
//   - A live lease makes its holder a runner on that ticket: working, or
//     waiting when its work model says it awaits something.
//   - A build ticket released while awaiting Windows keeps the agent that last
//     reported its work model as a waiting runner.
//   - Any other agent that reported a work model within teamRunnerWindow is idle.
//
// Tickets in flight are every open ticket In Progress or Ready, plus any
// other open ticket a runner holds or a build job serves.
func TeamRoster(tickets []TicketSummary, signals []domain.TimelineEntry, now time.Time) ([]TeamRunner, []TeamTicket) {
	type seen struct {
		model string
		at    time.Time
	}
	lastSeen := map[string]seen{}
	lastReporter := map[int64]string{}
	for _, entry := range signals {
		model, ok := entry.Data["model"].(string)
		if entry.Type != domain.TimelineEdited || !ok || !entry.Actor.IsAgent() {
			continue
		}
		lastReporter[entry.TicketID] = entry.Actor.Name
		if previous, ok := lastSeen[entry.Actor.Name]; !ok || entry.At.After(previous.at) {
			lastSeen[entry.Actor.Name] = seen{model: model, at: entry.At}
		}
	}
	runners := map[string]TeamRunner{}
	var inFlight []TeamTicket
	for _, summary := range tickets {
		t := summary.Ticket
		if t.Stage.IsCompleted() {
			continue
		}
		holder := ""
		switch {
		case t.IsLeased(now):
			holder = t.Lease.Holder
			if _, taken := runners[holder]; !taken {
				runners[holder] = leasedRunner(holder, t, lastSeen[holder].at)
			}
		case t.Stage == domain.StageBuild && strings.HasSuffix(t.WorkModel, awaitingWindowsSuffix) && lastReporter[t.ID] != "":
			holder = lastReporter[t.ID]
			if existing, taken := runners[holder]; !taken || existing.State == RunnerIdle {
				model, signal := splitWorkModel(t.WorkModel)
				runners[holder] = TeamRunner{Name: holder, State: RunnerWaiting, Model: model, Signal: signal, TicketID: t.ID, LastSeen: lastSeen[holder].at}
			}
		}
		if holder != "" || summary.Build != nil || t.Lane == domain.LaneInProgress || t.Stage == domain.StageReady {
			inFlight = append(inFlight, TeamTicket{TicketSummary: summary, Holder: holder})
		}
	}
	for name, last := range lastSeen {
		if _, taken := runners[name]; taken || now.Sub(last.at) > teamRunnerWindow {
			continue
		}
		model, _ := splitWorkModel(last.model)
		runners[name] = TeamRunner{Name: name, State: RunnerIdle, Model: model, LastSeen: last.at}
	}
	out := make([]TeamRunner, 0, len(runners))
	for _, runner := range runners {
		out = append(out, runner)
	}
	slices.SortFunc(out, func(a, b TeamRunner) int {
		return cmp.Or(cmp.Compare(runnerStateOrder[a.State], runnerStateOrder[b.State]), cmp.Compare(a.Name, b.Name))
	})
	if inFlight == nil {
		inFlight = []TeamTicket{}
	}
	return out, inFlight
}

// leasedRunner is the runner holding a live lease on t. A worker that holds
// its lease while a build host runs its commit is waiting, not working.
func leasedRunner(holder string, t domain.Ticket, lastSeen time.Time) TeamRunner {
	model, signal := splitWorkModel(t.WorkModel)
	state := RunnerWorking
	if strings.Contains(strings.ToLower(signal), "awaiting") {
		state = RunnerWaiting
	}
	return TeamRunner{Name: holder, State: state, Model: model, Signal: signal, TicketID: t.ID, LastSeen: lastSeen}
}

// splitWorkModel splits "model · signal" into its two parts.
func splitWorkModel(workModel string) (model, signal string) {
	model, signal, _ = strings.Cut(workModel, " · ")
	return strings.TrimSpace(model), strings.TrimSpace(signal)
}
