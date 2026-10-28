package auth

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rbretschneider/ownership_board/internal/clock"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

// Session lifetimes.
const (
	AccessTokenLifetime  = 15 * time.Minute
	RefreshTokenLifetime = 30 * 24 * time.Hour
	// APITokenPrefix marks a secret as this board's token, so secret scanners
	// and people can recognise a leaked one.
	APITokenPrefix = "ob_pat_"
	// apiTokenDisplayLength is how much of a token is kept to identify it.
	apiTokenDisplayLength = len(APITokenPrefix) + 6
	maxNameLength         = 80
	maxEmailLength        = 254
)

// Service signs people and agents in. It owns sessions entirely; an SSO
// provider only ever answers "who is this?".
type Service struct {
	store    Store
	signer   signer
	clock    clock.Clock
	throttle *Throttle
}

// SetupInput creates the first account.
type SetupInput struct {
	Name     string
	Email    string
	Password string
}

// NewService wires the service. secret signs access tokens and must be at
// least MinSecretLength characters.
func NewService(store Store, secret string, clk clock.Clock, throttle *Throttle) (*Service, error) {
	tokenSigner, err := newSigner(secret, clk)
	if err != nil {
		return nil, err
	}
	return &Service{store: store, signer: tokenSigner, clock: clk, throttle: throttle}, nil
}

// IsSetupRequired reports whether no account exists yet.
func (s *Service) IsSetupRequired(ctx context.Context) (bool, error) {
	count, err := s.store.CountUsers(ctx)
	return count == 0, err
}

// Setup creates the first, admin account and signs it in. It fails once any
// account exists, so there are never default credentials to find.
func (s *Service) Setup(ctx context.Context, input SetupInput) (Session, error) {
	user, err := newAdmin(input, s.clock.Now())
	if err != nil {
		return Session{}, err
	}
	if err := s.store.CreateFirstUser(ctx, &user); err != nil {
		return Session{}, err
	}
	return s.startSession(ctx, user)
}

// Login checks a password and starts a session. Failures are throttled per
// client address.
func (s *Service) Login(ctx context.Context, clientKey, email, password string) (Session, error) {
	if wait := s.throttle.Check(clientKey); wait > 0 {
		return Session{}, &ThrottledError{RetryAfter: wait}
	}
	user, err := s.checkPassword(ctx, email, password)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			s.throttle.Fail(clientKey)
		}
		return Session{}, err
	}
	s.throttle.Succeed(clientKey)
	return s.startSession(ctx, user)
}

// Refresh exchanges a refresh token for a new session, rotating the token.
// Presenting an already-rotated token means it was stolen or replayed, so every
// session of that user is revoked.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (Session, error) {
	stored, user, err := s.validRefreshToken(ctx, refreshToken)
	if err != nil {
		return Session{}, err
	}
	session, next, err := s.newSession(user)
	if err != nil {
		return Session{}, err
	}
	err = s.store.RotateRefreshToken(ctx, stored.ID, &next, s.clock.Now())
	if domain.KindOf(err) == domain.KindConflict {
		return Session{}, s.revokeAfterReuse(ctx, user.ID)
	}
	return session, err
}

// Logout revokes a refresh token. An unknown token is not an error: the
// browser's cookies are cleared either way.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	stored, err := s.store.RefreshTokenByHash(ctx, hashSecret(refreshToken))
	if domain.KindOf(err) == domain.KindNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	return s.store.RevokeRefreshToken(ctx, stored.ID, s.clock.Now())
}

// UserFromAccessToken returns the user an access token belongs to.
func (s *Service) UserFromAccessToken(ctx context.Context, token string) (User, error) {
	claims, err := s.signer.verify(audienceAccess, token)
	if err != nil {
		return User{}, err
	}
	userID, err := strconv.ParseInt(claimString(claims, "sub"), 10, 64)
	if err != nil {
		return User{}, ErrUnauthorized
	}
	user, err := s.store.UserByID(ctx, userID)
	if err != nil || int64(user.TokenVersion) != claimInt(claims, "ver") {
		return User{}, ErrUnauthorized
	}
	return user, nil
}

// TokenFromSecret returns the live API token matching secret.
func (s *Service) TokenFromSecret(ctx context.Context, secret string) (APIToken, error) {
	if !strings.HasPrefix(secret, APITokenPrefix) {
		return APIToken{}, ErrUnauthorized
	}
	token, err := s.store.APITokenByHash(ctx, hashSecret(secret))
	if err != nil || token.RevokedAt != nil {
		return APIToken{}, ErrUnauthorized
	}
	if err := s.store.TouchAPIToken(ctx, token.ID, s.clock.Now()); err != nil {
		return APIToken{}, err
	}
	return token, nil
}

// CreateAPIToken issues a token for an agent and returns its secret, which is
// shown once and never stored. A delegate token may also file autonomous
// tickets; the flag is set here, by a person, and never changes afterwards.
func (s *Service) CreateAPIToken(ctx context.Context, creator User, name string, isDelegate bool) (APIToken, string, error) {
	tokenName, err := domain.RequireText("name", name, maxNameLength)
	if err != nil {
		return APIToken{}, "", err
	}
	if err := domain.RejectHiddenText("name", tokenName); err != nil {
		return APIToken{}, "", err
	}
	// An agent named like a person would show as that person in decision
	// logs and briefs.
	isPerson, err := s.store.IsPersonName(ctx, tokenName)
	if err != nil {
		return APIToken{}, "", err
	}
	if isPerson {
		return APIToken{}, "", domain.Conflictf("an agent token cannot share a person's name; pick a name like claude-loop")
	}
	random, err := newSecret()
	if err != nil {
		return APIToken{}, "", err
	}
	secret := APITokenPrefix + random
	token := APIToken{
		Name: tokenName, Prefix: secret[:apiTokenDisplayLength], Hash: hashSecret(secret),
		CreatedBy: creator.ID, CreatedAt: s.clock.Now(), IsDelegate: isDelegate,
	}
	if err := s.store.CreateAPIToken(ctx, &token); err != nil {
		return APIToken{}, "", err
	}
	return token, secret, nil
}

// ListAPITokens returns tokens that have not been revoked.
func (s *Service) ListAPITokens(ctx context.Context) ([]APIToken, error) {
	return s.store.ListAPITokens(ctx)
}

// RevokeAPIToken stops a token working immediately.
func (s *Service) RevokeAPIToken(ctx context.Context, id int64) error {
	return s.store.RevokeAPIToken(ctx, id, s.clock.Now())
}

// SignInWithIdentity starts a session for the account linked to an SSO
// identity. Accounts are linked deliberately from Settings, never matched by
// email, so a reused or spoofed email cannot take over an account.
func (s *Service) SignInWithIdentity(ctx context.Context, identity Identity) (Session, error) {
	user, err := s.store.UserByIdentity(ctx, identity.Issuer, identity.Subject)
	if domain.KindOf(err) == domain.KindNotFound {
		return Session{}, ErrNotLinked
	}
	if err != nil {
		return Session{}, err
	}
	return s.startSession(ctx, user)
}

// LinkIdentity attaches an SSO identity to a signed-in user.
func (s *Service) LinkIdentity(ctx context.Context, userID int64, identity Identity) error {
	return s.store.LinkIdentity(ctx, identity, userID, s.clock.Now())
}

// UserByID returns a user, for reading back the signed-in account.
func (s *Service) UserByID(ctx context.Context, id int64) (User, error) {
	return s.store.UserByID(ctx, id)
}

func (s *Service) checkPassword(ctx context.Context, email, password string) (User, error) {
	if utf8.RuneCountInString(password) > MaxPasswordLength {
		return User{}, ErrUnauthorized
	}
	user, err := s.store.UserByEmail(ctx, strings.TrimSpace(email))
	if domain.KindOf(err) == domain.KindNotFound || (err == nil && user.PasswordHash == "") {
		return User{}, ErrUnauthorized
	}
	if err != nil {
		return User{}, err
	}
	isMatch, err := VerifyPassword(password, user.PasswordHash)
	if err != nil {
		return User{}, err
	}
	if !isMatch {
		return User{}, ErrUnauthorized
	}
	return user, nil
}

func (s *Service) validRefreshToken(ctx context.Context, refreshToken string) (RefreshToken, User, error) {
	if refreshToken == "" {
		return RefreshToken{}, User{}, ErrUnauthorized
	}
	stored, err := s.store.RefreshTokenByHash(ctx, hashSecret(refreshToken))
	if domain.KindOf(err) == domain.KindNotFound {
		return RefreshToken{}, User{}, ErrUnauthorized
	}
	if err != nil {
		return RefreshToken{}, User{}, err
	}
	user, err := s.store.UserByID(ctx, stored.UserID)
	if err != nil {
		return RefreshToken{}, User{}, ErrUnauthorized
	}
	if stored.RevokedAt != nil {
		return RefreshToken{}, User{}, s.revokeAfterReuse(ctx, user.ID)
	}
	if !s.clock.Now().Before(stored.ExpiresAt) || stored.TokenVersion != user.TokenVersion {
		return RefreshToken{}, User{}, ErrUnauthorized
	}
	return stored, user, nil
}

func (s *Service) revokeAfterReuse(ctx context.Context, userID int64) error {
	if err := s.store.RevokeUserRefreshTokens(ctx, userID, s.clock.Now()); err != nil {
		return fmt.Errorf("auth: revoke sessions after refresh token reuse: %w", err)
	}
	return ErrUnauthorized
}

func (s *Service) startSession(ctx context.Context, user User) (Session, error) {
	session, refresh, err := s.newSession(user)
	if err != nil {
		return Session{}, err
	}
	if err := s.store.SaveRefreshToken(ctx, &refresh); err != nil {
		return Session{}, err
	}
	return session, nil
}

// newSession mints a token pair; the caller stores the refresh token.
func (s *Service) newSession(user User) (Session, RefreshToken, error) {
	claims := map[string]any{"sub": strconv.FormatInt(user.ID, 10), "ver": user.TokenVersion}
	access, accessExpires, err := s.signer.sign(audienceAccess, AccessTokenLifetime, claims)
	if err != nil {
		return Session{}, RefreshToken{}, err
	}
	secret, err := newSecret()
	if err != nil {
		return Session{}, RefreshToken{}, err
	}
	now := s.clock.Now()
	refresh := RefreshToken{
		UserID: user.ID, Hash: hashSecret(secret), TokenVersion: user.TokenVersion,
		ExpiresAt: now.Add(RefreshTokenLifetime), CreatedAt: now,
	}
	session := Session{User: user, AccessToken: access, AccessExpires: accessExpires, RefreshToken: secret, RefreshExpiry: refresh.ExpiresAt}
	return session, refresh, nil
}

func newAdmin(input SetupInput, now time.Time) (User, error) {
	name, err := domain.RequireText("name", input.Name, maxNameLength)
	if err != nil {
		return User{}, err
	}
	email, err := parseEmail(input.Email)
	if err != nil {
		return User{}, err
	}
	length := utf8.RuneCountInString(input.Password)
	if length < MinPasswordLength || length > MaxPasswordLength {
		return User{}, domain.Invalidf("password must be %d to %d characters", MinPasswordLength, MaxPasswordLength)
	}
	hash, err := HashPassword(input.Password)
	if err != nil {
		return User{}, err
	}
	return User{Name: name, Email: email, PasswordHash: hash, IsAdmin: true, CreatedAt: now}, nil
}

func parseEmail(raw string) (string, error) {
	email := strings.TrimSpace(raw)
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > maxEmailLength {
		return "", domain.Invalidf("email must be a plain address such as you@example.com")
	}
	return email, nil
}
