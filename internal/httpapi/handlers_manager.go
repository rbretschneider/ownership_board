package httpapi

import (
	"net/http"

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
		CostUSD float64 `json:"cost_usd"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	status, err := s.deps.Board.RecordManagerUsage(r.Context(), who.actor, body.CostUSD)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toManagerStatus(status))
	return nil
}
