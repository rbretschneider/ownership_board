package httpapi

import (
	"net/http"

	"github.com/rbretschneider/ownership_board/internal/app"
)

// quarantineRoutes serve a repo's flaky-test quarantine list. Anyone signed in
// may read it; a person or a delegate agent may add or release a test.
func (s *server) quarantineRoutes(rt *router) {
	rt.anyone("GET /api/v1/repos/{id}/quarantine", s.listQuarantine)
	rt.anyone("POST /api/v1/repos/{id}/quarantine", s.addQuarantine)
	rt.anyone("POST /api/v1/repos/{id}/quarantine/release", s.releaseQuarantine)
}

func (s *server) listQuarantine(w http.ResponseWriter, r *http.Request, _ caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	list, err := s.deps.BuildJobs.QuarantineList(r.Context(), id)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"tests": list})
	return nil
}

func (s *server) addQuarantine(w http.ResponseWriter, r *http.Request, who caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var body struct {
		Test        string `json:"test"`
		Reason      string `json:"reason"`
		FixTicketID *int64 `json:"fix_ticket_id"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	q, err := s.deps.BuildJobs.Quarantine(r.Context(), who.actor, app.QuarantineInput{RepoID: id, Test: body.Test, Reason: body.Reason, FixTicketID: body.FixTicketID})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, q)
	return nil
}

func (s *server) releaseQuarantine(w http.ResponseWriter, r *http.Request, who caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var body struct {
		Test string `json:"test"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	q, err := s.deps.BuildJobs.ReleaseQuarantine(r.Context(), who.actor, id, body.Test)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, q)
	return nil
}
