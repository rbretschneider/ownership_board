package httpapi

import (
	"net/http"

	"github.com/rbretschneider/ownership_board/internal/app"
)

func (s *server) agentRoutes(rt *router) {
	rt.agentOnly("POST /api/v1/agent/claim", s.claim)
	rt.agentOnly("POST /api/v1/agent/tickets/{id}/heartbeat", s.heartbeat)
	rt.agentOnly("POST /api/v1/agent/tickets/{id}/release", s.release)
	rt.agentOnly("GET /api/v1/agent/tickets/{id}/brief", s.brief)
	rt.agentOnly("POST /mcp", s.mcp)
	rt.agentOnly("GET /mcp", s.mcpStreamUnsupported)
	rt.agentOnly("DELETE /mcp", s.mcpStreamUnsupported)
}

func (s *server) claim(w http.ResponseWriter, r *http.Request, who caller) error {
	var body struct {
		Repo         string `json:"repo"`
		WaitSeconds  int    `json:"wait_seconds"`
		LeaseMinutes int    `json:"lease_minutes"`
	}
	if err := decodeOptionalJSON(w, r, &body); err != nil {
		return err
	}
	input := app.ClaimInput{Repo: body.Repo, WaitSeconds: body.WaitSeconds, LeaseMinutes: body.LeaseMinutes}
	work, isClaimed, err := s.deps.Agents.Claim(r.Context(), who.actor, input)
	if err != nil {
		return err
	}
	if !isClaimed {
		writeNoContent(w)
		return nil
	}
	writeJSON(w, http.StatusOK, toWork(work))
	return nil
}

func (s *server) heartbeat(w http.ResponseWriter, r *http.Request, who caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var body struct {
		LeaseMinutes int `json:"lease_minutes"`
	}
	if err := decodeOptionalJSON(w, r, &body); err != nil {
		return err
	}
	detail, err := s.deps.Agents.Heartbeat(r.Context(), who.actor, id, body.LeaseMinutes)
	return respondDetail(w, http.StatusOK, detail, err)
}

func (s *server) release(w http.ResponseWriter, r *http.Request, who caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	detail, err := s.deps.Agents.Release(r.Context(), who.actor, id)
	return respondDetail(w, http.StatusOK, detail, err)
}

func (s *server) brief(w http.ResponseWriter, r *http.Request, _ caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	brief, err := s.deps.Agents.Brief(r.Context(), id)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	_, err = w.Write([]byte(brief))
	return err
}

func (s *server) mcp(w http.ResponseWriter, r *http.Request, who caller) error {
	s.deps.MCP.ServeMCP(w, r, who.actor)
	return nil
}

// mcpStreamUnsupported answers the optional server-to-client stream: the board
// only replies to requests, which the MCP spec allows with 405.
func (s *server) mcpStreamUnsupported(w http.ResponseWriter, _ *http.Request, _ caller) error {
	w.Header().Set("Allow", http.MethodPost)
	writeErrorCode(w, http.StatusMethodNotAllowed, codeBadRequest, "this MCP server does not open a server-sent event stream")
	return nil
}
