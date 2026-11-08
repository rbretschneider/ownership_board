package domain

import (
	"encoding/json"
	"strconv"
	"time"
)

type BuildHostReport struct {
	Facts         json.RawMessage  `json:"facts"`
	RunningJobIDs []BuildHostJobID `json:"running_job_ids"`
	FreeDiskBytes int64            `json:"free_disk_bytes"`
}

func (r BuildHostReport) Validate() error {
	if _, e := ParseBuildHostFacts(r.Facts); e != nil {
		return e
	}
	return ValidateBuildHostTelemetry(r.RunningJobIDs, &r.FreeDiskBytes)
}
func ValidateBuildHostTelemetry(ids []BuildHostJobID, free *int64) error {
	if len(ids) > 128 || free != nil && *free < 0 {
		return Invalidf("invalid host telemetry")
	}
	seen := map[BuildHostJobID]bool{}
	for _, id := range ids {
		if !id.valid() || seen[id] {
			return Invalidf("invalid host telemetry")
		}
		seen[id] = true
	}
	return nil
}

type BuildHostEvent struct {
	ID        int64           `json:"id"`
	HostID    int64           `json:"host_id"`
	Type      string          `json:"event_type"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
}

// BuildHostJobID preserves legacy numeric telemetry and canonical queue hashes.
type BuildHostJobID string

func (id BuildHostJobID) valid() bool {
	if ValidBuildJobID(string(id)) {
		return true
	}
	n, e := strconv.ParseInt(string(id), 10, 64)
	return e == nil && n > 0 && strconv.FormatInt(n, 10) == string(id)
}
func (id BuildHostJobID) MarshalJSON() ([]byte, error) {
	if !id.valid() {
		return nil, Invalidf("invalid job id")
	}
	if ValidBuildJobID(string(id)) {
		return json.Marshal(string(id))
	}
	return []byte(id), nil
}
func (id *BuildHostJobID) UnmarshalJSON(raw []byte) error {
	var s string
	if len(raw) > 0 && raw[0] == '"' {
		if json.Unmarshal(raw, &s) != nil || !ValidBuildJobID(s) {
			return Invalidf("invalid job hash")
		}
	} else {
		s = string(raw)
		if !BuildHostJobID(s).valid() || ValidBuildJobID(s) {
			return Invalidf("invalid legacy job id")
		}
	}
	*id = BuildHostJobID(s)
	return nil
}
