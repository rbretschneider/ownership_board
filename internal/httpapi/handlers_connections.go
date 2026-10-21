package httpapi

import (
	"net/http"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/connections"
	"github.com/rbretschneider/ownership_board/internal/domain"
	"github.com/rbretschneider/ownership_board/internal/reposource"
)

// Connection endpoints are person routes; the service also requires the admin
// flag. None of them is offered to agents, over REST or MCP.
func (s *server) connectionRoutes(rt *router) {
	rt.human("GET /api/v1/connections", s.listConnections)
	rt.human("POST /api/v1/connections/sync", s.syncConnections)
	rt.human("POST /api/v1/connections/github/device", s.startGitHub)
	rt.human("POST /api/v1/connections/github/device/{flow}/poll", s.pollGitHub)
	rt.human("POST /api/v1/connections/github/device/{flow}/confirm", s.confirmGitHub)
	rt.human("DELETE /api/v1/connections/github/device/{flow}", s.cancelGitHub)
	rt.human("PUT /api/v1/connections/forgejo", s.connectForgejo)
	rt.human("PUT /api/v1/connections/bitbucket", s.connectBitbucket)
	rt.human("DELETE /api/v1/connections/{provider}", s.disconnect)
}

type connectionDTO struct {
	Provider    domain.Provider        `json:"provider"`
	Label       string                 `json:"label"`
	IsAvailable bool                   `json:"is_available"`
	IsConnected bool                   `json:"is_connected"`
	Status      connections.Status     `json:"status"`
	ManagedBy   *connections.ManagedBy `json:"managed_by"`
	Account     string                 `json:"account"`
	BaseURL     string                 `json:"base_url"`
	Email       string                 `json:"email"`
	ConnectedAt *time.Time             `json:"connected_at"`
	RepoCount   int                    `json:"repo_count"`
	LastSync    *lastSyncDTO           `json:"last_sync"`
}

type lastSyncDTO struct {
	At    time.Time `json:"at"`
	Error string    `json:"error"`
}

type deviceFlowDTO struct {
	FlowID          string    `json:"flow_id"`
	UserCode        string    `json:"user_code"`
	VerificationURI string    `json:"verification_uri"`
	ExpiresAt       time.Time `json:"expires_at"`
	IntervalSeconds int       `json:"interval_seconds"`
}

type devicePollDTO struct {
	Status          string `json:"status"`
	IntervalSeconds int    `json:"interval_seconds"`
	Login           string `json:"login"`
	Replaces        string `json:"replaces"`
	Message         string `json:"message"`
}

func (s *server) listConnections(w http.ResponseWriter, r *http.Request, who caller) error {
	views, err := s.deps.Connections.List(r.Context(), adminOf(who))
	return respondConnections(w, views, err)
}

func (s *server) syncConnections(w http.ResponseWriter, r *http.Request, who caller) error {
	views, err := s.deps.Connections.SyncAll(r.Context(), adminOf(who))
	return respondConnections(w, views, err)
}

func (s *server) startGitHub(w http.ResponseWriter, r *http.Request, who caller) error {
	flow, err := s.deps.Connections.StartGitHub(r.Context(), adminOf(who))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, deviceFlowDTO{
		FlowID: flow.ID, UserCode: flow.UserCode, VerificationURI: reposource.GitHubDeviceVerifyURL,
		ExpiresAt: flow.ExpiresAt, IntervalSeconds: int(flow.Interval.Seconds()),
	})
	return nil
}

func (s *server) pollGitHub(w http.ResponseWriter, r *http.Request, who caller) error {
	poll, err := s.deps.Connections.PollGitHub(r.Context(), adminOf(who), r.PathValue("flow"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, devicePollDTO{
		Status: poll.Status, IntervalSeconds: int(poll.Interval.Seconds()), Login: poll.Login, Replaces: poll.Replaces, Message: poll.Message,
	})
	return nil
}

func (s *server) confirmGitHub(w http.ResponseWriter, r *http.Request, who caller) error {
	view, err := s.deps.Connections.ConfirmGitHub(r.Context(), adminOf(who), r.PathValue("flow"))
	return respondConnection(w, view, err)
}

func (s *server) cancelGitHub(w http.ResponseWriter, r *http.Request, who caller) error {
	if err := s.deps.Connections.CancelGitHub(adminOf(who), r.PathValue("flow")); err != nil {
		return err
	}
	writeNoContent(w)
	return nil
}

func (s *server) connectForgejo(w http.ResponseWriter, r *http.Request, who caller) error {
	var body struct {
		BaseURL string `json:"base_url"`
		Token   string `json:"token"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	view, err := s.deps.Connections.ConnectForgejo(r.Context(), adminOf(who), body.BaseURL, body.Token)
	return respondConnection(w, view, err)
}

func (s *server) connectBitbucket(w http.ResponseWriter, r *http.Request, who caller) error {
	var body struct {
		Email string `json:"email"`
		Token string `json:"token"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	view, err := s.deps.Connections.ConnectBitbucket(r.Context(), adminOf(who), body.Email, body.Token)
	return respondConnection(w, view, err)
}

func (s *server) disconnect(w http.ResponseWriter, r *http.Request, who caller) error {
	provider := domain.Provider(r.PathValue("provider"))
	if err := s.deps.Connections.Disconnect(r.Context(), adminOf(who), provider); err != nil {
		return err
	}
	writeNoContent(w)
	return nil
}

func adminOf(who caller) connections.Admin {
	return connections.Admin{ID: who.user.ID, Name: who.user.Name, IsAdmin: who.user.IsAdmin}
}

func respondConnections(w http.ResponseWriter, views []connections.View, err error) error {
	if err != nil {
		return err
	}
	out := make([]connectionDTO, 0, len(views))
	for _, view := range views {
		out = append(out, toConnection(view))
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func respondConnection(w http.ResponseWriter, view connections.View, err error) error {
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toConnection(view))
	return nil
}

func toConnection(view connections.View) connectionDTO {
	dto := connectionDTO{
		Provider: view.Provider, Label: view.Label, IsAvailable: view.IsAvailable, Status: view.Status, IsConnected: view.Status == connections.StatusConnected,
		Account: view.Account, BaseURL: view.BaseURL, Email: view.Email, ConnectedAt: view.ConnectedAt,
		RepoCount: view.RepoCount, LastSync: toLastSync(view.LastSync),
	}
	if view.ManagedBy != "" {
		managedBy := view.ManagedBy
		dto.ManagedBy = &managedBy
	}
	return dto
}

func toLastSync(outcome *app.SyncOutcome) *lastSyncDTO {
	if outcome == nil {
		return nil
	}
	return &lastSyncDTO{At: outcome.At, Error: outcome.Error}
}
