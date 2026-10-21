package httpapi

import (
	"net/http"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

func (s *server) repoRoutes(rt *router) {
	rt.anyone("GET /api/v1/repos", s.listRepos)
	rt.human("POST /api/v1/repos", s.createRepo)
	rt.human("PATCH /api/v1/repos/{id}", s.restyleRepo)
	rt.human("GET /api/v1/repo-sources", s.repoSources)
	rt.human("POST /api/v1/repo-sources/sync", s.syncRepos)
}

func (s *server) listRepos(w http.ResponseWriter, r *http.Request, _ caller) error {
	repos, err := s.deps.Repos.List(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toRepos(repos))
	return nil
}

func (s *server) createRepo(w http.ResponseWriter, r *http.Request, who caller) error {
	var body struct {
		FullName    string `json:"full_name"`
		URL         string `json:"url"`
		Description string `json:"description"`
		Color       string `json:"color"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	draft := domain.ManualRepoDraft{FullName: body.FullName, URL: body.URL, Description: body.Description, Color: body.Color}
	repo, err := s.deps.Repos.CreateManual(r.Context(), who.actor, draft)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, toRepo(repo))
	return nil
}

func (s *server) restyleRepo(w http.ResponseWriter, r *http.Request, who caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var body struct {
		Color       *string `json:"color"`
		Description *string `json:"description"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	repo, err := s.deps.Repos.Restyle(r.Context(), who.actor, id, domain.RepoStyle{Color: body.Color, Description: body.Description})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toRepo(repo))
	return nil
}

func (s *server) repoSources(w http.ResponseWriter, r *http.Request, _ caller) error {
	statuses, err := s.deps.Repos.Sources(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toSources(statuses))
	return nil
}

func (s *server) syncRepos(w http.ResponseWriter, r *http.Request, _ caller) error {
	statuses, err := s.deps.Repos.Sync(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toSources(statuses))
	return nil
}
