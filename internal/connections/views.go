package connections

import (
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

// ManagedBy says where a provider's credentials come from.
type ManagedBy string

// The sources of credentials.
const (
	ManagedBySettings  ManagedBy = "settings"
	ManagedByServerEnv ManagedBy = "server_env"
)

// PollConfirm is the poll status asking the admin to confirm the account.
const PollConfirm = "confirm"

var labels = map[domain.Provider]string{
	domain.ProviderGitHub:    "GitHub",
	domain.ProviderForgejo:   "Forgejo / Gitea",
	domain.ProviderBitbucket: "Bitbucket",
}

// View describes one provider for Settings. It never carries a token.
type View struct {
	Provider    domain.Provider
	Label       string
	IsAvailable bool
	Status      Status
	ManagedBy   ManagedBy
	Account     string
	BaseURL     string
	Email       string
	ConnectedAt *time.Time
	RepoCount   int
	LastSync    *app.SyncOutcome
}

// FlowView is a started device sign-in, without the device code.
type FlowView struct {
	ID        string
	UserCode  string
	ExpiresAt time.Time
	Interval  time.Duration
}

// PollView is one poll's answer.
type PollView struct {
	Status   string
	Interval time.Duration
	Login    string
	Replaces string
	Message  string
}
