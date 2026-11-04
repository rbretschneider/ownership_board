package httpapi

import (
	"net/http"
	"strconv"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
	"github.com/rbretschneider/ownership_board/internal/stats"
)

func (s *server) statsRoutes(rt *router) {
	// The product owner's view; agents have no use for it.
	rt.human("GET /api/v1/stats", s.stats)
}

// stats reports delivery metrics. Query: days (1-365, default 30) and
// tz_offset (minutes east of UTC, default 0).
func (s *server) stats(w http.ResponseWriter, r *http.Request, _ caller) error {
	query := r.URL.Query()
	days, err := optionalInt(query.Get("days"), "days", stats.DefaultWindowDays)
	if err != nil {
		return err
	}
	offset, err := optionalInt(query.Get("tz_offset"), "tz_offset", 0)
	if err != nil {
		return err
	}
	report, err := s.deps.Board.Stats(r.Context(), app.StatsQuery{Days: days, OffsetMinutes: offset})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, report)
	return nil
}

func optionalInt(raw, name string, fallback int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, domain.Invalidf("%s must be a whole number", name)
	}
	return value, nil
}
