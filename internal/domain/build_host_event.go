package domain

import (
	"encoding/json"
	"time"
)

type BuildHostReport struct {
	Facts         json.RawMessage `json:"facts"`
	RunningJobIDs []int64         `json:"running_job_ids"`
	FreeDiskBytes int64           `json:"free_disk_bytes"`
}

func (r BuildHostReport) Validate() error {
	if _, e := ParseBuildHostFacts(r.Facts); e != nil {
		return e
	}
	return ValidateBuildHostTelemetry(r.RunningJobIDs, &r.FreeDiskBytes)
}
func ValidateBuildHostTelemetry(ids []int64, free *int64) error {
	if len(ids) > 128 || free != nil && *free < 0 {
		return Invalidf("invalid host telemetry")
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if id <= 0 || seen[id] {
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
