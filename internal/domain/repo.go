package domain

import (
	"regexp"
	"strings"
	"time"
)

// Repo is a code repository a ticket can target. The board is repository
// agnostic: repos are synced from hosting providers or added by hand, and a
// ticket may have none until the agent works out where it belongs.
type Repo struct {
	BuildTargets  []RepoBuildTarget
	ID            int64
	Provider      Provider
	FullName      string
	URL           string
	Description   string
	Color         string
	DefaultBranch string
	IsPrivate     bool
	IsArchived    bool
	SyncedAt      *time.Time
	// MissingSince is set while the provider no longer lists the repo.
	MissingSince *time.Time
	// TreatAsPublic is a person's override: handle the repo as public even if
	// the provider says it is private, for example before open-sourcing it.
	TreatAsPublic bool
	// RequireSpecApproval and RequireDesignApproval are the owner's
	// checkpoints for tickets in this repo.
	RequireSpecApproval   bool
	RequireDesignApproval bool
	// AllowAgentMerge is the owner's standing grant for this repo only: an
	// agent may merge a ticket's pull request itself once every check is
	// green and no review asks for changes. Off by default; only a person
	// can turn it on, per repo.
	AllowAgentMerge bool
	// DeployMode is a person-controlled policy preserved across provider refreshes.
	DeployMode DeployMode
	// Merge sequences order ordinary completions independently of ticket IDs.
	MergeSequence                     int64
	HealthCheckedThroughMergeSequence int64
	// HealthCheckEvery is the merge interval; 0 disables scheduling.
	HealthCheckEvery int
	// HealthCheckedThroughTicket and HealthCheckedAt record where the last
	// health check ended. Both are empty until the first check.
	HealthCheckedThroughTicket *int64
	HealthCheckedAt            *time.Time
	CreatedAt                  time.Time
}

// Architecture health check interval limits.
const (
	DefaultHealthCheckEvery = 8
	MaxHealthCheckEvery     = 50
)

// Visibility is how agents must treat a repo's contents and history.
type Visibility string

// The visibilities.
const (
	VisibilityPublic  Visibility = "public"
	VisibilityPrivate Visibility = "private"
)

// privateEvidenceAge is how fresh a provider's "private" must be to count.
const privateEvidenceAge = 7 * 24 * time.Hour

// Visibility fails closed: a repo is private only when its provider said so in
// a sync within the last week, it is still listed, and nobody marked it public.
// Hand-added repos have no provider to ask, so they always count as public.
func (r Repo) Visibility(now time.Time) Visibility {
	isFreshlyPrivate := r.IsPrivate && r.SyncedAt != nil && now.Sub(*r.SyncedAt) <= privateEvidenceAge
	if r.Provider == ProviderManual || r.TreatAsPublic || r.IsMissing() || !isFreshlyPrivate {
		return VisibilityPublic
	}
	return VisibilityPrivate
}

// RepoListing is a repository as a provider reports it during a sync.
type RepoListing struct {
	Provider      Provider
	FullName      string
	URL           string
	Description   string
	DefaultBranch string
	IsPrivate     bool
	IsArchived    bool
}

// ManualRepoDraft is the input for a repo added by hand.
type ManualRepoDraft struct {
	FullName    string
	URL         string
	Description string
	Color       string
}

// RepoStyle is what a person may change on any repo, synced or not.
type RepoStyle struct {
	BuildTargets  *[]RepoBuildTarget
	DeployMode    *DeployMode
	Color         *string
	Description   *string
	TreatAsPublic *bool
	// RequireSpecApproval and RequireDesignApproval change the checkpoints.
	RequireSpecApproval   *bool
	RequireDesignApproval *bool
	// AllowAgentMerge changes the agent-merge grant.
	AllowAgentMerge *bool
	// HealthCheckEvery changes the architecture check interval (0 = off).
	HealthCheckEvery *int
}

// Repo field limits.
const (
	MaxRepoNameLength        = 200
	MaxRepoDescriptionLength = 2_000
)

var (
	repoNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)+$`)
	colorPattern    = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

// NewManualRepo validates a hand-entered repo.
func NewManualRepo(draft ManualRepoDraft, now time.Time) (Repo, error) {
	listing := RepoListing{Provider: ProviderManual, FullName: draft.FullName, URL: draft.URL, Description: draft.Description}
	repo, err := NewSyncedRepo(listing, now)
	if err != nil {
		return Repo{}, err
	}
	repo.SyncedAt = nil
	if repo.Color, err = parseColor(draft.Color); err != nil {
		return Repo{}, err
	}
	return repo, nil
}

// NewSyncedRepo validates a provider listing as a new repo.
func NewSyncedRepo(listing RepoListing, now time.Time) (Repo, error) {
	repo := Repo{BuildTargets: []RepoBuildTarget{}, DeployMode: DeployModeOff, Provider: listing.Provider, CreatedAt: now, RequireSpecApproval: true, HealthCheckEvery: DefaultHealthCheckEvery}
	if err := repo.Refresh(listing, now); err != nil {
		return Repo{}, err
	}
	return repo, nil
}

// Name is the last path segment of the full name, such as "api" in "acme/api".
func (r Repo) Name() string {
	return r.FullName[strings.LastIndex(r.FullName, "/")+1:]
}

// Refresh overwrites provider-owned fields from a newer listing. The color a
// person picked survives.
func (r *Repo) Refresh(listing RepoListing, now time.Time) error {
	fullName, err := parseRepoName(listing.FullName)
	if err != nil {
		return err
	}
	repoURL, err := parseOptionalURL("url", listing.URL)
	if err != nil {
		return err
	}
	description, err := LimitText("description", StripHiddenText(strings.TrimSpace(listing.Description)), MaxRepoDescriptionLength)
	if err != nil {
		return err
	}
	r.FullName, r.URL, r.Description = fullName, repoURL, description
	r.DefaultBranch, r.IsPrivate, r.IsArchived = strings.TrimSpace(listing.DefaultBranch), listing.IsPrivate, listing.IsArchived
	r.SyncedAt = &now
	r.MissingSince = nil
	return nil
}

// MarkMissing records that the provider stopped listing the repo. The first
// sighting of its absence is kept.
func (r *Repo) MarkMissing(now time.Time) {
	if r.MissingSince == nil {
		r.MissingSince = &now
	}
}

// IsMissing reports whether the provider no longer lists the repo.
func (r Repo) IsMissing() bool {
	return r.MissingSince != nil
}

// Restyle applies a person's display changes.
func (r *Repo) Restyle(style RepoStyle) error {
	if style.BuildTargets != nil {
		targets, err := NormalizeRepoBuildTargets(*style.BuildTargets)
		if err != nil {
			return err
		}
		r.BuildTargets = targets
	}
	if style.DeployMode != nil {
		mode, err := ParseDeployMode(string(*style.DeployMode))
		if err != nil {
			return err
		}
		r.DeployMode = mode
	}

	if style.Color != nil {
		color, err := parseColor(*style.Color)
		if err != nil {
			return err
		}
		r.Color = color
	}
	if style.Description != nil {
		description, err := LimitText("description", strings.TrimSpace(*style.Description), MaxRepoDescriptionLength)
		if err != nil {
			return err
		}
		r.Description = description
	}
	if style.HealthCheckEvery != nil {
		if *style.HealthCheckEvery < 0 || *style.HealthCheckEvery > MaxHealthCheckEvery {
			return Invalidf("health_check_every must be between 0 and %d (0 turns the check off)", MaxHealthCheckEvery)
		}
		r.HealthCheckEvery = *style.HealthCheckEvery
	}
	if style.TreatAsPublic != nil {
		r.TreatAsPublic = *style.TreatAsPublic
	}
	if style.RequireSpecApproval != nil {
		r.RequireSpecApproval = *style.RequireSpecApproval
	}
	if style.RequireDesignApproval != nil {
		r.RequireDesignApproval = *style.RequireDesignApproval
	}
	if style.AllowAgentMerge != nil {
		r.AllowAgentMerge = *style.AllowAgentMerge
	}
	return nil
}

// ChangesGates reports whether style changes a checkpoint or deployment setting.
func (style RepoStyle) ChangesGates() bool {
	return style.DeployMode != nil || style.RequireSpecApproval != nil || style.RequireDesignApproval != nil || style.AllowAgentMerge != nil
}

// ChangesHealthCheck reports whether style changes the health check interval.
func (style RepoStyle) ChangesHealthCheck() bool {
	return style.HealthCheckEvery != nil
}

func parseRepoName(raw string) (string, error) {
	name := strings.Trim(strings.TrimSpace(raw), "/")
	if len(name) > MaxRepoNameLength || !repoNamePattern.MatchString(name) || hasDotSegment(name) {
		return "", Invalidf("repo name must look like owner/name, at most %d characters", MaxRepoNameLength)
	}
	return name, nil
}

// hasDotSegment rejects "." and ".." segments, since agents may turn a repo
// name into a path.
func hasDotSegment(name string) bool {
	for _, segment := range strings.Split(name, "/") {
		if strings.Trim(segment, ".") == "" {
			return true
		}
	}
	return false
}

func parseColor(raw string) (string, error) {
	color := strings.TrimSpace(raw)
	if color == "" {
		return "", nil
	}
	if !colorPattern.MatchString(color) {
		return "", Invalidf("color must look like #7c8cff")
	}
	return strings.ToLower(color), nil
}

// DeployMode controls whether merged work waits for deployment.
type DeployMode string

const (
	DeployModeOff   DeployMode = "off"
	DeployModeAgent DeployMode = "agent"
)

// ParseDeployMode validates the supported repository deployment policies.
func ParseDeployMode(raw string) (DeployMode, error) {
	switch DeployMode(raw) {
	case DeployModeOff, DeployModeAgent:
		return DeployMode(raw), nil
	default:
		return "", Invalidf("deploy_mode must be off or agent")
	}
}
