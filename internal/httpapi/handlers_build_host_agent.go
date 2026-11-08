package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func decodeMachineBody(w http.ResponseWriter, r *http.Request, out any, required ...string) error {
	raw, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 96*1024))
	if e != nil {
		return domain.Invalidf("request body exceeds the size limit")
	}
	if e = domain.DecodeStrictObject(raw, out); e != nil {
		return e
	}
	var obj map[string]json.RawMessage
	json.Unmarshal(raw, &obj)
	for _, k := range required {
		if _, ok := obj[k]; !ok {
			return domain.Invalidf("missing report field")
		}
	}
	return nil
}
func (s *server) pairBuildHost(w http.ResponseWriter, r *http.Request, _ caller) error {
	if e := s.pairLimiter.allow(s.network.clientIP(r)); e != nil {
		return e
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.RawPath != "" {
		return domain.Invalidf("invalid request path")
	}
	var in app.BuildHostPairInput
	if e := decodeMachineBody(w, r, &in, "code", "public_key", "name", "facts"); e != nil {
		return e
	}
	h, e := s.deps.BuildHosts.Pair(r.Context(), in)
	if e != nil {
		return e
	}
	writeJSON(w, 201, struct {
		ID     int64                  `json:"id"`
		Status domain.BuildHostStatus `json:"status"`
	}{h.ID, h.Status})
	return nil
}
func (s *server) buildHostStatus(w http.ResponseWriter, r *http.Request, who caller) error {
	body, e := io.ReadAll(r.Body)
	if e != nil || len(body) != 0 {
		return domain.Invalidf("status body must be empty")
	}
	h, e := s.deps.BuildHosts.HostStatus(r.Context(), who.host)
	if e != nil {
		return e
	}
	writeJSON(w, 200, struct {
		ID           int64                  `json:"id"`
		Status       domain.BuildHostStatus `json:"status"`
		Enabled      bool                   `json:"enabled"`
		Capabilities []string               `json:"capabilities"`
		MaxParallel  int                    `json:"max_parallel"`
		Waiting      bool                   `json:"waiting_for_approval"`
	}{h.ID, h.Status, h.Enabled, h.Capabilities, h.MaxParallel, h.Status == domain.BuildHostPending})
	return nil
}
func (s *server) buildHostHeartbeat(w http.ResponseWriter, r *http.Request, who caller) error {
	var in domain.BuildHostReport
	if e := decodeMachineBody(w, r, &in, "facts", "running_job_ids", "free_disk_bytes"); e != nil {
		return e
	}
	h, e := s.deps.BuildHosts.Heartbeat(r.Context(), who.host, in)
	if e != nil {
		return e
	}
	writeJSON(w, 200, struct {
		ID   int64      `json:"id"`
		Seen *time.Time `json:"last_seen_at"`
	}{h.ID, h.LastSeenAt})
	return nil
}
