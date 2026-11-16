package domain

import (
	"encoding/json"
	"slices"
	"strings"
	"time"
)

// VerdictStatus is what the board knows about one commit on one build target.
type VerdictStatus string

const (
	// VerdictPassed: a build job ran the commit and it passed.
	VerdictPassed VerdictStatus = "passed"
	// VerdictFailed: a build job ran the commit and the commit failed. A job
	// that failed for the host's sake (infrastructure, cancelled) is no verdict.
	VerdictFailed VerdictStatus = "failed"
	// VerdictCleanMerge: the commit is a conflict-free merge of the default
	// branch into a passed commit (MergeOf), reported by the agent holding the
	// ticket. It counts as passed without running the host again.
	VerdictCleanMerge VerdictStatus = "clean_merge"
)

// VerdictSource says who produced a verdict.
type VerdictSource string

const (
	VerdictFromBuildJob   VerdictSource = "build_job"
	VerdictFromCleanMerge VerdictSource = "clean_merge"
)

// Verdict is the board's single record of whether a commit passed on a build
// target. It is keyed by (Repo, Target, SHA); the worker, the merge sweep and
// the manager all read it here instead of keeping their own copies
// (engine_lessons.md, "A pass recorded in one place and checked in another").
type Verdict struct {
	Repo   string        `json:"repo"`
	Target string        `json:"target"`
	SHA    string        `json:"sha"`
	Status VerdictStatus `json:"status"`
	// MergeOf is the passed commit a clean merge was made from; empty otherwise.
	MergeOf string `json:"merge_of,omitempty"`
	// JobID is the build job whose run the verdict rests on. For a clean merge
	// it is the job that passed MergeOf (or that commit's own base).
	JobID      string        `json:"job_id"`
	TicketID   int64         `json:"ticket_id"`
	Source     VerdictSource `json:"source"`
	ReportedBy string        `json:"reported_by"`
	RecordedAt time.Time     `json:"recorded_at"`
}

// IsPassed reports whether the commit counts as passed on its target.
func (v Verdict) IsPassed() bool { return v.Status == VerdictPassed || v.Status == VerdictCleanMerge }

// Key names a build target for verdicts: the OS, then any required
// capabilities after a colon, comma-separated and sorted: "windows",
// "windows:cuda,msvc".
func (t BuildJobTarget) Key() string {
	if len(t.RequiredCapabilities) == 0 {
		return string(t.OS)
	}
	caps := slices.Clone(t.RequiredCapabilities)
	slices.Sort(caps)
	caps = slices.Compact(caps)
	return string(t.OS) + ":" + strings.Join(caps, ",")
}

// NormalizeVerdictTarget checks a target key and returns its canonical form.
func NormalizeVerdictTarget(key string) (string, error) {
	osName, caps, hasCaps := strings.Cut(strings.TrimSpace(key), ":")
	target := BuildJobTarget{OS: BuildHostOS(osName)}
	if hasCaps {
		if caps == "" {
			return "", Invalidf("invalid target %q: give the OS, then optional capabilities after a colon", key)
		}
		target.RequiredCapabilities = strings.Split(caps, ",")
	}
	if err := target.Validate(); err != nil {
		return "", Invalidf("invalid target %q: give the OS, then optional capabilities after a colon", key)
	}
	return target.Key(), nil
}

// NormalizeCommitSHA lowercases a full commit hash and checks it.
func NormalizeCommitSHA(field, sha string) (string, error) {
	sha = strings.ToLower(strings.TrimSpace(sha))
	if !gitSHAPattern.MatchString(sha) {
		return "", Invalidf("%s must be a full 40- or 64-character commit hash", field)
	}
	return sha, nil
}

// VerdictForJob is the verdict a finished build job gives its commit, if any.
// A failure the host blames on itself (infrastructure, cancelled) says nothing
// about the commit, so it gives none.
func VerdictForJob(j BuildJob, reportedBy string, at time.Time) (Verdict, bool) {
	var status VerdictStatus
	switch j.Status {
	case "passed":
		status = VerdictPassed
	case "failed":
		if JobResultBlamesHost(j.Result) {
			return Verdict{}, false
		}
		status = VerdictFailed
	default:
		return Verdict{}, false
	}
	return Verdict{
		Repo: j.Repo, Target: j.Target.Key(), SHA: j.SHA, Status: status, JobID: j.ID, TicketID: j.TicketID,
		Source: VerdictFromBuildJob, ReportedBy: reportedBy, RecordedAt: at,
	}, true
}

// JobResultBlamesHost reports whether a failed job's result blames the host,
// not the commit: an infrastructure failure or a stopped run.
func JobResultBlamesHost(result []byte) bool {
	kind, ok := failureKind(result)
	return ok && (kind == "infrastructure" || kind == "cancelled")
}

// CleanMergeReport is an agent's word that SHA is a conflict-free merge of the
// default branch into MergeOf. Target empty means every target MergeOf passed.
type CleanMergeReport struct {
	TicketID int64  `json:"ticket_id"`
	Repo     string `json:"repo"`
	Target   string `json:"target"`
	SHA      string `json:"sha"`
	MergeOf  string `json:"merge_of"`
}

// Validate normalizes the report and checks its fields.
func (r *CleanMergeReport) Validate() error {
	if r.TicketID <= 0 {
		return Invalidf("ticket_id is required")
	}
	r.Repo = strings.TrimSpace(r.Repo)
	if r.Repo == "" || len(r.Repo) > 512 {
		return Invalidf("repo is required")
	}
	var err error
	if r.SHA, err = NormalizeCommitSHA("sha", r.SHA); err != nil {
		return err
	}
	if r.MergeOf, err = NormalizeCommitSHA("merge_of", r.MergeOf); err != nil {
		return err
	}
	if r.SHA == r.MergeOf {
		return Invalidf("sha and merge_of are the same commit")
	}
	if strings.TrimSpace(r.Target) != "" {
		if r.Target, err = NormalizeVerdictTarget(r.Target); err != nil {
			return err
		}
	} else {
		r.Target = ""
	}
	return nil
}

func failureKind(result []byte) (string, bool) {
	var r struct {
		FailureKind string `json:"failure_kind"`
	}
	if json.Unmarshal(result, &r) != nil {
		return "", false
	}
	return r.FailureKind, true
}
