package auth_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/auth"
	"github.com/rbretschneider/ownership_board/internal/clock/clocktest"
	"github.com/rbretschneider/ownership_board/internal/domain"
	"github.com/rbretschneider/ownership_board/internal/store/sqlite"
)

const (
	testSecret   = "0123456789abcdef0123456789abcdef"
	testPassword = "a long enough password"
	testEmail    = "owner@example.com"
	clientIP     = "192.0.2.10"
)

type harness struct {
	t     *testing.T
	ctx   context.Context
	clock *clocktest.Clock
	auth  *auth.Service
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	clk := clocktest.New()
	service, err := auth.NewService(sqlite.NewAuthStore(db), testSecret, clk, auth.NewThrottle(clk))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return &harness{t: t, ctx: ctx, clock: clk, auth: service}
}

func (h *harness) setup() auth.Session {
	h.t.Helper()
	session, err := h.auth.Setup(h.ctx, auth.SetupInput{Name: "Owner", Email: testEmail, Password: testPassword})
	if err != nil {
		h.t.Fatalf("Setup: %v", err)
	}
	return session
}

func TestSecretShorterThanMinimumIsRefused(t *testing.T) {
	if _, err := auth.NewService(nil, "short", clocktest.New(), nil); err == nil {
		t.Fatal("accepted a short signing secret")
	}
}

func TestSetupWorksOnlyOnce(t *testing.T) {
	h := newHarness(t)
	h.setup()
	_, err := h.auth.Setup(h.ctx, auth.SetupInput{Name: "Intruder", Email: "x@example.com", Password: testPassword})
	if domain.KindOf(err) != domain.KindConflict {
		t.Fatalf("second setup: %v", err)
	}
}

func TestSetupRejectsShortPassword(t *testing.T) {
	h := newHarness(t)
	_, err := h.auth.Setup(h.ctx, auth.SetupInput{Name: "Owner", Email: testEmail, Password: "short"})
	if domain.KindOf(err) != domain.KindInvalid {
		t.Fatalf("short password: %v", err)
	}
}

func TestAccessTokenIdentifiesUserUntilItExpires(t *testing.T) {
	h := newHarness(t)
	session := h.setup()
	user, err := h.auth.UserFromAccessToken(h.ctx, session.AccessToken)
	if err != nil || user.Email != testEmail {
		t.Fatalf("user %+v err %v", user, err)
	}
	h.clock.Advance(auth.AccessTokenLifetime + time.Second)
	if _, err := h.auth.UserFromAccessToken(h.ctx, session.AccessToken); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("expired token: %v", err)
	}
}

func TestWrongPasswordIsUnauthorized(t *testing.T) {
	h := newHarness(t)
	h.setup()
	if _, err := h.auth.Login(h.ctx, clientIP, testEmail, "not it"); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("wrong password: %v", err)
	}
	if _, err := h.auth.Login(h.ctx, clientIP, "nobody@example.com", testPassword); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("unknown email: %v", err)
	}
}

func TestRepeatedFailuresAreThrottledPerAddress(t *testing.T) {
	h := newHarness(t)
	h.setup()
	for i := 0; i < 5; i++ {
		_, _ = h.auth.Login(h.ctx, clientIP, testEmail, "wrong")
	}
	var throttled *auth.ThrottledError
	if _, err := h.auth.Login(h.ctx, clientIP, testEmail, testPassword); !errors.As(err, &throttled) {
		t.Fatalf("expected throttling, got %v", err)
	}
	if _, err := h.auth.Login(h.ctx, "192.0.2.99", testEmail, testPassword); err != nil {
		t.Fatalf("another address was locked out: %v", err)
	}
	h.clock.Advance(throttled.RetryAfter)
	if _, err := h.auth.Login(h.ctx, clientIP, testEmail, testPassword); err != nil {
		t.Fatalf("after waiting: %v", err)
	}
}

func TestRefreshRotatesAndReuseRevokesEverything(t *testing.T) {
	h := newHarness(t)
	first := h.setup()
	second, err := h.auth.Refresh(h.ctx, first.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if _, err := h.auth.Refresh(h.ctx, first.RefreshToken); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("replayed token was accepted: %v", err)
	}
	if _, err := h.auth.Refresh(h.ctx, second.RefreshToken); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("reuse did not revoke the newer session: %v", err)
	}
}

func TestLogoutEndsTheRefreshToken(t *testing.T) {
	h := newHarness(t)
	session := h.setup()
	if err := h.auth.Logout(h.ctx, session.RefreshToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := h.auth.Refresh(h.ctx, session.RefreshToken); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("refresh after logout: %v", err)
	}
}

func TestAPITokenWorksUntilRevoked(t *testing.T) {
	h := newHarness(t)
	owner := h.setup().User
	token, secret, err := h.auth.CreateAPIToken(h.ctx, owner, "loop", false)
	if err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}
	if !strings.HasPrefix(secret, auth.APITokenPrefix) || !strings.HasPrefix(secret, token.Prefix) {
		t.Fatalf("secret %q prefix %q", secret, token.Prefix)
	}
	found, err := h.auth.TokenFromSecret(h.ctx, secret)
	if err != nil || found.Actor().Kind != domain.ActorAgent || found.Actor().Name != "loop" {
		t.Fatalf("token lookup: %+v %v", found, err)
	}
	if err := h.auth.RevokeAPIToken(h.ctx, token.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := h.auth.TokenFromSecret(h.ctx, secret); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("revoked token: %v", err)
	}
}

func TestOnlyADelegateTokenMayFileAutonomousTickets(t *testing.T) {
	h := newHarness(t)
	owner := h.setup().User
	for name, isDelegate := range map[string]bool{"desktop": true, "runner": false} {
		_, secret, err := h.auth.CreateAPIToken(h.ctx, owner, name, isDelegate)
		if err != nil {
			t.Fatalf("CreateAPIToken %s: %v", name, err)
		}
		found, err := h.auth.TokenFromSecret(h.ctx, secret)
		if err != nil || found.Actor().MayFileAutonomous() != isDelegate {
			t.Fatalf("%s: delegate %v, want %v (%v)", name, found.Actor().MayFileAutonomous(), isDelegate, err)
		}
	}
}

func TestSSOSignInNeedsALinkedAccount(t *testing.T) {
	h := newHarness(t)
	owner := h.setup().User
	identity := auth.Identity{Issuer: "https://idp.example.com", Subject: "abc", Email: testEmail, IsEmailVerified: true}
	if _, err := h.auth.SignInWithIdentity(h.ctx, identity); !errors.Is(err, auth.ErrNotLinked) {
		t.Fatalf("unlinked identity matched by email: %v", err)
	}
	if err := h.auth.LinkIdentity(h.ctx, owner.ID, identity); err != nil {
		t.Fatalf("LinkIdentity: %v", err)
	}
	session, err := h.auth.SignInWithIdentity(h.ctx, identity)
	if err != nil || session.User.ID != owner.ID || !session.User.IsSSOLinked {
		t.Fatalf("linked sign-in: %+v %v", session.User, err)
	}
}

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := auth.HashPassword(testPassword)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if isMatch, err := auth.VerifyPassword(testPassword, hash); err != nil || !isMatch {
		t.Fatalf("verify right password: %v %v", isMatch, err)
	}
	if isMatch, _ := auth.VerifyPassword("wrong", hash); isMatch {
		t.Fatal("wrong password matched")
	}
}

func TestAgentTokenCannotTakeAPersonsName(t *testing.T) {
	h := newHarness(t)
	owner := h.setup().User
	if _, _, err := h.auth.CreateAPIToken(h.ctx, owner, "owner", false); domain.KindOf(err) != domain.KindConflict {
		t.Fatalf("token named like a person: %v", err)
	}
}

func TestARevokedTokensNameIsNeverReused(t *testing.T) {
	h := newHarness(t)
	owner := h.setup().User
	token, _, err := h.auth.CreateAPIToken(h.ctx, owner, "desktop", true)
	if err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}
	if err := h.auth.RevokeAPIToken(h.ctx, token.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, _, err := h.auth.CreateAPIToken(h.ctx, owner, "desktop", false); domain.KindOf(err) != domain.KindConflict {
		t.Fatalf("a new token took a revoked token's name: %v", err)
	}
}
