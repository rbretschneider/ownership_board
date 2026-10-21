// Package connections links the board to repo hosting accounts: the GitHub
// device flow, pasted Forgejo and Bitbucket tokens, encrypted storage, and the
// catalog of sources the repo sync uses.
package connections

import (
	"context"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// Status is the state of a stored connection.
type Status string

// The statuses.
const (
	StatusDisconnected   Status = "disconnected"
	StatusConnected      Status = "connected"
	StatusNeedsReconnect Status = "needs_reconnect"
)

// Providers lists the providers a connection can be made to, in display order.
var Providers = []domain.Provider{domain.ProviderGitHub, domain.ProviderForgejo, domain.ProviderBitbucket}

// SealedRecord is a stored connection with its tokens still encrypted.
type SealedRecord struct {
	Provider         domain.Provider
	BaseURL          string
	Account          string
	AccountID        string
	Email            string
	AccessTokenEnc   string
	RefreshTokenEnc  string
	AccessExpiresAt  *time.Time
	RefreshExpiresAt *time.Time
	Status           Status
	Generation       int64
	ConnectedBy      int64
	ConnectedAt      time.Time
	UpdatedAt        time.Time
}

// Store persists connections. Put is a compare-and-swap: it writes only if the
// stored generation equals expectedGeneration (zero meaning "no row yet") and
// reports whether it did.
type Store interface {
	Get(ctx context.Context, provider domain.Provider) (SealedRecord, bool, error)
	Put(ctx context.Context, record SealedRecord, expectedGeneration int64) (bool, error)
	Delete(ctx context.Context, provider domain.Provider) error
}

// Admin is the person performing a connection change.
type Admin struct {
	ID      int64
	Name    string
	IsAdmin bool
}

func (a Admin) require() error {
	if a.ID <= 0 || !a.IsAdmin {
		return domain.Forbiddenf("only the board's admin can manage connections")
	}
	return nil
}
