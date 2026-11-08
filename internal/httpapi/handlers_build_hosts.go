package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

type buildHostDTO struct {
	Online        bool                   `json:"online"`
	RunningJobIDs []int64                `json:"running_job_ids"`
	FreeDiskBytes *int64                 `json:"free_disk_bytes"`
	ID            int64                  `json:"id"`
	Name          string                 `json:"name"`
	OS            domain.BuildHostOS     `json:"os"`
	Arch          string                 `json:"arch"`
	Status        domain.BuildHostStatus `json:"status"`
	Capabilities  []string               `json:"capabilities"`
	MaxParallel   int                    `json:"max_parallel"`
	Enabled       bool                   `json:"enabled"`
	PublicKey     string                 `json:"public_key"`
	Facts         json.RawMessage        `json:"facts"`
	AgentVersion  string                 `json:"agent_version"`
	LastSeenAt    *time.Time             `json:"last_seen_at"`
	CreatedAt     time.Time              `json:"created_at"`
	ApprovedAt    *time.Time             `json:"approved_at"`
	ApprovedBy    *int64                 `json:"approved_by"`
	RevokedAt     *time.Time             `json:"revoked_at"`
}

func toBuildHost(h domain.BuildHost) buildHostDTO {
	jobs := h.RunningJobIDs
	if jobs == nil {
		jobs = []int64{}
	}
	return buildHostDTO{RunningJobIDs: jobs, FreeDiskBytes: h.FreeDiskBytes, ID: h.ID, Name: h.Name, OS: h.OS, Arch: h.Arch, Status: h.Status, Capabilities: h.Capabilities, MaxParallel: h.MaxParallel, Enabled: h.Enabled, PublicKey: h.PublicKey, Facts: h.Facts, AgentVersion: h.AgentVersion, LastSeenAt: h.LastSeenAt, CreatedAt: h.CreatedAt, ApprovedAt: h.ApprovedAt, ApprovedBy: h.ApprovedBy, RevokedAt: h.RevokedAt}
}

func (s *server) buildHostRoutes(rt *router) {
	rt.machinePublic("POST /api/v1/build-host/pair", s.pairBuildHost)
	rt.signedHost("GET /api/v1/build-host/status", true, s.buildHostStatus)
	rt.signedHost("POST /api/v1/build-host/heartbeat", false, s.buildHostHeartbeat)
	rt.human("GET /api/v1/build-hosts/{id}/events", s.buildHostEvents)
	rt.human("GET /api/v1/build-hosts", s.listBuildHosts)
	rt.human("GET /api/v1/build-hosts/{id}", s.getBuildHost)
	rt.human("POST /api/v1/build-hosts/pairing-codes", s.createBuildHostPairingCode)
	rt.human("POST /api/v1/build-hosts/{id}/approve", s.approveBuildHost)
	rt.human("PATCH /api/v1/build-hosts/{id}", s.updateBuildHost)
	rt.human("POST /api/v1/build-hosts/{id}/revoke", s.revokeBuildHost)
}

func (s *server) listBuildHosts(w http.ResponseWriter, r *http.Request, who caller) error {
	hosts, err := s.deps.BuildHosts.List(r.Context(), who.actor)
	if err != nil {
		return err
	}
	out := make([]buildHostDTO, 0, len(hosts))
	now := s.deps.BuildHosts.Now()
	for _, h := range hosts {
		dto := toBuildHost(h)
		dto.Online = h.Online(now)
		out = append(out, dto)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *server) getBuildHost(w http.ResponseWriter, r *http.Request, who caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	h, err := s.deps.BuildHosts.Get(r.Context(), who.actor, id)
	if err != nil {
		return err
	}
	dto := toBuildHost(h)
	dto.Online = h.Online(s.deps.BuildHosts.Now())
	writeJSON(w, http.StatusOK, dto)
	return nil
}

func (s *server) createBuildHostPairingCode(w http.ResponseWriter, r *http.Request, who caller) error {
	if err := decodeBuildHostBody(w, r, &struct{}{}, true); err != nil {
		return err
	}
	code, err := s.deps.BuildHosts.CreatePairingCode(r.Context(), who.actor)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, struct {
		Code      string    `json:"code"`
		ExpiresAt time.Time `json:"expires_at"`
	}{code.Code.Reveal(), code.ExpiresAt})
	return nil
}

func (s *server) approveBuildHost(w http.ResponseWriter, r *http.Request, who caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var body struct {
		Name         *string   `json:"name"`
		Capabilities *[]string `json:"capabilities"`
		MaxParallel  *int      `json:"max_parallel"`
	}
	if err := decodeBuildHostBody(w, r, &body, true); err != nil {
		return err
	}
	h, err := s.deps.BuildHosts.Approve(r.Context(), who.actor, id, domain.BuildHostEdit{Name: body.Name, Capabilities: body.Capabilities, MaxParallel: body.MaxParallel})
	if err != nil {
		return err
	}
	dto := toBuildHost(h)
	dto.Online = h.Online(s.deps.BuildHosts.Now())
	writeJSON(w, http.StatusOK, dto)
	return nil
}

func (s *server) updateBuildHost(w http.ResponseWriter, r *http.Request, who caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var body struct {
		Name         *string   `json:"name"`
		Capabilities *[]string `json:"capabilities"`
		MaxParallel  *int      `json:"max_parallel"`
		Enabled      *bool     `json:"enabled"`
	}
	if err := decodeBuildHostBody(w, r, &body, false); err != nil {
		return err
	}
	h, err := s.deps.BuildHosts.Update(r.Context(), who.actor, id, domain.BuildHostEdit{Name: body.Name, Capabilities: body.Capabilities, MaxParallel: body.MaxParallel, Enabled: body.Enabled})
	if err != nil {
		return err
	}
	dto := toBuildHost(h)
	dto.Online = h.Online(s.deps.BuildHosts.Now())
	writeJSON(w, http.StatusOK, dto)
	return nil
}

func (s *server) revokeBuildHost(w http.ResponseWriter, r *http.Request, who caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if err := decodeBuildHostBody(w, r, &struct{}{}, true); err != nil {
		return err
	}
	h, err := s.deps.BuildHosts.Revoke(r.Context(), who.actor, id)
	if err != nil {
		return err
	}
	dto := toBuildHost(h)
	dto.Online = h.Online(s.deps.BuildHosts.Now())
	writeJSON(w, http.StatusOK, dto)
	return nil
}

// decodeBuildHostBody preserves omitted fields and refuses null, non-objects,
// duplicate keys and trailing values without changing older endpoint contracts.
func decodeBuildHostBody(w http.ResponseWriter, r *http.Request, out any, optional bool) error {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		return domain.Invalidf("request body exceeds the size limit")
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		if optional {
			return nil
		}
		return domain.Invalidf("request body is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return domain.Invalidf("request body must be an object")
	}
	fields := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return domain.Invalidf("invalid request body")
		}
		key, ok := token.(string)
		if !ok || fields[key] {
			return domain.Invalidf("duplicate or invalid request field")
		}
		fields[key] = true
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return domain.Invalidf("invalid request field")
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return domain.Invalidf("%s cannot be null", key)
		}
	}
	if _, err := decoder.Token(); err != nil {
		return domain.Invalidf("invalid request body")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return domain.Invalidf("request body must contain exactly one object")
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return domain.Invalidf("request body is not valid: %v", err)
	}
	return nil
}

func (s *server) buildHostEvents(w http.ResponseWriter, r *http.Request, who caller) error {
	id, e := pathID(r, "id")
	if e != nil {
		return e
	}
	q, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil {
		return domain.Invalidf("invalid pagination")
	}
	after := int64(0)
	limit := 100
	for key, vals := range q {
		if len(vals) != 1 {
			return domain.Invalidf("invalid pagination")
		}
		switch key {
		case "after_id":
			after, e = strconv.ParseInt(vals[0], 10, 64)
		case "limit":
			limit, e = strconv.Atoi(vals[0])
		default:
			return domain.Invalidf("invalid pagination")
		}
		if e != nil {
			return domain.Invalidf("invalid pagination")
		}
	}
	events, e := s.deps.BuildHosts.Events(r.Context(), who.actor, id, after, limit)
	if e != nil {
		return e
	}
	writeJSON(w, 200, events)
	return nil
}
