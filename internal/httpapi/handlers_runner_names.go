package httpapi

import (
	"net/http"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// runnerNameRoutes serve the runners' friendly names. Anyone who may read the
// board may read them; only people rename a runner, like other settings.
func (s *server) runnerNameRoutes(rt *router) {
	rt.anyone("GET /api/v1/runner-names", s.runnerNames)
	rt.human("PATCH /api/v1/runner-names/{worker}", s.renameRunner)
}

// runnerNameDTO is one named runner. Worker is the raw worker id ("claude_1")
// that identities and leases keep using.
type runnerNameDTO struct {
	Worker      string     `json:"worker"`
	Family      string     `json:"family"`
	Position    int        `json:"position"`
	DisplayName string     `json:"display_name"`
	AssignedAt  *time.Time `json:"assigned_at"`
}

type runnerNamesDTO struct {
	Runners []runnerNameDTO `json:"runners"`
}

func (s *server) runnerNames(w http.ResponseWriter, r *http.Request, _ caller) error {
	names, err := s.deps.Board.RunnerNames(r.Context())
	if err != nil {
		return err
	}
	out := runnerNamesDTO{Runners: make([]runnerNameDTO, 0, len(names))}
	for _, name := range names {
		out.Runners = append(out.Runners, toRunnerName(name))
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *server) renameRunner(w http.ResponseWriter, r *http.Request, who caller) error {
	var body struct {
		DisplayName string `json:"display_name"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	renamed, err := s.deps.Board.RenameRunner(r.Context(), who.actor, r.PathValue("worker"), body.DisplayName)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toRunnerName(renamed))
	return nil
}

func toRunnerName(name domain.RunnerName) runnerNameDTO {
	return runnerNameDTO{
		Worker: name.Worker, Family: name.Family, Position: name.Position,
		DisplayName: name.Name, AssignedAt: nonZeroTime(name.AssignedAt),
	}
}
