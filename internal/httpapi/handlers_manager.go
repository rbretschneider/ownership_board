package httpapi

import (
	"net/http"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

type managerStatusDTO struct {
	Enabled        bool                `json:"enabled"`
	BuildCap       int                 `json:"build_cap"`
	StallMinutes   int                 `json:"stall_minutes"`
	DailyBudgetUSD int                 `json:"daily_budget_usd"`
	Today          domain.ManagerUsage `json:"today"`
	RemainingUSD   float64             `json:"remaining_usd"`
}

type managerFlagsDTO struct {
	Flags []domain.ManagerFlag `json:"flags"`
}

func (s *server) managerRoutes(rt *router) {
	// The board manager polls these every few minutes; people read them on the Manager page.
	rt.anyone("GET /api/v1/manager/status", s.managerStatus)
	rt.anyone("GET /api/v1/manager/flags", s.managerFlags)
	rt.anyone("POST /api/v1/manager/usage", s.managerUsage)
	rt.anyone("GET /api/v1/manager/sessions", s.managerSessions)
	// The manager loop reports when a session starts and ends; the Team view draws it.
	rt.anyone("POST /api/v1/manager/activity", s.managerActivity)
}

func toManagerStatus(status app.ManagerStatus) managerStatusDTO {
	return managerStatusDTO{Enabled: status.Settings.ManagerEnabled, BuildCap: status.Settings.ManagerBuildCap,
		StallMinutes: status.Settings.ManagerStallMinutes, DailyBudgetUSD: status.Settings.ManagerDailyBudgetUSD,
		Today: status.Today, RemainingUSD: status.RemainingUSD}
}

func (s *server) managerStatus(w http.ResponseWriter, r *http.Request, who caller) error {
	status, err := s.deps.Board.ManagerStatus(r.Context(), who.actor)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toManagerStatus(status))
	return nil
}

func (s *server) managerFlags(w http.ResponseWriter, r *http.Request, who caller) error {
	scope, err := parseRepoScope(r.URL.Query().Get("repo"))
	if err != nil {
		return err
	}
	flags, err := s.deps.Board.ManagerFlags(r.Context(), who.actor, scope)
	if err != nil {
		return err
	}
	if flags == nil {
		flags = []domain.ManagerFlag{}
	}
	writeJSON(w, http.StatusOK, managerFlagsDTO{Flags: flags})
	return nil
}

func (s *server) managerUsage(w http.ResponseWriter, r *http.Request, who caller) error {
	var body struct {
		CostUSD  float64 `json:"cost_usd"`
		Model    string  `json:"model"`
		Summary  string  `json:"summary"`
		Headline string  `json:"headline"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	status, err := s.deps.Board.RecordManagerUsage(r.Context(), who.actor, app.ManagerSessionInput{CostUSD: body.CostUSD, Model: body.Model, Summary: body.Summary, Headline: body.Headline})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toManagerStatus(status))
	return nil
}

func (s *server) managerSessions(w http.ResponseWriter, r *http.Request, who caller) error {
	sessions, err := s.deps.Board.ManagerSessions(r.Context(), who.actor, 20)
	if err != nil {
		return err
	}
	if sessions == nil {
		sessions = []domain.ManagerSession{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
	return nil
}

func (s *server) managerActivity(w http.ResponseWriter, r *http.Request, who caller) error {
	var body struct {
		State     string     `json:"state"`
		Kind      string     `json:"kind"`
		TicketIDs []int64    `json:"ticket_ids"`
		Host      string     `json:"host"`
		StartedAt *time.Time `json:"started_at"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	activity, err := s.deps.Board.ReportManagerActivity(r.Context(), who.actor, app.ManagerActivityInput{
		State: body.State, Kind: body.Kind, TicketIDs: body.TicketIDs, Host: body.Host, StartedAt: body.StartedAt,
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toTeamActivity(activity))
	return nil
}
