package httpapi

import (
	"net/http"
	"strconv"
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
	// BudgetOff means the owner lifted the daily budget; RemainingUSD is then informational.
	BudgetOff bool `json:"budget_off"`
	// CanSpend is whether the manager may start a model session now.
	CanSpend bool `json:"can_spend"`
}

type managerFlagsDTO struct {
	Flags []domain.ManagerFlag `json:"flags"`
}

func (s *server) managerRoutes(rt *router) {
	// The board manager polls these every few minutes; people read them on the Manager page.
	rt.anyone("GET /api/v1/manager/status", s.managerStatus)
	rt.anyone("GET /api/v1/manager/flags", s.managerFlags)
	// CI sensors call the repos' providers, so they have their own endpoint and cost.
	rt.anyone("GET /api/v1/manager/ci-flags", s.managerCIFlags)
	rt.anyone("POST /api/v1/manager/usage", s.managerUsage)
	rt.anyone("GET /api/v1/manager/sessions", s.managerSessions)
	// The manager loop reports when a session starts and ends; the Team view draws it.
	rt.anyone("POST /api/v1/manager/activity", s.managerActivity)
	// The manager loop expires its own stale depends_on links every pass.
	rt.anyone("POST /api/v1/manager/expire-links", s.managerExpireLinks)
}

func (s *server) managerExpireLinks(w http.ResponseWriter, r *http.Request, who caller) error {
	removed, err := s.deps.Board.ExpireManagerLinks(r.Context(), who.actor)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]int{"removed": removed})
	return nil
}

func toManagerStatus(status app.ManagerStatus) managerStatusDTO {
	return managerStatusDTO{Enabled: status.Settings.ManagerEnabled, BuildCap: status.Settings.ManagerBuildCap,
		StallMinutes: status.Settings.ManagerStallMinutes, DailyBudgetUSD: status.Settings.ManagerDailyBudgetUSD,
		Today: status.Today, RemainingUSD: status.RemainingUSD, BudgetOff: status.Settings.ManagerBudgetOff, CanSpend: status.CanSpend()}
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

type managerCIFlagsDTO struct {
	Flags []domain.ManagerFlag `json:"flags"`
	// Notes name what the board could not read (a refused token, a missing
	// permission), for the loop's log.
	Notes []string `json:"notes"`
}

func (s *server) managerCIFlags(w http.ResponseWriter, r *http.Request, who caller) error {
	scope, err := parseRepoScope(r.URL.Query().Get("repo"))
	if err != nil {
		return err
	}
	queueLimit, err := parseQueueMinutes(r.URL.Query().Get("queue_minutes"))
	if err != nil {
		return err
	}
	if s.deps.CI == nil {
		writeJSON(w, http.StatusOK, managerCIFlagsDTO{Flags: []domain.ManagerFlag{}, Notes: []string{}})
		return nil
	}
	result, err := s.deps.CI.Flags(r.Context(), who.actor, scope, queueLimit)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, managerCIFlagsDTO{Flags: nonNil(result.Flags), Notes: nonNil(result.Notes)})
	return nil
}

// parseQueueMinutes reads the CI queue limit; empty means the board default.
func parseQueueMinutes(raw string) (time.Duration, error) {
	if raw == "" {
		return 0, nil
	}
	minutes, err := strconv.Atoi(raw)
	if err != nil {
		return 0, domain.Invalidf("queue_minutes must be a whole number of minutes")
	}
	return time.Duration(minutes) * time.Minute, nil
}

func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
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
