package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/rbretschneider/ownership_board/internal/auth"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

// AuthStore implements auth.Store.
type AuthStore struct {
	db *DB
}

// NewAuthStore wraps the database for the auth service.
func NewAuthStore(db *DB) *AuthStore {
	return &AuthStore{db: db}
}

const userColumns = `id, name, email, password_hash, is_admin, token_version, created_at,
	EXISTS (SELECT 1 FROM identity_links l WHERE l.user_id = users.id)`

// CreateFirstUser implements auth.Store.
func (s *AuthStore) CreateFirstUser(ctx context.Context, user *auth.User) error {
	result, err := s.db.db.ExecContext(ctx,
		`INSERT INTO users (name, email, password_hash, is_admin, token_version, created_at)
		 SELECT ?, ?, ?, ?, 0, ? WHERE NOT EXISTS (SELECT 1 FROM users)`,
		user.Name, user.Email, user.PasswordHash, boolToInt(user.IsAdmin), formatTime(user.CreatedAt))
	if err != nil {
		return fmt.Errorf("sqlite: create first user: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected == 0 {
		return domain.Conflictf("setup is already complete; sign in instead")
	}
	user.ID, err = result.LastInsertId()
	return err
}

// CountUsers implements auth.Store.
func (s *AuthStore) CountUsers(ctx context.Context) (int, error) {
	var count int
	if err := s.db.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return 0, fmt.Errorf("sqlite: count users: %w", err)
	}
	return count, nil
}

// UserByID implements auth.Store.
func (s *AuthStore) UserByID(ctx context.Context, id int64) (auth.User, error) {
	user, err := scanUser(s.db.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id))
	return user, notFoundOr(err, "user %d does not exist", id)
}

// UserByEmail implements auth.Store.
func (s *AuthStore) UserByEmail(ctx context.Context, email string) (auth.User, error) {
	user, err := scanUser(s.db.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE email = ?`, email))
	return user, notFoundOr(err, "no user has that email")
}

// UserByIdentity implements auth.Store.
func (s *AuthStore) UserByIdentity(ctx context.Context, issuer, subject string) (auth.User, error) {
	user, err := scanUser(s.db.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = (SELECT user_id FROM identity_links WHERE issuer = ? AND subject = ?)`,
		issuer, subject))
	return user, notFoundOr(err, "no user is linked to that identity")
}

// LinkIdentity implements auth.Store.
func (s *AuthStore) LinkIdentity(ctx context.Context, identity auth.Identity, userID int64, now time.Time) error {
	_, err := s.db.db.ExecContext(ctx,
		`INSERT INTO identity_links (issuer, subject, user_id, created_at) VALUES (?, ?, ?, ?)`,
		identity.Issuer, identity.Subject, userID, formatTime(now))
	return conflictOr(err, "that SSO identity is already linked to an account")
}

// SaveRefreshToken implements auth.Store.
func (s *AuthStore) SaveRefreshToken(ctx context.Context, token *auth.RefreshToken) error {
	return insertRefreshToken(ctx, s.db.db, token)
}

// RefreshTokenByHash implements auth.Store.
func (s *AuthStore) RefreshTokenByHash(ctx context.Context, hash string) (auth.RefreshToken, error) {
	var token auth.RefreshToken
	var expires, created string
	var revoked sql.NullString
	err := s.db.db.QueryRowContext(ctx,
		`SELECT id, user_id, token_hash, token_version, expires_at, revoked_at, created_at FROM refresh_tokens WHERE token_hash = ?`, hash).
		Scan(&token.ID, &token.UserID, &token.Hash, &token.TokenVersion, &expires, &revoked, &created)
	if err != nil {
		return auth.RefreshToken{}, notFoundOr(err, "refresh token not found")
	}
	if token.ExpiresAt, err = parseTime(expires); err != nil {
		return auth.RefreshToken{}, err
	}
	if token.RevokedAt, err = parseOptionalTime(revoked); err != nil {
		return auth.RefreshToken{}, err
	}
	token.CreatedAt, err = parseTime(created)
	return token, err
}

// RotateRefreshToken implements auth.Store.
func (s *AuthStore) RotateRefreshToken(ctx context.Context, oldID int64, next *auth.RefreshToken, now time.Time) error {
	return s.db.inTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE refresh_tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, formatTime(now), oldID)
		if err != nil {
			return fmt.Errorf("sqlite: revoke refresh token: %w", err)
		}
		if affected, err := result.RowsAffected(); err != nil || affected == 0 {
			return domain.Conflictf("refresh token was already used")
		}
		return insertRefreshToken(ctx, tx, next)
	})
}

// RevokeRefreshToken implements auth.Store.
func (s *AuthStore) RevokeRefreshToken(ctx context.Context, id int64, now time.Time) error {
	_, err := s.db.db.ExecContext(ctx, `UPDATE refresh_tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, formatTime(now), id)
	return err
}

// RevokeUserRefreshTokens implements auth.Store.
func (s *AuthStore) RevokeUserRefreshTokens(ctx context.Context, userID int64, now time.Time) error {
	_, err := s.db.db.ExecContext(ctx, `UPDATE refresh_tokens SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`, formatTime(now), userID)
	return err
}

// CreateAPIToken implements auth.Store.
func (s *AuthStore) CreateAPIToken(ctx context.Context, token *auth.APIToken) error {
	result, err := s.db.db.ExecContext(ctx,
		`INSERT INTO api_tokens (name, prefix, token_hash, created_by, created_at) VALUES (?, ?, ?, ?, ?)`,
		token.Name, token.Prefix, token.Hash, token.CreatedBy, formatTime(token.CreatedAt))
	if err != nil {
		return conflictOr(err, "a token named %q already exists; agents are told apart by token name", token.Name)
	}
	token.ID, err = result.LastInsertId()
	return err
}

// APITokenByHash implements auth.Store.
func (s *AuthStore) APITokenByHash(ctx context.Context, hash string) (auth.APIToken, error) {
	token, err := scanAPIToken(s.db.db.QueryRowContext(ctx, `SELECT `+apiTokenColumns+` FROM api_tokens WHERE token_hash = ?`, hash))
	return token, notFoundOr(err, "token not found")
}

// ListAPITokens implements auth.Store.
func (s *AuthStore) ListAPITokens(ctx context.Context) ([]auth.APIToken, error) {
	rows, err := s.db.db.QueryContext(ctx, `SELECT `+apiTokenColumns+` FROM api_tokens WHERE revoked_at IS NULL ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list tokens: %w", err)
	}
	defer rows.Close()
	var tokens []auth.APIToken
	for rows.Next() {
		token, err := scanAPIToken(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, rows.Err()
}

// RevokeAPIToken implements auth.Store. A revoked token keeps its row (and its
// name) so history attributed to it stays unambiguous.
func (s *AuthStore) RevokeAPIToken(ctx context.Context, id int64, now time.Time) error {
	result, err := s.db.db.ExecContext(ctx, `UPDATE api_tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, formatTime(now), id)
	if err != nil {
		return fmt.Errorf("sqlite: revoke token %d: %w", id, err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected == 0 {
		return domain.NotFoundf("token %d does not exist or is already revoked", id)
	}
	return nil
}

// TouchAPIToken implements auth.Store.
func (s *AuthStore) TouchAPIToken(ctx context.Context, id int64, now time.Time) error {
	_, err := s.db.db.ExecContext(ctx, `UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, formatTime(now), id)
	return err
}

const apiTokenColumns = `id, name, prefix, token_hash, created_by, created_at, last_used_at, revoked_at`

func insertRefreshToken(ctx context.Context, q querier, token *auth.RefreshToken) error {
	result, err := q.ExecContext(ctx,
		`INSERT INTO refresh_tokens (user_id, token_hash, token_version, expires_at, created_at) VALUES (?, ?, ?, ?, ?)`,
		token.UserID, token.Hash, token.TokenVersion, formatTime(token.ExpiresAt), formatTime(token.CreatedAt))
	if err != nil {
		return fmt.Errorf("sqlite: save refresh token: %w", err)
	}
	token.ID, err = result.LastInsertId()
	return err
}

func scanUser(row scanner) (auth.User, error) {
	var user auth.User
	var isAdmin, isLinked int
	var created string
	err := row.Scan(&user.ID, &user.Name, &user.Email, &user.PasswordHash, &isAdmin, &user.TokenVersion, &created, &isLinked)
	if err != nil {
		return auth.User{}, err
	}
	user.IsAdmin, user.IsSSOLinked = isAdmin == 1, isLinked == 1
	user.CreatedAt, err = parseTime(created)
	return user, err
}

func scanAPIToken(row scanner) (auth.APIToken, error) {
	var token auth.APIToken
	var created string
	var lastUsed, revoked sql.NullString
	err := row.Scan(&token.ID, &token.Name, &token.Prefix, &token.Hash, &token.CreatedBy, &created, &lastUsed, &revoked)
	if err != nil {
		return auth.APIToken{}, err
	}
	if token.CreatedAt, err = parseTime(created); err != nil {
		return auth.APIToken{}, err
	}
	if token.LastUsedAt, err = parseOptionalTime(lastUsed); err != nil {
		return auth.APIToken{}, err
	}
	token.RevokedAt, err = parseOptionalTime(revoked)
	return token, err
}

// IsPersonName implements auth.Store.
func (s *AuthStore) IsPersonName(ctx context.Context, name string) (bool, error) {
	var exists bool
	err := s.db.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE name = ? COLLATE NOCASE)`, name).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("sqlite: check user name: %w", err)
	}
	return exists, nil
}
