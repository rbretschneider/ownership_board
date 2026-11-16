package httpapi

import (
	"net/http"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

func (s *server) splitRoutes(rt *router) {
	rt.anyone("POST /api/v1/tickets/{id}/split", s.splitTicket)
}

// splitRequest mirrors the split_ticket tool: an omitted "after" means the
// child waits for the one before it, and [] means it waits for none.
type splitRequest struct {
	Reason   string `json:"reason"`
	KeepSpec string `json:"keep_spec"`
	Children []struct {
		Title string `json:"title"`
		Kind  string `json:"kind"`
		Body  string `json:"body"`
		Spec  string `json:"spec"`
		After *[]int `json:"after"`
	} `json:"children"`
}

func (request splitRequest) draft() domain.SplitDraft {
	draft := domain.SplitDraft{Reason: request.Reason, KeepSpec: request.KeepSpec}
	for _, child := range request.Children {
		parsed := domain.SplitChildDraft{Title: child.Title, Kind: child.Kind, Body: child.Body, Spec: child.Spec}
		if child.After != nil {
			parsed.After, parsed.HasAfter = *child.After, true
		}
		draft.Children = append(draft.Children, parsed)
	}
	return draft
}

// splitResponse is the parent as it now stands and the children filed, in
// build order.
type splitResponse struct {
	Parent   ticketDetailDTO `json:"parent"`
	ChildIDs []int64         `json:"child_ids"`
}

func (s *server) splitTicket(w http.ResponseWriter, r *http.Request, who caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var body splitRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	result, err := s.deps.Board.Split(r.Context(), who.actor, id, body.draft())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, splitResponse{Parent: toDetail(result.Parent), ChildIDs: result.ChildIDs})
	return nil
}
