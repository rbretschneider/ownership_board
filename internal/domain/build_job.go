package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const BuildJobResultLimit = 256 * 1024
const BuildJobBundleLimit int64 = 100 * 1024 * 1024
const BuildJobStorageLimit int64 = 2 * 1024 * 1024 * 1024

var jobIDPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var gitSHAPattern = regexp.MustCompile(`^(?:[a-f0-9]{40}|[a-f0-9]{64})$`)

func ValidBuildJobID(id string) bool { return jobIDPattern.MatchString(id) }

type BuildJobTarget struct {
	OS                   BuildHostOS `json:"os"`
	RequiredCapabilities []string    `json:"required_capabilities"`
}

func (t *BuildJobTarget) Validate() error {
	if t.OS != "windows" && t.OS != "linux" && t.OS != "macos" {
		return Invalidf("invalid target os")
	}
	if len(t.RequiredCapabilities) > 32 {
		return Invalidf("at most 32 capabilities are allowed")
	}
	for _, c := range t.RequiredCapabilities {
		if !capabilityPattern.MatchString(c) {
			return Invalidf("invalid target capability")
		}
	}
	t.RequiredCapabilities = slices.Clone(t.RequiredCapabilities)
	slices.Sort(t.RequiredCapabilities)
	t.RequiredCapabilities = slices.Compact(t.RequiredCapabilities)
	if t.RequiredCapabilities == nil {
		t.RequiredCapabilities = []string{}
	}
	return nil
}
func (t BuildJobTarget) Matches(h BuildHost) bool {
	if t.OS != h.OS {
		return false
	}
	for _, c := range t.RequiredCapabilities {
		if !slices.Contains(h.Capabilities, c) {
			return false
		}
	}
	return true
}
func (t BuildJobTarget) Canonical() string { b, _ := json.Marshal(t); return string(b) }

type BuildJobSubmission struct {
	TicketID     int64          `json:"ticket_id"`
	Repo         string         `json:"repo"`
	SHA          string         `json:"sha"`
	Preset       string         `json:"preset"`
	Target       BuildJobTarget `json:"target"`
	BundleBytes  int64          `json:"bundle_bytes"`
	BundleSHA256 string         `json:"bundle_sha256"`
}

func (s *BuildJobSubmission) Validate() error {
	s.SHA = strings.ToLower(s.SHA)
	if s.TicketID <= 0 || s.Repo == "" || len(s.Repo) > 512 || !gitSHAPattern.MatchString(s.SHA) {
		return Invalidf("invalid ticket, repository or commit")
	}
	if s.Preset != "dotnet" && s.Preset != "cmake" && s.Preset != "script" {
		return Invalidf("invalid build preset")
	}
	if s.BundleBytes > BuildJobBundleLimit {
		return TooLargef("bundle exceeds 100 MiB")
	}
	if s.BundleBytes <= 0 || !ValidBuildJobID(s.BundleSHA256) {
		return Invalidf("invalid bundle size or digest")
	}
	return s.Target.Validate()
}
func (s BuildJobSubmission) ID() string {
	raw, _ := json.Marshal([]any{"build-job-v1", s.Repo, s.SHA, s.Preset, s.Target})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type BuildJob struct {
	ID                   string          `json:"id"`
	TicketID             int64           `json:"ticket_id"`
	Repo                 string          `json:"repo"`
	SHA                  string          `json:"sha"`
	Preset               string          `json:"preset"`
	Target               BuildJobTarget  `json:"target"`
	Status               string          `json:"status"`
	HostID               *int64          `json:"host_id"`
	Attempt              int             `json:"attempt"`
	CreatedAt            time.Time       `json:"created_at"`
	StartedAt            *time.Time      `json:"started_at"`
	FinishedAt           *time.Time      `json:"finished_at"`
	AssignmentReleasedAt *time.Time      `json:"-"`
	Phase                string          `json:"phase"`
	Detail               string          `json:"detail"`
	Result               json.RawMessage `json:"result"`
	BundleBytes          int64           `json:"bundle_bytes"`
	BundleSHA256         string          `json:"bundle_sha256"`
}
type BuildJobProgress struct {
	Attempt int    `json:"attempt"`
	Phase   string `json:"phase"`
	Detail  string `json:"detail"`
}

func (p BuildJobProgress) Validate() error {
	if p.Attempt < 1 || p.Attempt > 3 || len(p.Phase) > 64 || len(p.Detail) > 4096 || !utf8.ValidString(p.Phase) || !utf8.ValidString(p.Detail) {
		return Invalidf("invalid progress")
	}
	return nil
}

type BuildJobCommand struct {
	Label    string `json:"label"`
	ExitCode int    `json:"exit_code"`
	Log      string `json:"log"`
}
type BuildJobResult struct {
	Commands    []BuildJobCommand `json:"commands"`
	FailureKind string            `json:"failure_kind,omitempty"`
}

func (r BuildJobResult) Validate(status string) error {
	if status != "passed" && status != "failed" {
		return Invalidf("invalid result status")
	}
	if len(r.Commands) > 128 || len(r.FailureKind) > 64 || !utf8.ValidString(r.FailureKind) {
		return Invalidf("invalid result")
	}
	failed := r.FailureKind != ""
	for _, c := range r.Commands {
		if len(c.Log) > 16*1024 {
			return TooLargef("command log exceeds 16 KiB")
		}
		if len(c.Label) == 0 || len(c.Label) > 200 || !utf8.ValidString(c.Label) || !utf8.ValidString(c.Log) {
			return Invalidf("invalid command")
		}
		if c.ExitCode != 0 {
			failed = true
		}
	}
	if status == "passed" && (failed || len(r.Commands) == 0) || status == "failed" && !failed {
		return Invalidf("result contradicts status")
	}
	raw, _ := json.Marshal(r)
	if len(raw) > BuildJobResultLimit {
		return TooLargef("result exceeds 256 KiB")
	}
	return nil
}

type BuildJobCompletion struct {
	Attempt int             `json:"attempt"`
	Status  string          `json:"status"`
	Result  json.RawMessage `json:"result"`
}

func (c *BuildJobCompletion) Validate() error {
	if len(c.Result) > BuildJobResultLimit {
		return TooLargef("result exceeds 256 KiB")
	}
	if c.Attempt < 1 || c.Attempt > 3 {
		return Invalidf("invalid attempt")
	}
	var r BuildJobResult
	if err := DecodeStrictObject(c.Result, &r); err != nil {
		return err
	}
	var fields struct {
		Commands []map[string]json.RawMessage `json:"commands"`
	}
	if err := json.Unmarshal(c.Result, &fields); err != nil {
		return Invalidf("invalid result")
	}
	for _, cmd := range fields.Commands {
		for _, key := range []string{"label", "exit_code", "log"} {
			if _, ok := cmd[key]; !ok {
				return Invalidf("missing command field")
			}
		}
	}
	if err := r.Validate(c.Status); err != nil {
		return err
	}
	// Normalize insignificant whitespace for idempotent terminal reports.
	c.Result, _ = json.Marshal(r)
	return nil
}
