package httpapi

import (
	"net/http"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

// The Team view: the board manager, its runners, the tickets in flight and
// the build hosts verifying them, in one read.

type teamDTO struct {
	ObservedAt time.Time                  `json:"observed_at"`
	Manager    teamManagerDTO             `json:"manager"`
	Runners    []teamRunnerDTO            `json:"runners"`
	Tickets    []teamTicketDTO            `json:"tickets"`
	BuildHosts []teamBuildHostDTO         `json:"build_hosts"`
	QueuedByOS map[domain.BuildHostOS]int `json:"queued_by_os"`
	CIRunners  ciRunnersSnapshot          `json:"ci_runners"`
}

type teamManagerDTO struct {
	managerStatusDTO
	OpenFlags     int                     `json:"open_flags"`
	LatestSummary string                  `json:"latest_summary"`
	LatestAt      *time.Time              `json:"latest_at"`
	Sessions      []domain.ManagerSession `json:"sessions"`
}

type teamRunnerDTO struct {
	Name           string          `json:"name"`
	State          app.RunnerState `json:"state"`
	Model          string          `json:"model"`
	Doing          string          `json:"doing"`
	TicketID       *int64          `json:"ticket_id"`
	LastSeen       *time.Time      `json:"last_seen"`
	ProfileVersion string          `json:"profile_version"`
}

type teamTicketDTO struct {
	ticketSummaryDTO
	Holder string `json:"holder"`
}

type teamBuildHostDTO struct {
	ID          int64              `json:"id"`
	Name        string             `json:"name"`
	OS          domain.BuildHostOS `json:"os"`
	Online      bool               `json:"online"`
	Enabled     bool               `json:"enabled"`
	MaxParallel int                `json:"max_parallel"`
	Running     []teamHostJobDTO   `json:"running"`
	Queued      []teamQueuedJobDTO `json:"queued"`
}

type teamHostJobDTO struct {
	TicketID  int64      `json:"ticket_id"`
	Repo      string     `json:"repo"`
	Status    string     `json:"status"`
	Phase     string     `json:"phase"`
	Detail    string     `json:"detail"`
	StartedAt *time.Time `json:"started_at"`
	Attempt   int        `json:"attempt"`
}

type teamQueuedJobDTO struct {
	TicketID int64     `json:"ticket_id"`
	Repo     string    `json:"repo"`
	QueuedAt time.Time `json:"queued_at"`
}

func (s *server) teamRoutes(rt *router) {
	rt.anyone("GET /api/v1/team", s.team)
}

func (s *server) team(w http.ResponseWriter, r *http.Request, _ caller) error {
	team, err := s.deps.Board.Team(r.Context())
	if err != nil {
		return err
	}
	out := toTeam(team)
	out.CIRunners = readCIRunners(s.deps.CIRunnersFile)
	writeJSON(w, http.StatusOK, out)
	return nil
}

func toTeam(team app.Team) teamDTO {
	out := teamDTO{
		ObservedAt: team.ObservedAt, Manager: toTeamManager(team.Manager),
		Runners: make([]teamRunnerDTO, 0, len(team.Runners)), Tickets: make([]teamTicketDTO, 0, len(team.Tickets)),
		BuildHosts: make([]teamBuildHostDTO, 0, len(team.Hosts)), QueuedByOS: team.QueuedByOS,
	}
	for _, runner := range team.Runners {
		out.Runners = append(out.Runners, teamRunnerDTO{
			Name: runner.Name, State: runner.State, Model: runner.Model, Doing: runnerDoing(runner),
			TicketID: positiveID(&runner.TicketID), LastSeen: nonZeroTime(runner.LastSeen), ProfileVersion: runner.ProfileVersion,
		})
	}
	for _, ticket := range team.Tickets {
		out.Tickets = append(out.Tickets, teamTicketDTO{ticketSummaryDTO: toSummary(ticket.TicketSummary), Holder: ticket.Holder})
	}
	for _, row := range team.Hosts {
		out.BuildHosts = append(out.BuildHosts, toTeamBuildHost(row, team.ObservedAt))
	}
	return out
}

func toTeamManager(manager app.TeamManager) teamManagerDTO {
	sessions := manager.Sessions
	if sessions == nil {
		sessions = []domain.ManagerSession{}
	}
	out := teamManagerDTO{managerStatusDTO: toManagerStatus(manager.Status), OpenFlags: manager.OpenFlags, Sessions: sessions}
	if len(sessions) > 0 {
		out.LatestSummary = sessions[0].Summary
		at := sessions[0].At
		out.LatestAt = &at
	}
	return out
}

func toTeamBuildHost(row app.BuildHostRunner, now time.Time) teamBuildHostDTO {
	h := row.Host
	out := teamBuildHostDTO{
		ID: h.ID, Name: h.Name, OS: h.OS, Online: h.Online(now), Enabled: h.Enabled, MaxParallel: h.MaxParallel,
		Running: make([]teamHostJobDTO, 0, len(row.Jobs)), Queued: make([]teamQueuedJobDTO, 0, len(row.Queue)),
	}
	for _, job := range row.Jobs {
		out.Running = append(out.Running, teamHostJobDTO{
			TicketID: job.TicketID, Repo: job.Repo, Status: job.Status, Phase: job.Phase,
			Detail: cleanDetail(job.Detail), StartedAt: job.StartedAt, Attempt: job.Attempt,
		})
	}
	for _, job := range row.Queue {
		out.Queued = append(out.Queued, teamQueuedJobDTO{TicketID: job.TicketID, Repo: job.Repo, QueuedAt: job.CreatedAt})
	}
	return out
}

// runnerDoing is the runner card's one line: the signal its worker reported,
// or what its state implies when it reported none.
func runnerDoing(runner app.TeamRunner) string {
	if runner.Signal != "" {
		return runner.Signal
	}
	switch runner.State {
	case app.RunnerWorking:
		return "Working"
	case app.RunnerWaiting:
		return "Waiting on a build host"
	}
	return "No ticket held"
}

func nonZeroTime(at time.Time) *time.Time {
	if at.IsZero() {
		return nil
	}
	return &at
}
