package connections

import (
	"fmt"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
	"github.com/rbretschneider/ownership_board/internal/secretbox"
)

// tokenKeyPurpose names the HKDF derivation for stored provider tokens.
const tokenKeyPurpose = "ownership_board/provider-tokens/v1"

// Record is a connection with its tokens decrypted. Tokens are Secrets, so a
// Record is safe to log.
type Record struct {
	Provider         domain.Provider
	BaseURL          string
	Account          string
	AccountID        string
	Email            string
	AccessToken      secretbox.Secret
	RefreshToken     secretbox.Secret
	AccessExpiresAt  *time.Time
	RefreshExpiresAt *time.Time
	Status           Status
	Generation       int64
	ConnectedBy      int64
	ConnectedAt      time.Time
	UpdatedAt        time.Time
}

// vault seals and opens records. The associated data binds each ciphertext to
// its provider, column, base URL and generation, so a value cannot be replayed
// into another row, column, server or an older rotation.
type vault struct {
	box *secretbox.Box
}

// newVault derives the token encryption key from the board's master secret.
func newVault(masterSecret string) (vault, error) {
	key, err := secretbox.DeriveKey(masterSecret, tokenKeyPurpose)
	if err != nil {
		return vault{}, err
	}
	box, err := secretbox.New(key)
	if err != nil {
		return vault{}, err
	}
	return vault{box: box}, nil
}

func (v vault) seal(record Record) (SealedRecord, error) {
	access, err := v.box.Seal(record.AccessToken, associatedData(record, "access"))
	if err != nil {
		return SealedRecord{}, err
	}
	refresh, err := v.box.Seal(record.RefreshToken, associatedData(record, "refresh"))
	if err != nil {
		return SealedRecord{}, err
	}
	return SealedRecord{
		Provider: record.Provider, BaseURL: record.BaseURL, Account: record.Account, AccountID: record.AccountID,
		Email: record.Email, AccessTokenEnc: access, RefreshTokenEnc: refresh,
		AccessExpiresAt: record.AccessExpiresAt, RefreshExpiresAt: record.RefreshExpiresAt, Status: record.Status,
		Generation: record.Generation, ConnectedBy: record.ConnectedBy, ConnectedAt: record.ConnectedAt, UpdatedAt: record.UpdatedAt,
	}, nil
}

// open decrypts a record. A failure (corrupt value, or a changed master
// secret) is returned so the caller can ask for a reconnect.
func (v vault) open(sealed SealedRecord) (Record, error) {
	record := Record{
		Provider: sealed.Provider, BaseURL: sealed.BaseURL, Account: sealed.Account, AccountID: sealed.AccountID,
		Email: sealed.Email, AccessExpiresAt: sealed.AccessExpiresAt, RefreshExpiresAt: sealed.RefreshExpiresAt,
		Status: sealed.Status, Generation: sealed.Generation, ConnectedBy: sealed.ConnectedBy,
		ConnectedAt: sealed.ConnectedAt, UpdatedAt: sealed.UpdatedAt,
	}
	var err error
	if record.AccessToken, err = v.box.Open(sealed.AccessTokenEnc, associatedData(record, "access")); err != nil {
		return Record{}, err
	}
	if record.RefreshToken, err = v.box.Open(sealed.RefreshTokenEnc, associatedData(record, "refresh")); err != nil {
		return Record{}, err
	}
	return record, nil
}

func associatedData(record Record, column string) string {
	return fmt.Sprintf("%s|%s|%s|%d", record.Provider, column, record.BaseURL, record.Generation)
}
