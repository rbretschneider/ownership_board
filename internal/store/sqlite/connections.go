package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/rbretschneider/ownership_board/internal/connections"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

// ConnectionStore implements connections.Store. Token columns arrive already
// encrypted; this adapter never sees plaintext.
type ConnectionStore struct {
	db *DB
}

// NewConnectionStore wraps the database for provider connections.
func NewConnectionStore(db *DB) *ConnectionStore {
	return &ConnectionStore{db: db}
}

const connectionColumns = `provider, base_url, account, account_id, email, access_token_enc, refresh_token_enc,
	access_expires_at, refresh_expires_at, status, generation, connected_by, connected_at, updated_at`

// Get implements connections.Store.
func (s *ConnectionStore) Get(ctx context.Context, provider domain.Provider) (connections.SealedRecord, bool, error) {
	row := s.db.db.QueryRowContext(ctx, `SELECT `+connectionColumns+` FROM provider_connections WHERE provider = ?`, string(provider))
	record, err := scanConnection(row)
	if errors.Is(err, sql.ErrNoRows) {
		return connections.SealedRecord{}, false, nil
	}
	if err != nil {
		return connections.SealedRecord{}, false, fmt.Errorf("sqlite: read connection %s: %w", provider, err)
	}
	return record, true, nil
}

// Put implements connections.Store as a compare-and-swap on generation.
func (s *ConnectionStore) Put(ctx context.Context, record connections.SealedRecord, expectedGeneration int64) (bool, error) {
	var isWritten bool
	err := s.db.inTx(ctx, func(tx *sql.Tx) error {
		var err error
		if expectedGeneration == 0 {
			isWritten, err = insertConnection(ctx, tx, record)
		} else {
			isWritten, err = updateConnection(ctx, tx, record, expectedGeneration)
		}
		return err
	})
	return isWritten, err
}

// Delete implements connections.Store.
func (s *ConnectionStore) Delete(ctx context.Context, provider domain.Provider) error {
	if _, err := s.db.db.ExecContext(ctx, `DELETE FROM provider_connections WHERE provider = ?`, string(provider)); err != nil {
		return fmt.Errorf("sqlite: delete connection %s: %w", provider, err)
	}
	return nil
}

func insertConnection(ctx context.Context, tx *sql.Tx, r connections.SealedRecord) (bool, error) {
	result, err := tx.ExecContext(ctx,
		`INSERT INTO provider_connections (`+connectionColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (provider) DO NOTHING`,
		connectionValues(r)...)
	return affectedOne(result, err)
}

func updateConnection(ctx context.Context, tx *sql.Tx, r connections.SealedRecord, expected int64) (bool, error) {
	values := connectionValues(r)
	args := append(values[1:], values[0], expected)
	result, err := tx.ExecContext(ctx,
		`UPDATE provider_connections SET base_url = ?, account = ?, account_id = ?, email = ?, access_token_enc = ?,
			refresh_token_enc = ?, access_expires_at = ?, refresh_expires_at = ?, status = ?, generation = ?,
			connected_by = ?, connected_at = ?, updated_at = ?
		 WHERE provider = ? AND generation = ?`, args...)
	return affectedOne(result, err)
}

func affectedOne(result sql.Result, err error) (bool, error) {
	if err != nil {
		return false, fmt.Errorf("sqlite: write connection: %w", err)
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

func connectionValues(r connections.SealedRecord) []any {
	return []any{
		string(r.Provider), r.BaseURL, r.Account, r.AccountID, r.Email, r.AccessTokenEnc, r.RefreshTokenEnc,
		formatOptionalTime(r.AccessExpiresAt), formatOptionalTime(r.RefreshExpiresAt), string(r.Status), r.Generation,
		r.ConnectedBy, formatTime(r.ConnectedAt), formatTime(r.UpdatedAt),
	}
}

func scanConnection(row scanner) (connections.SealedRecord, error) {
	var r connections.SealedRecord
	var provider, status, connectedAt, updatedAt string
	var accessExpires, refreshExpires sql.NullString
	err := row.Scan(&provider, &r.BaseURL, &r.Account, &r.AccountID, &r.Email, &r.AccessTokenEnc, &r.RefreshTokenEnc,
		&accessExpires, &refreshExpires, &status, &r.Generation, &r.ConnectedBy, &connectedAt, &updatedAt)
	if err != nil {
		return connections.SealedRecord{}, err
	}
	r.Provider, r.Status = domain.Provider(provider), connections.Status(status)
	if r.AccessExpiresAt, err = parseOptionalTime(accessExpires); err != nil {
		return connections.SealedRecord{}, err
	}
	if r.RefreshExpiresAt, err = parseOptionalTime(refreshExpires); err != nil {
		return connections.SealedRecord{}, err
	}
	if r.ConnectedAt, err = parseTime(connectedAt); err != nil {
		return connections.SealedRecord{}, err
	}
	r.UpdatedAt, err = parseTime(updatedAt)
	return r, err
}
