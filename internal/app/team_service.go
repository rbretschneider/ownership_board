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

// teamPresenceWindow is how recently a worker must have called the board to
// appear on the Team view without having reported work. Workers poll every
// 15 to 60 seconds, so a few minutes covers a slow loop.
const teamPresenceWindow = 3 * time.Minute

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
	// RunnerStandby is a running worker whose slot is above its pool's size in
	// Settings, so it takes no tickets.
	RunnerStandby RunnerState = "standby"
)

var runnerStateOrder = map[RunnerState]int{RunnerWorking: 0, RunnerWaiting: 1, RunnerIdle: 2, RunnerStandby: 3}

// RosterPresence is what the board knows about workers beyond their tickets:
// when each identity last called the board, and each pool's size in Settings
// ("codex" and "claude" worker counts).
type RosterPresence struct {
	Seen  map[string]time.Time
	Pools map[string]int
}

// TeamRunner is one agent seen recently: holding a lease, waiting on a build
// host for a ticket it released, or idle after recent work.
type TeamRunner struct {
	Name string
	// DisplayName is the runner's friendly name from its family's pool, and
	// Family that pool; both are empty for a runner without one.
	DisplayName string
	Family      string
	State       RunnerState
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
	// Claims is how full the autonomous in-flight cap is: when InFlight
	// reaches Cap, idle runners poll and get nothing from Ready.
	Claims TeamClaims
}

// TeamClaims is the autonomous in-flight count against its cap.
type TeamClaims struct {
	InFlight int
	Cap      int
}

// WorkerIdlePoll is how long an idle worker waits between claims: the
// agent_runner worker loop sleeps 30 s after a claim returns no work. The Team
// view counts each idle runner down from its last call by this much.
const WorkerIdlePoll = 30 * time.Second

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
		names, err := repos.RunnerNames().Assigned(ctx)
		if err != nil {
			return err
		}
		presence := RosterPresence{
			Seen:  s.Presence.Snapshot(),
			Pools: map[string]int{"codex": settings.CodexWorkers, "claude": settings.ClaudeWorkers},
		}
		team.Runners, team.Tickets = TeamRosterWith(board.Tickets, signals, now, presence)
		nameRunners(team.Runners, domain.NewRunnerDirectory(names))
		inFlight, err := repos.Tickets().CountInFlightAutonomous(ctx)
		if err != nil {
			return err
		}
		team.Claims = TeamClaims{InFlight: inFlight, Cap: settings.AutonomousInFlightCap}
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
//   - A build ticket released while awaiting Windows has no holder: any free
//     worker picks it up when its verdict returns, so it is not shown under
//     the worker that last touched it.
//   - Any other agent that reported a work model within teamRunnerWindow is idle.
//
// Tickets in flight are the ones being worked: open tickets a runner holds,
// tickets awaiting Windows, or tickets a build job serves.
func TeamRoster(tickets []TicketSummary, signals []domain.TimelineEntry, now time.Time) ([]TeamRunner, []TeamTicket) {
	return TeamRosterWith(tickets, signals, now, RosterPresence{})
}

// TeamRosterWith is TeamRoster plus presence: a worker that called the board
// within teamPresenceWindow appears even if it never reported work, and a
// worker whose slot is above its pool's size in Settings is on standby.
func TeamRosterWith(tickets []TicketSummary, signals []domain.TimelineEntry, now time.Time, presence RosterPresence) ([]TeamRunner, []TeamTicket) {
	type seen struct {
		model string
		at    time.Time
	}
	lastSeen := map[string]seen{}
	for _, entry := range signals {
		model, ok := entry.Data["model"].(string)
		if entry.Type != domain.TimelineEdited || !ok || !entry.Actor.IsAgent() {
			continue
		}
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
		if t.IsLeased(now) {
			holder = t.Lease.Holder
			if _, taken := runners[holder]; !taken {
				runners[holder] = leasedRunner(holder, t, lastSeen[holder].at)
			}
		}
		awaitingWindows := t.Stage == domain.StageBuild && strings.HasSuffix(t.WorkModel, awaitingWindowsSuffix)
		if holder != "" || awaitingWindows || summary.Build != nil {
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
	for name, at := range presence.Seen {
		if now.Sub(at) > teamPresenceWindow {
			continue
		}
		runner, taken := runners[name]
		if !taken {
			runner = TeamRunner{Name: name, State: RunnerIdle}
		}
		if at.After(runner.LastSeen) {
			runner.LastSeen = at
		}
		runners[name] = runner
	}
	for name, runner := range runners {
		if runner.State == RunnerIdle && aboveItsPool(name, presence.Pools) {
			runner.State = RunnerStandby
			runners[name] = runner
		}
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

// aboveItsPool reports whether a worker identity's slot ("codex_3" is slot 3
// of the codex pool) is above that pool's size in Settings. Identities outside
// a known pool are never on standby.
func aboveItsPool(name string, pools map[string]int) bool {
	workerID, ok := domain.WorkerIDOf(name)
	if !ok {
		return false
	}
	family, slot := domain.WorkerFamily(workerID)
	size, known := pools[family]
	return known && slot > size
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

// nameRunners gives each runner its display name and family, when it has one.
func nameRunners(runners []TeamRunner, directory domain.RunnerDirectory) {
	for i := range runners {
		if name, isNamed := directory.Lookup(runners[i].Name); isNamed {
			runners[i].DisplayName, runners[i].Family = name.Name, name.Family
		}
	}
}
