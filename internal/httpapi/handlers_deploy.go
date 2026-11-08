package httpapi

import "net/http"

func (s *server) requestDeploy(w http.ResponseWriter, r *http.Request, who caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if err := decodeOptionalJSON(w, r, &struct{}{}); err != nil {
		return err
	}
	detail, err := s.deps.Board.RequestDeploy(r.Context(), who.actor, id)
	return respondDetail(w, http.StatusOK, detail, err)
}
