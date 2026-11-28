package httpapi

import "net/http"

// forgeCredentialDTO is the only response that carries a provider token in
// plain text. It is REST-only (never an MCP tool, so a model cannot ask for it
// through its tool list) and is never cached.
type forgeCredentialDTO struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	CloneURL string `json:"clone_url"`
	Account  string `json:"account"`
	Token    string `json:"token"`
}

// forgeCredential lends a board worker the Forgejo token for one repo.
func (s *server) forgeCredential(w http.ResponseWriter, r *http.Request, who caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	credential, err := s.deps.Connections.AgentCredentialFor(r.Context(), who.actor, id)
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, forgeCredentialDTO{
		Provider: string(credential.Provider), BaseURL: credential.BaseURL, CloneURL: credential.CloneURL,
		Account: credential.Account, Token: credential.Token.Reveal(),
	})
	return nil
}
