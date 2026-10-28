package httpapi

import "net/http"

func (s *server) listTokens(w http.ResponseWriter, r *http.Request, _ caller) error {
	tokens, err := s.deps.Auth.ListAPITokens(r.Context())
	if err != nil {
		return err
	}
	out := make([]tokenDTO, 0, len(tokens))
	for _, token := range tokens {
		out = append(out, toToken(token))
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *server) createToken(w http.ResponseWriter, r *http.Request, who caller) error {
	var body struct {
		Name       string `json:"name"`
		IsDelegate bool   `json:"is_delegate"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	token, secret, err := s.deps.Auth.CreateAPIToken(r.Context(), who.user, body.Name, body.IsDelegate)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, createdTokenDTO{tokenDTO: toToken(token), Secret: secret})
	return nil
}

func (s *server) revokeToken(w http.ResponseWriter, r *http.Request, _ caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if err := s.deps.Auth.RevokeAPIToken(r.Context(), id); err != nil {
		return err
	}
	writeNoContent(w)
	return nil
}
