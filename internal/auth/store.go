package auth

import (
	"context"
	"time"
)

// Store persists accounts and credentials.
type Store interface {
	// CreateFirstUser inserts user only while no user exists, so the setup
	// wizard cannot be replayed. It returns a conflict otherwise.
	CreateFirstUser(ctx context.Context, user *User) error
	CountUsers(ctx context.Context) (int, error)
	UserByID(ctx context.Context, id int64) (User, error)
	UserByEmail(ctx context.Context, email string) (User, error)
	// IsPersonName reports whether any user has this name, ignoring case.
	IsPersonName(ctx context.Context, name string) (bool, error)
	UserByIdentity(ctx context.Context, issuer, subject string) (User, error)
	LinkIdentity(ctx context.Context, identity Identity, userID int64, now time.Time) error

	SaveRefreshToken(ctx context.Context, token *RefreshToken) error
	RefreshTokenByHash(ctx context.Context, hash string) (RefreshToken, error)
	// RotateRefreshToken revokes old and saves next atomically. It returns a
	// conflict when old was already revoked, which signals token reuse.
	RotateRefreshToken(ctx context.Context, oldID int64, next *RefreshToken, now time.Time) error
	RevokeRefreshToken(ctx context.Context, id int64, now time.Time) error
	RevokeUserRefreshTokens(ctx context.Context, userID int64, now time.Time) error

	CreateAPIToken(ctx context.Context, token *APIToken) error
	APITokenByHash(ctx context.Context, hash string) (APIToken, error)
	ListAPITokens(ctx context.Context) ([]APIToken, error)
	RevokeAPIToken(ctx context.Context, id int64, now time.Time) error
	TouchAPIToken(ctx context.Context, id int64, now time.Time) error
}
