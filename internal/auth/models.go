package auth

import (
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// User is a person who can sign in.
type User struct {
	ID           int64
	Name         string
	Email        string
	PasswordHash string
	IsAdmin      bool
	TokenVersion int
	CreatedAt    time.Time
	// IsSSOLinked is filled on reads: the user has at least one SSO identity.
	IsSSOLinked bool
}

// Actor returns the user as the domain sees them.
func (u User) Actor() domain.Actor {
	return domain.Actor{Kind: domain.ActorHuman, ID: u.ID, Name: u.Name}
}

// RefreshToken is a stored, hashed refresh token.
type RefreshToken struct {
	ID           int64
	UserID       int64
	Hash         string
	TokenVersion int
	ExpiresAt    time.Time
	RevokedAt    *time.Time
	CreatedAt    time.Time
}

// APIToken is a stored, hashed personal access token for an agent.
type APIToken struct {
	ID         int64
	Name       string
	Prefix     string
	Hash       string
	CreatedBy  int64
	CreatedAt  time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
}

// Actor returns the token as the domain sees it: an agent named after it.
func (t APIToken) Actor() domain.Actor {
	return domain.Actor{Kind: domain.ActorAgent, ID: t.ID, Name: t.Name}
}

// Identity is who an SSO provider says someone is.
type Identity struct {
	Issuer          string
	Subject         string
	Email           string
	IsEmailVerified bool
	Name            string
}

// Session is the pair of tokens a browser holds after signing in.
type Session struct {
	User          User
	AccessToken   string
	AccessExpires time.Time
	RefreshToken  string
	RefreshExpiry time.Time
}
