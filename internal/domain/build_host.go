package domain

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"slices"
	"time"
)

// BuildHostOS identifies the operating system reported by a host.
type BuildHostOS string

// BuildHostStatus is a host's approval lifecycle.
type BuildHostStatus string

// Registry lifecycle states.
const (
	BuildHostPending  BuildHostStatus = "pending"
	BuildHostApproved BuildHostStatus = "approved"
	BuildHostRevoked  BuildHostStatus = "revoked"
)

// BuildHost is a registered build identity. Facts are untrusted self-reported metadata.
type BuildHost struct {
	RunningJobIDs []int64
	FreeDiskBytes *int64
	ID            int64
	Name          string
	OS            BuildHostOS
	Arch          string
	Status        BuildHostStatus
	Capabilities  []string
	MaxParallel   int
	Enabled       bool
	PublicKey     string
	Facts         json.RawMessage
	AgentVersion  string
	LastSeenAt    *time.Time
	CreatedAt     time.Time
	ApprovedAt    *time.Time
	ApprovedBy    *int64
	RevokedAt     *time.Time
}

// BuildHostDraft supplies enrollment metadata for a new pending identity.
type BuildHostDraft struct {
	Name         string
	OS           BuildHostOS
	Arch         string
	PublicKey    string
	Capabilities []string
	MaxParallel  *int
	Facts        json.RawMessage
	AgentVersion string
}

// BuildHostEdit contains the administrative fields that may change.
type BuildHostEdit struct {
	Name         *string
	Capabilities *[]string
	MaxParallel  *int
	Enabled      *bool
}

// BuildHostPairingCode stores only a digest and issuance/consumption metadata.
type BuildHostPairingCode struct {
	CodeHash   string `json:"-"`
	CreatedBy  int64
	CreatedAt  time.Time
	ExpiresAt  time.Time
	UsedAt     *time.Time
	UsedByHost *int64
}

var capabilityPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
var buildArchPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// NewBuildHost creates a validated pending identity with conservative defaults.
func NewBuildHost(d BuildHostDraft, now time.Time) (BuildHost, error) {
	h := BuildHost{RunningJobIDs: []int64{}, Name: d.Name, OS: d.OS, Arch: d.Arch, PublicKey: d.PublicKey, Capabilities: slices.Clone(d.Capabilities), MaxParallel: 1, Enabled: true, Status: BuildHostPending, Facts: slices.Clone(d.Facts), AgentVersion: d.AgentVersion, CreatedAt: now.UTC()}
	if d.MaxParallel != nil {
		h.MaxParallel = *d.MaxParallel
	}
	if h.Facts == nil {
		h.Facts = json.RawMessage(`{}`)
	}
	err := h.Validate()
	return h, err
}

// Validate validates and canonicalizes management fields and identity encoding.
func (h *BuildHost) Validate() error {
	if err := ValidateBuildHostTelemetry(h.RunningJobIDs, h.FreeDiskBytes); err != nil {
		return err
	}
	var err error
	if h.Name, err = RequireText("name", h.Name, 200); err != nil {
		return err
	}
	if h.OS != "windows" && h.OS != "linux" && h.OS != "macos" {
		return Invalidf("os must be windows, linux or macos")
	}
	if !buildArchPattern.MatchString(h.Arch) {
		return Invalidf("arch must be a lowercase token of at most 64 characters")
	}
	if h.MaxParallel < 1 || h.MaxParallel > 8 {
		return Invalidf("max_parallel must be between 1 and 8")
	}
	if len(h.Capabilities) > 32 {
		return Invalidf("at most 32 capabilities are allowed")
	}
	for _, c := range h.Capabilities {
		if !capabilityPattern.MatchString(c) {
			return Invalidf("capabilities must be lowercase tokens of at most 32 characters")
		}
	}
	h.Capabilities = slices.Clone(h.Capabilities)
	slices.Sort(h.Capabilities)
	h.Capabilities = slices.Compact(h.Capabilities)
	if h.Capabilities == nil {
		h.Capabilities = []string{}
	}
	key, err := base64.StdEncoding.DecodeString(h.PublicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return Invalidf("public_key must be a base64 Ed25519 public key")
	}
	h.PublicKey = base64.StdEncoding.EncodeToString(key)
	var facts map[string]json.RawMessage
	if len(h.Facts) > 65536 || json.Unmarshal(h.Facts, &facts) != nil || facts == nil {
		return Invalidf("facts must be a JSON object of at most 64 KiB")
	}
	if _, err := LimitText("agent_version", h.AgentVersion, 64); err != nil {
		return err
	}
	if (h.ApprovedAt == nil) != (h.ApprovedBy == nil) {
		return Invalidf("incomplete approval metadata")
	}
	switch h.Status {
	case BuildHostPending:
		if h.ApprovedAt != nil || h.RevokedAt != nil {
			return Invalidf("pending host has lifecycle metadata")
		}
	case BuildHostApproved:
		if h.ApprovedAt == nil || h.RevokedAt != nil {
			return Invalidf("invalid approval metadata")
		}
	case BuildHostRevoked:
		if h.RevokedAt == nil || h.Enabled {
			return Invalidf("revoked host must be disabled with revocation time")
		}
	default:
		return Invalidf("invalid host status")
	}
	return nil
}

// ApplyEdit validates an edit before replacing any fields.
func (h *BuildHost) ApplyEdit(e BuildHostEdit) error {
	next := *h
	if e.Name != nil {
		next.Name = *e.Name
	}
	if e.Capabilities != nil {
		next.Capabilities = *e.Capabilities
	}
	if e.MaxParallel != nil {
		next.MaxParallel = *e.MaxParallel
	}
	if e.Enabled != nil {
		if *e.Enabled && h.Status == BuildHostRevoked {
			return Conflictf("revoked hosts cannot be enabled")
		}
		next.Enabled = *e.Enabled
	}
	if err := next.Validate(); err != nil {
		return err
	}
	*h = next
	return nil
}

// Approve records a pending host's first approval.
func (h *BuildHost) Approve(actorID int64, now time.Time, e BuildHostEdit) error {
	if h.Status != BuildHostPending {
		return Conflictf("only pending hosts can be approved")
	}
	if actorID <= 0 {
		return Invalidf("approving user is required")
	}
	if e.Enabled != nil {
		return Invalidf("approval cannot change enabled")
	}
	next := *h
	if err := next.ApplyEdit(e); err != nil {
		return err
	}
	now = now.UTC()
	next.Status = BuildHostApproved
	next.ApprovedAt = &now
	next.ApprovedBy = &actorID
	*h = next
	return nil
}

// Revoke permanently disables a host, retaining its first revocation time.
func (h *BuildHost) Revoke(now time.Time) {
	if h.Status == BuildHostRevoked {
		return
	}
	now = now.UTC()
	h.Status = BuildHostRevoked
	h.Enabled = false
	h.RevokedAt = &now
}

// Online projects recent approved connectivity without persisting another state.
func (h BuildHost) Online(now time.Time) bool {
	return h.Status == BuildHostApproved && h.LastSeenAt != nil && now.Sub(*h.LastSeenAt) < 3*time.Minute
}
