package httpapi

import (
	"net/http"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func (s *server) agentRoutes(rt *router) {
	rt.agentOnly("POST /api/v1/agent/claim", s.claim)
	rt.agentOnly("POST /api/v1/agent/tickets/{id}/model", s.workModel)
	rt.agentOnly("POST /api/v1/agent/tickets/{id}/usage", s.recordUsage)
	rt.agentOnly("POST /api/v1/agent/tickets/{id}/heartbeat", s.heartbeat)
	rt.agentOnly("POST /api/v1/agent/tickets/{id}/release", s.release)
	rt.agentOnly("GET /api/v1/agent/tickets/{id}/brief", s.brief)
	rt.agentOnly("POST /api/v1/agent/tickets/{id}/pr-state", s.reportPRState)
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

func (s *server) reportPRState(w http.ResponseWriter, r *http.Request, who caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var body struct {
		State string `json:"state"`
		Note  string `json:"note"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	detail, err := s.deps.Board.ReportPullRequest(r.Context(), who.actor, id, app.PullRequestState(body.State), body.Note)
	return respondDetail(w, http.StatusOK, detail, err)
}

func (s *server) recordUsage(w http.ResponseWriter, r *http.Request, who caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var body struct {
		CallKey          string  `json:"call_key"`
		Provider         string  `json:"provider"`
		Model            string  `json:"model"`
		InputTokens      int64   `json:"input_tokens"`
		CacheWriteTokens int64   `json:"cache_write_tokens"`
		CacheReadTokens  int64   `json:"cache_read_tokens"`
		OutputTokens     int64   `json:"output_tokens"`
		CostUSD          float64 `json:"cost_usd"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	usage, err := s.deps.Agents.RecordUsage(r.Context(), who.actor, domain.UsageReport{
		TicketID: id, CallKey: body.CallKey, Provider: body.Provider, Model: body.Model,
		InputTokens: body.InputTokens, CacheWriteTokens: body.CacheWriteTokens,
		CacheReadTokens: body.CacheReadTokens, OutputTokens: body.OutputTokens, CostUSD: body.CostUSD,
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toUsage(usage))
	return nil
}

func (s *server) workModel(w http.ResponseWriter, r *http.Request, who caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var body struct {
		Model string `json:"model"`
	}
	if err := decodeOptionalJSON(w, r, &body); err != nil {
		return err
	}
	detail, err := s.deps.Agents.SetWorkModel(r.Context(), who.actor, id, body.Model)
	return respondDetail(w, http.StatusOK, detail, err)
}
