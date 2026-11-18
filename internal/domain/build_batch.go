package domain

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// Batched verification limits. A repo opts in with a batch size between the two.
const (
	MinBatchSize     = 2
	MaxBatchSize     = 8
	DefaultBatchSize = 4
)

// BatchAgentVersion is the first build-host agent that merges batch members.
// An older agent ignores a job's batch field and would verify the lead commit
// alone, so the board never hands it a batch.
const BatchAgentVersion = "1.1"

// Batch statuses. queued is a bisection half waiting for a host; dissolved is a
// batch whose members went back to the queue without a verdict.
const (
	BatchQueued    = "queued"
	BatchRunning   = "running"
	BatchPassed    = "passed"
	BatchFailed    = "failed"
	BatchDissolved = "dissolved"
	BatchCancelled = "cancelled"
)

// Member outcomes: what a batch did for one ticket job it held.
const (
	MemberPending    = "pending"
	MemberPassed     = "passed"
	MemberBisected   = "bisected"
	MemberConflict   = "conflict"
	MemberSuperseded = "superseded"
	MemberReturned   = "returned"
)

// BuildBatch is one verification run of several tickets' commits merged together,
// in member order, on one build host. Each ticket keeps its own BuildJob, which
// the batch holds while it runs and settles when it ends.
type BuildBatch struct {
	ID                   string
	Repo                 string
	Preset               string
	Target               BuildJobTarget
	Status               string
	ParentID             string
	HostID               *int64
	Attempt              int
	CreatedAt            time.Time
	StartedAt            *time.Time
	FinishedAt           *time.Time
	AssignmentReleasedAt *time.Time
	Phase                string
	Detail               string
	Result               json.RawMessage
	Members              []BuildBatchMember
}

// BuildBatchMember is a ticket job in a batch, with the job's commit and bundle.
type BuildBatchMember struct {
	JobID        string
	TicketID     int64
	SHA          string
	Position     int
	Outcome      string
	BundleBytes  int64
	BundleSHA256 string
}

// BuildJobBatch is the batch part of a host assignment: the members to merge, in
// order. The first member is the lead: the assignment's sha and bundle are its.
type BuildJobBatch struct {
	Members []BuildJobBatchMember `json:"members"`
}

// BuildJobBatchMember is one commit a build host merges into a batch candidate.
type BuildJobBatchMember struct {
	ID           string `json:"id"`
	TicketID     int64  `json:"ticket_id"`
	SHA          string `json:"sha"`
	BundleBytes  int64  `json:"bundle_bytes"`
	BundleSHA256 string `json:"bundle_sha256"`
}

// Lead is the first member: the commit a host checks out before merging the rest.
func (b BuildBatch) Lead() BuildBatchMember {
	if len(b.Members) == 0 {
		return BuildBatchMember{}
	}
	return b.Members[0]
}

// Assignment is the batch as a host claims it: job-shaped, so the host's run,
// progress and completion protocol is the one it already speaks.
func (b BuildBatch) Assignment() BuildJob {
	lead := b.Lead()
	members := make([]BuildJobBatchMember, 0, len(b.Members))
	for _, m := range b.Members {
		members = append(members, BuildJobBatchMember{ID: m.JobID, TicketID: m.TicketID, SHA: m.SHA, BundleBytes: m.BundleBytes, BundleSHA256: m.BundleSHA256})
	}
	status := b.Status
	return BuildJob{
		ID: b.ID, TicketID: lead.TicketID, Repo: b.Repo, SHA: lead.SHA, Preset: b.Preset, Target: b.Target,
		Status: status, HostID: b.HostID, Attempt: b.Attempt, CreatedAt: b.CreatedAt, StartedAt: b.StartedAt,
		FinishedAt: b.FinishedAt, AssignmentReleasedAt: b.AssignmentReleasedAt, Phase: b.Phase, Detail: b.Detail,
		Result: b.Result, BundleBytes: lead.BundleBytes, BundleSHA256: lead.BundleSHA256,
		Batch: &BuildJobBatch{Members: members},
	}
}

// TicketList names the member tickets, for example "#198, #199".
func (b BuildBatch) TicketList(except int64) string {
	var parts []string
	for _, m := range b.Members {
		if m.TicketID != except {
			parts = append(parts, "#"+strconv.FormatInt(m.TicketID, 10))
		}
	}
	return strings.Join(parts, ", ")
}

// SupportsBatches reports whether a host's agent merges batch members.
func SupportsBatches(agentVersion string) bool {
	return versionAtLeast(agentVersion, BatchAgentVersion)
}

func versionAtLeast(have, want string) bool {
	h, w := strings.Split(strings.TrimSpace(have), "."), strings.Split(want, ".")
	for i := range w {
		need, _ := strconv.Atoi(w[i])
		if i >= len(h) {
			return need == 0
		}
		got, err := strconv.Atoi(h[i])
		if err != nil {
			return false
		}
		if got != need {
			return got > need
		}
	}
	return true
}

// ValidBatchSize reports whether n is an allowed per-repo batch size.
func ValidBatchSize(n int) bool { return n >= MinBatchSize && n <= MaxBatchSize }
