package httpapi

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/auth"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestRouterAuthorizationOrderAndExistingSecurityGates(t *testing.T) {
	rt := router{origin: "https://board.example", writes: newWriteLimiter(time.Now)}
	request := httptest.NewRequest("POST", "https://board.example/api/v1/example", nil)
	if err := rt.authorize(request, caller{}, accessHuman); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal(err)
	}
	agent := caller{actor: domain.Actor{Kind: domain.ActorAgent, ID: 1}, isBearer: true}
	for i := 0; i < agentWritesPerWindow; i++ {
		if err := rt.authorize(request, agent, accessAgent); err != nil {
			t.Fatal(err)
		}
	}
	var limited *rateLimitedError
	if err := rt.authorize(request, agent, accessAgent); !errors.As(err, &limited) {
		t.Fatal("authorized agent escaped limiter", err)
	}
	if err := rt.authorize(request, agent, accessHuman); domain.KindOf(err) != domain.KindForbidden {
		t.Fatal("limiter masked access denial", err)
	}
	human := caller{actor: domain.Actor{Kind: domain.ActorHuman, ID: 1}}
	if err := rt.authorize(request, human, accessHuman); domain.KindOf(err) != domain.KindForbidden {
		t.Fatal("missing origin accepted", err)
	}
	request.Header.Set("Origin", "https://foreign.example")
	if err := rt.authorize(request, human, accessHuman); domain.KindOf(err) != domain.KindForbidden {
		t.Fatal("foreign origin accepted", err)
	}
	request.Header.Set("Origin", rt.origin)
	if err := rt.authorize(request, human, accessHuman); err != nil {
		t.Fatal(err)
	}
	// Public setup/login still need Origin, but do not require an existing account.
	if err := rt.authorize(request, caller{}, accessPublic); err != nil {
		t.Fatal(err)
	}
	request.Header.Del("Origin")
	if err := rt.authorize(request, caller{}, accessPublic); domain.KindOf(err) != domain.KindForbidden {
		t.Fatal("public write lost CSRF gate", err)
	}
	request.Method = "GET"
	if err := rt.authorize(request, caller{}, accessPublic); err != nil {
		t.Fatal(err)
	}
	if err := rt.authorize(request, human, accessAgent); domain.KindOf(err) != domain.KindForbidden {
		t.Fatal("human entered agent route", err)
	}
}
