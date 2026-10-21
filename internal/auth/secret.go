package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// secretBytes is the entropy in every opaque token the board issues.
const secretBytes = 32

// newSecret returns a random URL-safe string.
func newSecret() (string, error) {
	raw := make([]byte, secretBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("auth: read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// hashSecret is how opaque tokens are stored: only their SHA-256, so a leaked
// database yields no usable token. A fast hash is right here because the input
// already has 256 bits of entropy.
func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
