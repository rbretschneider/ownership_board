package httpapi

import (
	"net/http"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

type settingsDTO struct {
	WindowsParallelBuilds     int             `json:"windows_parallel_builds"`
	WindowsParallelBuildLimit int             `json:"windows_parallel_build_limit"`
	WindowsRunnerEnabled      bool            `json:"windows_runner_enabled"`
	CodexWorkers              int             `json:"codex_workers"`
	CodexWorkerLimit          int             `json:"codex_worker_limit"`
	ClaudeWorkers             int             `json:"claude_workers"`
	ClaudeWorkerLimit         int             `json:"claude_worker_limit"`
	CodexWeeklyLimit          int             `json:"codex_weekly_limit"`
	ClaudeWeeklyLimit         int             `json:"claude_weekly_limit"`
	AutonomousInFlightCap     int             `json:"autonomous_in_flight_cap"`
	ReviewBy                  domain.ReviewBy `json:"review_by"`
	ReviewFreshContext        bool            `json:"review_fresh_context"`
	ReviewAdversarial         bool            `json:"review_adversarial"`
}

func (s *server) settingsRoutes(rt *router) {
	rt.anyone("GET /api/v1/settings", s.settings)
	// Only people change settings; the service refuses agents as well.
	rt.human("PATCH /api/v1/settings", s.editSettings)
}

func (s *server) settings(w http.ResponseWriter, r *http.Request, _ caller) error {
	settings, err := s.deps.Board.Settings(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toSettings(settings))
	return nil
}

func (s *server) editSettings(w http.ResponseWriter, r *http.Request, who caller) error {
	var body struct {
		WindowsParallelBuilds *int             `json:"windows_parallel_builds"`
		WindowsRunnerEnabled  *bool            `json:"windows_runner_enabled"`
		CodexWorkers          *int             `json:"codex_workers"`
		ClaudeWorkers         *int             `json:"claude_workers"`
		CodexWeeklyLimit      *int             `json:"codex_weekly_limit"`
		ClaudeWeeklyLimit     *int             `json:"claude_weekly_limit"`
		AutonomousInFlightCap *int             `json:"autonomous_in_flight_cap"`
		ReviewBy              *domain.ReviewBy `json:"review_by"`
		ReviewFreshContext    *bool            `json:"review_fresh_context"`
		ReviewAdversarial     *bool            `json:"review_adversarial"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	settings, err := s.deps.Board.UpdateSettings(r.Context(), who.actor, domain.SettingsEdit{WindowsParallelBuilds: body.WindowsParallelBuilds, AutonomousInFlightCap: body.AutonomousInFlightCap, CodexWorkers: body.CodexWorkers, ClaudeWorkers: body.ClaudeWorkers, CodexWeeklyLimit: body.CodexWeeklyLimit, ClaudeWeeklyLimit: body.ClaudeWeeklyLimit, WindowsRunnerEnabled: body.WindowsRunnerEnabled, ReviewBy: body.ReviewBy, ReviewFreshContext: body.ReviewFreshContext, ReviewAdversarial: body.ReviewAdversarial})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toSettings(settings))
	return nil
}

func toSettings(settings domain.BoardSettings) settingsDTO {
	return settingsDTO{WindowsParallelBuilds: settings.WindowsParallelBuilds, WindowsParallelBuildLimit: settings.WindowsParallelBuildLimit, AutonomousInFlightCap: settings.AutonomousInFlightCap, CodexWorkers: settings.CodexWorkers, ClaudeWorkers: settings.ClaudeWorkers, ClaudeWorkerLimit: domain.MaxClaudeWorkers, CodexWeeklyLimit: settings.CodexWeeklyLimit, ClaudeWeeklyLimit: settings.ClaudeWeeklyLimit, CodexWorkerLimit: domain.MaxCodexWorkers, WindowsRunnerEnabled: settings.WindowsRunnerEnabled,
		ReviewBy: settings.ReviewBy, ReviewFreshContext: settings.ReviewFreshContext, ReviewAdversarial: settings.ReviewAdversarial}
}
