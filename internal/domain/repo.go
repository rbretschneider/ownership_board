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
	// AllowAutonomous lets a delegate agent file autonomous tickets here,
	// which skip both checkpoints. Off unless a person turns it on.
	AllowAutonomous bool
	CreatedAt       time.Time
}

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
	Color         *string
	Description   *string
	TreatAsPublic *bool
	// RequireSpecApproval and RequireDesignApproval change the checkpoints.
	RequireSpecApproval   *bool
	RequireDesignApproval *bool
	AllowAutonomous       *bool
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
	repo := Repo{Provider: listing.Provider, CreatedAt: now, RequireSpecApproval: true}
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
	if style.TreatAsPublic != nil {
		r.TreatAsPublic = *style.TreatAsPublic
	}
	if style.RequireSpecApproval != nil {
		r.RequireSpecApproval = *style.RequireSpecApproval
	}
	if style.RequireDesignApproval != nil {
		r.RequireDesignApproval = *style.RequireDesignApproval
	}
	if style.AllowAutonomous != nil {
		r.AllowAutonomous = *style.AllowAutonomous
	}
	return nil
}

// ChangesGates reports whether style changes a checkpoint setting, including
// whether autonomous tickets may skip them.
func (style RepoStyle) ChangesGates() bool {
	return style.RequireSpecApproval != nil || style.RequireDesignApproval != nil || style.AllowAutonomous != nil
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
