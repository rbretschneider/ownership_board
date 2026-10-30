package httpapi

import (
	"net/http"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

type settingsDTO struct {
	AutonomousInFlightCap int `json:"autonomous_in_flight_cap"`
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
		AutonomousInFlightCap *int `json:"autonomous_in_flight_cap"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	settings, err := s.deps.Board.UpdateSettings(r.Context(), who.actor, domain.SettingsEdit{AutonomousInFlightCap: body.AutonomousInFlightCap})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toSettings(settings))
	return nil
}

func toSettings(settings domain.BoardSettings) settingsDTO {
	return settingsDTO{AutonomousInFlightCap: settings.AutonomousInFlightCap}
}
