package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters, following the OWASP recommendation for interactive
// logins on a small server.
const (
	argonTime      = 3
	argonMemoryKiB = 64 * 1024
	argonThreads   = 2
	argonKeyLength = 32
	argonSaltBytes = 16
	argonVersion   = argon2.Version
	// MinPasswordLength is the shortest password accepted at setup.
	MinPasswordLength = 12
	// MaxPasswordLength stops a huge password from becoming a CPU sink.
	MaxPasswordLength = 256
)

var errMalformedHash = errors.New("auth: malformed password hash")

// HashPassword returns an encoded argon2id hash of password.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: read salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemoryKiB, argonThreads, argonKeyLength)
	encoding := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argonVersion, argonMemoryKiB, argonTime, argonThreads, encoding.EncodeToString(salt), encoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches an encoded hash, comparing
// in constant time.
func VerifyPassword(password, encoded string) (bool, error) {
	params, salt, key, err := decodeHash(encoded)
	if err != nil {
		return false, err
	}
	candidate := argon2.IDKey([]byte(password), salt, params.time, params.memory, params.threads, uint32(len(key)))
	return subtle.ConstantTimeCompare(candidate, key) == 1, nil
}

type argonParams struct {
	memory  uint32
	time    uint32
	threads uint8
}

func decodeHash(encoded string) (argonParams, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return argonParams{}, nil, nil, errMalformedHash
	}
	var version int
	var params argonParams
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argonVersion {
		return argonParams{}, nil, nil, errMalformedHash
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.memory, &params.time, &params.threads); err != nil {
		return argonParams{}, nil, nil, errMalformedHash
	}
	salt, saltErr := base64.RawStdEncoding.DecodeString(parts[4])
	key, keyErr := base64.RawStdEncoding.DecodeString(parts[5])
	if saltErr != nil || keyErr != nil {
		return argonParams{}, nil, nil, errMalformedHash
	}
	return params, salt, key, nil
}
