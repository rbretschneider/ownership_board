package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"unicode/utf8"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

func (s *server) repoRoutes(rt *router) {
	rt.anyone("GET /api/v1/repos", s.listRepos)
	rt.human("POST /api/v1/repos", s.createRepo)
	rt.human("PATCH /api/v1/repos/{id}", s.restyleRepo)
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
		OverlapIgnore    json.RawMessage        `json:"overlap_ignore"`
		BuildTargets     json.RawMessage        `json:"build_targets"`
		Color            *string                `json:"color"`
		TreatAsPublic    *bool                  `json:"treat_as_public"`
		Description      *string                `json:"description"`
		RequireSpec      *bool                  `json:"require_spec_approval"`
		RequireDesign    *bool                  `json:"require_design_approval"`
		AllowAgentMerge  *bool                  `json:"allow_agent_merge"`
		HealthCheckEvery *int                   `json:"health_check_every"`
		DeployMode       *domain.DeployMode     `json:"deploy_mode"`
		ReviewBlocking   *domain.ReviewBlocking `json:"review_blocking"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	var targets *[]domain.RepoBuildTarget
	if body.BuildTargets != nil {
		var entries []json.RawMessage
		if err := json.Unmarshal(body.BuildTargets, &entries); err != nil || entries == nil {
			return domain.Invalidf("build_targets must be an array")
		}
		parsed := make([]domain.RepoBuildTarget, 0, len(entries))
		for _, entry := range entries {
			var item struct {
				OS           *domain.BuildHostOS `json:"os"`
				Preset       *string             `json:"preset"`
				Capabilities *[]*string          `json:"capabilities"`
			}
			decoder := json.NewDecoder(bytes.NewReader(entry))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&item); err != nil || item.OS == nil || item.Preset == nil || item.Capabilities == nil {
				return domain.Invalidf("each build target requires os, preset and capabilities")
			}
			caps := make([]string, 0, len(*item.Capabilities))
			for _, c := range *item.Capabilities {
				if c == nil {
					return domain.Invalidf("capabilities must contain strings")
				}
				caps = append(caps, *c)
			}
			parsed = append(parsed, domain.RepoBuildTarget{OS: *item.OS, Preset: *item.Preset, Capabilities: caps})
		}
		targets = &parsed
	}
	var overlapIgnore *[]string
	if body.OverlapIgnore != nil {
		if !utf8.Valid(body.OverlapIgnore) {
			return domain.Invalidf("overlap_ignore must contain valid UTF-8")
		}
		var entries []*string
		if err := json.Unmarshal(body.OverlapIgnore, &entries); err != nil || entries == nil {
			return domain.Invalidf("overlap_ignore must be an array of strings")
		}
		parsed := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry == nil {
				return domain.Invalidf("overlap_ignore must contain strings")
			}
			parsed = append(parsed, *entry)
		}
		overlapIgnore = &parsed
	}
	style := domain.RepoStyle{OverlapIgnore: overlapIgnore, BuildTargets: targets,
		Color: body.Color, Description: body.Description, TreatAsPublic: body.TreatAsPublic, DeployMode: body.DeployMode,
		RequireSpecApproval: body.RequireSpec, RequireDesignApproval: body.RequireDesign,
		AllowAgentMerge: body.AllowAgentMerge, HealthCheckEvery: body.HealthCheckEvery, ReviewBlocking: body.ReviewBlocking,
	}
	repo, err := s.deps.Repos.Restyle(r.Context(), who.actor, id, style)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toRepo(repo))
	return nil
}
