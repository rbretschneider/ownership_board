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
	OpenFlags     int    `json:"open_flags"`
	LatestSummary string `json:"latest_summary"`
	// LatestHeadline is the latest session's one line for the card.
	LatestHeadline string                  `json:"latest_headline"`
	Activity       teamActivityDTO         `json:"activity"`
	LatestAt       *time.Time              `json:"latest_at"`
	Sessions       []teamManagerSessionDTO `json:"sessions"`
}

// teamManagerSessionDTO is one history entry: the session plus its headline
// and whether its summary has a HEADLINE line (the structured format).
type teamManagerSessionDTO struct {
	domain.ManagerSession
	Headline     string `json:"headline"`
	IsStructured bool   `json:"structured"`
}

// teamActivityDTO is what the manager is acting on right now.
type teamActivityDTO struct {
	State     domain.ManagerActivityState `json:"state"`
	Kind      domain.ManagerActivityKind  `json:"kind"`
	TicketIDs []int64                     `json:"ticket_ids"`
	Host      string                      `json:"host"`
	StartedAt *time.Time                  `json:"started_at"`
}

// teamPRDTO is the merge sweep's view of a ticket's open pull request.
type teamPRDTO struct {
	URL          string `json:"url"`
	Verdict      string `json:"verdict"`
	Reason       string `json:"reason"`
	ChecksTotal  int    `json:"checks_total"`
	ChecksDone   int    `json:"checks_done"`
	ChecksFailed int    `json:"checks_failed"`
}

type teamRunnerDTO struct {
	Name string `json:"name"`
	// DisplayName is the runner's friendly name, empty when its family has no
	// pool; Name stays the raw identity that leases and timelines match on.
	DisplayName    string          `json:"display_name"`
	Family         string          `json:"family"`
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
	// PR is set when the merge sweep reported on the ticket's pull request.
	PR *teamPRDTO `json:"pr"`
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
	attachPRStatus(out.Tickets, readPRStatus(s.deps.PRStatusFile))
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
			Name: runner.Name, DisplayName: runner.DisplayName, Family: runner.Family, State: runner.State, Model: runner.Model, Doing: runnerDoing(runner),
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
	entries := make([]teamManagerSessionDTO, 0, len(sessions))
	for _, session := range sessions {
		entries = append(entries, teamManagerSessionDTO{ManagerSession: session,
			Headline: domain.ManagerHeadline(session.Summary), IsStructured: domain.HasManagerHeadline(session.Summary)})
	}
	out := teamManagerDTO{managerStatusDTO: toManagerStatus(manager.Status), OpenFlags: manager.OpenFlags, Sessions: entries, Activity: toTeamActivity(manager.Activity)}
	if len(sessions) > 0 {
		out.LatestSummary = sessions[0].Summary
		out.LatestHeadline = domain.ManagerHeadline(sessions[0].Summary)
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

func toTeamActivity(a domain.ManagerActivity) teamActivityDTO {
	out := teamActivityDTO{State: a.State, Kind: a.Kind, TicketIDs: a.TicketIDs, Host: a.Host, StartedAt: nonZeroTime(a.StartedAt)}
	if out.State == "" {
		out.State = domain.ManagerIdle
	}
	if out.TicketIDs == nil {
		out.TicketIDs = []int64{}
	}
	return out
}

// attachPRStatus adds the merge sweep's latest view of each ticket's pull
// request, counting its checks.
func attachPRStatus(tickets []teamTicketDTO, snapshot prStatusSnapshot) {
	byTicket := make(map[int64]prStatus, len(snapshot.PRs))
	for _, pr := range snapshot.PRs {
		byTicket[pr.TicketID] = pr
	}
	for i := range tickets {
		pr, ok := byTicket[tickets[i].ID]
		if !ok {
			continue
		}
		out := &teamPRDTO{URL: pr.URL, Verdict: pr.Verdict, Reason: pr.Reason, ChecksTotal: len(pr.Checks)}
		for _, check := range pr.Checks {
			if check.Status != "COMPLETED" {
				continue
			}
			out.ChecksDone++
			switch check.Conclusion {
			case "FAILURE", "TIMED_OUT", "CANCELLED", "STARTUP_FAILURE", "ERROR", "ACTION_REQUIRED":
				out.ChecksFailed++
			}
		}
		tickets[i].PR = out
	}
}
