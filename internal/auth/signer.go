package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/rbretschneider/ownership_board/internal/clock"
)

// MinSecretLength is the shortest signing secret the board accepts.
const MinSecretLength = 32

const issuerName = "ownership_board"

// Audiences keep a token minted for one purpose from being accepted for
// another, such as an OIDC flow cookie presented as an access token.
const (
	audienceAccess   = "access"
	audienceOIDCFlow = "oidc-flow"
)

// signer mints and checks HMAC-signed JWTs.
type signer struct {
	secret []byte
	clock  clock.Clock
}

func newSigner(secret string, clk clock.Clock) (signer, error) {
	if len(secret) < MinSecretLength {
		return signer{}, fmt.Errorf("auth: signing secret must be at least %d characters", MinSecretLength)
	}
	return signer{secret: []byte(secret), clock: clk}, nil
}

func (s signer) sign(audience string, lifetime time.Duration, claims map[string]any) (string, time.Time, error) {
	now := s.clock.Now()
	expires := now.Add(lifetime)
	payload := jwt.MapClaims{"iss": issuerName, "aud": audience, "iat": now.Unix(), "exp": expires.Unix()}
	for key, value := range claims {
		payload[key] = value
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, payload).SignedString(s.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: sign token: %w", err)
	}
	return token, expires, nil
}

func (s signer) verify(audience, raw string) (jwt.MapClaims, error) {
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) { return s.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(issuerName),
		jwt.WithAudience(audience),
		jwt.WithTimeFunc(s.clock.Now),
		jwt.WithExpirationRequired())
	if err != nil {
		return nil, errors.Join(ErrUnauthorized, err)
	}
	return claims, nil
}

func claimString(claims jwt.MapClaims, key string) string {
	value, _ := claims[key].(string)
	return value
}

func claimInt(claims jwt.MapClaims, key string) int64 {
	value, _ := claims[key].(float64)
	return int64(value)
}
