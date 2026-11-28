package connections

import (
	"context"

	"github.com/rbretschneider/ownership_board/internal/domain"
	"github.com/rbretschneider/ownership_board/internal/reposource"
	"github.com/rbretschneider/ownership_board/internal/secretbox"
)

// AgentCredential is what a board agent needs to clone, push and open pull
// requests on one repo. Token is a Secret, so the value is safe to log.
type AgentCredential struct {
	Provider domain.Provider
	// BaseURL is the forge's root URL. The token must only ever go there, so
	// CloneURL is built from it rather than from the URL the forge reports
	// (Forgejo reports its ROOT_URL, which may be a name agents cannot reach).
	BaseURL  string
	CloneURL string
	// Account is the login the token belongs to.
	Account string
	Token   secretbox.Secret
}

// AgentCredential hands the Forgejo connection's token to a board worker for
// repo. It fails closed: only a non-delegate agent, only a Forgejo repo, and
// only when the owner turned on agent access for a connection stored in
// Settings. Server-env tokens are never handed out. GitHub repos keep using
// the workers' own token.
func (s *Service) AgentCredential(ctx context.Context, actor domain.Actor, repo domain.Repo) (AgentCredential, error) {
	if !actor.IsAgent() || actor.IsDelegate || actor.IsManager {
		return AgentCredential{}, domain.Forbiddenf("only a board worker's agent token can borrow a forge credential")
	}
	if repo.Provider != domain.ProviderForgejo {
		return AgentCredential{}, domain.Invalidf("repo %s is on %s; the board hands out credentials only for Forgejo repos", repo.FullName, repo.Provider)
	}
	if err := reposource.CheckRepoName(repo.FullName); err != nil {
		return AgentCredential{}, err
	}
	record, isStored, err := s.catalog.load(ctx, domain.ProviderForgejo)
	if err != nil {
		return AgentCredential{}, err
	}
	if err := checkAgentAccess(record, isStored); err != nil {
		return AgentCredential{}, err
	}
	s.logger.Info("forge credential handed to agent", "agent", actor.Name, "repo", repo.FullName, "host", record.BaseURL)
	return AgentCredential{
		Provider: domain.ProviderForgejo, BaseURL: record.BaseURL, CloneURL: record.BaseURL + "/" + repo.FullName + ".git",
		Account: record.Account, Token: record.AccessToken,
	}, nil
}

func checkAgentAccess(record Record, isStored bool) error {
	if !isStored {
		return domain.Conflictf("connect Forgejo in Settings > Repositories with agent access on; server-env tokens are never handed to agents")
	}
	if record.Status != StatusConnected {
		return domain.Conflictf("the Forgejo connection needs reconnecting in Settings > Repositories")
	}
	if !record.AllowAgentAccess {
		return domain.Forbiddenf("the owner has not allowed agents to use the Forgejo connection; turn it on in Settings > Repositories")
	}
	return nil
}
