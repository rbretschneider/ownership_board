// Package secretbox encrypts small secrets, such as provider tokens, for
// storage at rest.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// keyLength selects AES-256.
const keyLength = 32

// formatVersion prefixes every sealed value so the format can change later.
const formatVersion = "v1"

// keyIDLength is how many hex characters of the key's hash identify it.
const keyIDLength = 8

// ErrCannotOpen means a sealed value is corrupt or is being opened with
// different associated data than it was sealed with.
var ErrCannotOpen = errors.New("secretbox: cannot open sealed value")

// ErrWrongKey means a value was sealed under a different key, typically after
// the master secret changed. The credential has to be entered again.
var ErrWrongKey = errors.New("secretbox: value was sealed under a different key")

// Box seals and opens values with AES-256-GCM. It is safe for concurrent use.
type Box struct {
	aead  cipher.AEAD
	keyID string
}

// DeriveKey derives a purpose-specific key from a master secret with
// HKDF-SHA256, so one configured secret can serve several purposes without
// the keys being related in any usable way.
func DeriveKey(masterSecret, purpose string) ([]byte, error) {
	if masterSecret == "" || purpose == "" {
		return nil, errors.New("secretbox: master secret and purpose are required")
	}
	return hkdf.Key(sha256.New, []byte(masterSecret), nil, purpose, keyLength)
}

// New builds a box from a 32-byte key.
func New(key []byte) (*Box, error) {
	if len(key) != keyLength {
		return nil, fmt.Errorf("secretbox: key must be %d bytes", keyLength)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secretbox: cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secretbox: gcm: %w", err)
	}
	digest := sha256.Sum256(key)
	return &Box{aead: aead, keyID: hex.EncodeToString(digest[:])[:keyIDLength]}, nil
}

// Seal encrypts plaintext. associatedData binds the ciphertext to where it is
// stored (for example its row and column), so it cannot be moved elsewhere and
// still open. An empty plaintext seals to an empty string.
func (b *Box) Seal(plaintext Secret, associatedData string) (string, error) {
	if plaintext.IsEmpty() {
		return "", nil
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("secretbox: nonce: %w", err)
	}
	sealed := b.aead.Seal(nonce, nonce, []byte(plaintext.Reveal()), []byte(associatedData))
	return formatVersion + "." + b.keyID + ":" + base64.RawStdEncoding.EncodeToString(sealed), nil
}

// Open decrypts a value from Seal with the same associated data.
func (b *Box) Open(sealed, associatedData string) (Secret, error) {
	if sealed == "" {
		return Secret{}, nil
	}
	header, encoded, hasHeader := strings.Cut(sealed, ":")
	version, keyID, hasKeyID := strings.Cut(header, ".")
	if !hasHeader || !hasKeyID || version != formatVersion {
		return Secret{}, ErrCannotOpen
	}
	if keyID != b.keyID {
		return Secret{}, ErrWrongKey
	}
	raw, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil || len(raw) < b.aead.NonceSize() {
		return Secret{}, ErrCannotOpen
	}
	nonce, ciphertext := raw[:b.aead.NonceSize()], raw[b.aead.NonceSize():]
	plaintext, err := b.aead.Open(nil, nonce, ciphertext, []byte(associatedData))
	if err != nil {
		return Secret{}, ErrCannotOpen
	}
	return NewSecret(string(plaintext)), nil
}
