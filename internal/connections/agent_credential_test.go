package connections_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/connections"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

const forgejoCanary = "fj_AGENT_CANARY"

// newFakeForgejo answers the two calls a Forgejo connection makes. html_url
// deliberately uses another host, as a Forgejo whose ROOT_URL differs from the
// address the board reaches it at would.
func newFakeForgejo(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token "+forgejoCanary {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/api/v1/user":
			_, _ = w.Write([]byte(`{"id":7,"login":"bot"}`))
		case r.URL.Path == "/api/v1/user/repos" && r.URL.Query().Get("page") == "1":
			_, _ = w.Write([]byte(`[{"full_name":"team/app","html_url":"https://git.public.example/team/app","default_branch":"main"}]`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func (h *harness) connectForgejo(baseURL string, allowAgents bool) domain.Repo {
	h.t.Helper()
	input := connections.ForgejoInput{BaseURL: baseURL, Token: forgejoCanary, AllowAgentAccess: allowAgents}
	view, err := h.service.ConnectForgejo(h.ctx, h.admin, input)
	if err != nil {
		h.t.Fatalf("ConnectForgejo: %v", err)
	}
	if view.AllowAgentAccess != allowAgents {
		h.t.Fatalf("view.AllowAgentAccess = %v, want %v", view.AllowAgentAccess, allowAgents)
	}
	repos, err := h.repos.List(h.ctx)
	if err != nil || len(repos) != 1 || repos[0].Provider != domain.ProviderForgejo {
		h.t.Fatalf("synced repos = %+v, %v; want the one Forgejo repo", repos, err)
	}
	return repos[0]
}

var worker = domain.Actor{Kind: domain.ActorAgent, ID: 3, Name: "@worker/3/w1"}

func TestAgentCredentialIsHandedOutWhenTheOwnerAllowsIt(t *testing.T) {
	h := newHarness(t, connections.Policy{})
	forge := newFakeForgejo(t)
	repo := h.connectForgejo(forge.URL, true)

	credential, err := h.service.AgentCredential(h.ctx, worker, repo)
	if err != nil {
		t.Fatalf("AgentCredential: %v", err)
	}
	if credential.Token.Reveal() != forgejoCanary || credential.Account != "bot" || credential.BaseURL != forge.URL {
		t.Fatalf("credential = %+v", credential)
	}
	// Built from the address the board reaches, not the forge's reported ROOT_URL.
	if credential.CloneURL != forge.URL+"/team/app.git" {
		t.Fatalf("CloneURL = %q", credential.CloneURL)
	}
}

func TestAgentCredentialFailsClosed(t *testing.T) {
	h := newHarness(t, connections.Policy{})
	forge := newFakeForgejo(t)
	repo := h.connectForgejo(forge.URL, false)
	delegate := worker
	delegate.IsDelegate = true
	person := domain.Actor{Kind: domain.ActorHuman, ID: 1, Name: "Owner"}
	github := repo
	github.Provider = domain.ProviderGitHub
	crafted := repo
	crafted.FullName = "team/../admin"

	cases := map[string]struct {
		actor domain.Actor
		repo  domain.Repo
		want  domain.ErrorKind
	}{
		"agent access off": {worker, repo, domain.KindForbidden},
		"delegate token":   {delegate, repo, domain.KindForbidden},
		"a person":         {person, repo, domain.KindForbidden},
		"github repo":      {worker, github, domain.KindInvalid},
		"crafted name":     {worker, crafted, domain.KindInvalid},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := h.service.AgentCredential(h.ctx, tc.actor, tc.repo)
			var domainErr *domain.Error
			if !errors.As(err, &domainErr) || domainErr.Kind != tc.want {
				t.Fatalf("err = %v, want kind %v", err, tc.want)
			}
		})
	}
}

func TestAgentCredentialNeedsAStoredConnection(t *testing.T) {
	h := newHarness(t, connections.Policy{})
	repo := domain.Repo{Provider: domain.ProviderForgejo, FullName: "team/app"}
	_, err := h.service.AgentCredential(h.ctx, worker, repo)
	if err == nil || !strings.Contains(err.Error(), "server-env tokens are never handed to agents") {
		t.Fatalf("err = %v", err)
	}
}
