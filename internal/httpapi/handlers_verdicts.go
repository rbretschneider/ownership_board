package httpapi

import (
	"net/http"
	"strconv"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// verdictRoutes serve the board's one record of whether a commit passed on a
// build target. Owner and agent auth like the board; recording a clean merge
// also needs the lease on the ticket (the service checks).
func (s *server) verdictRoutes(rt *router) {
	rt.anyone("GET /api/v1/verdicts", s.verdicts)
	rt.anyone("POST /api/v1/verdicts/clean-merge", s.reportCleanMerge)
}

type verdictsDTO struct {
	Verdicts []domain.Verdict `json:"verdicts"`
}

// verdicts looks verdicts up by commit (?repo=&sha=) or by ticket (?ticket=).
func (s *server) verdicts(w http.ResponseWriter, r *http.Request, _ caller) error {
	query := r.URL.Query()
	ticket, repo, sha := query.Get("ticket"), query.Get("repo"), query.Get("sha")
	var verdicts []domain.Verdict
	var err error
	switch {
	case ticket != "" && repo == "" && sha == "":
		id, parseErr := strconv.ParseInt(ticket, 10, 64)
		if parseErr != nil || id <= 0 {
			return domain.Invalidf("ticket must be a positive number")
		}
		verdicts, err = s.deps.Verdicts.ForTicket(r.Context(), id)
	case ticket == "" && repo != "" && sha != "":
		verdicts, err = s.deps.Verdicts.ForCommit(r.Context(), repo, sha)
	default:
		return domain.Invalidf("give either repo and sha, or ticket")
	}
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, verdictsDTO{Verdicts: nonNilVerdicts(verdicts)})
	return nil
}

func (s *server) reportCleanMerge(w http.ResponseWriter, r *http.Request, who caller) error {
	var report domain.CleanMergeReport
	if err := decodeJSON(w, r, &report); err != nil {
		return err
	}
	verdicts, err := s.deps.Verdicts.ReportCleanMerge(r.Context(), who.actor, report)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, verdictsDTO{Verdicts: nonNilVerdicts(verdicts)})
	return nil
}
