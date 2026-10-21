package reposource_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
	"github.com/rbretschneider/ownership_board/internal/reposource"
	"github.com/rbretschneider/ownership_board/internal/secretbox"
)

func TestGuardedClientRefusesLoopbackAndHidesWhy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("guarded client reached a loopback server")
		fmt.Fprint(w, `{"id":1,"login":"x"}`)
	}))
	defer server.Close()
	client := reposource.NewHTTPClient(reposource.ClientOptions{Timeout: 5 * time.Second})
	_, err := reposource.NewForgejo(client, server.URL, secretbox.NewSecret("t")).Whoami(context.Background())
	if !errors.Is(err, reposource.ErrUnreachable) {
		t.Fatalf("error = %v, want the generic unreachable error", err)
	}
}

func TestForgejoWhoamiRequiresAForgejoShapedAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"status":"ok","banner":"some internal service"}`)
	}))
	defer server.Close()
	client := reposource.NewHTTPClient(reposource.ClientOptions{Timeout: 5 * time.Second, AllowLoopback: true})
	_, err := reposource.NewForgejo(client, server.URL, secretbox.NewSecret("t")).Whoami(context.Background())
	if err == nil || strings.Contains(err.Error(), "banner") {
		t.Fatalf("error = %v", err)
	}
}

func TestNormalizeForgejoURL(t *testing.T) {
	cases := map[string]struct {
		raw  string
		want string
		kind domain.ErrorKind
	}{
		"plain":           {"https://Git.Example.com/", "https://git.example.com", 0},
		"subpath":         {"http://192.168.1.50:3021/forge/", "http://192.168.1.50:3021/forge", 0},
		"credentials":     {"https://user:pass@git.example.com", "", domain.KindInvalid},
		"query":           {"https://git.example.com/?x=1", "", domain.KindInvalid},
		"fragment":        {"https://git.example.com/#x", "", domain.KindInvalid},
		"other scheme":    {"file:///etc/passwd", "", domain.KindInvalid},
		"not allowlisted": {"https://evil.example.com", "", domain.KindForbidden},
		"wrong port":      {"http://192.168.1.50:2375", "", domain.KindForbidden},
		"dot segment":     {"https://git.example.com/a/../b", "", domain.KindInvalid},
		"encoded dot":     {"https://git.example.com/%2e%2e/x", "", domain.KindInvalid},
		"port too big":    {"https://git.example.com:99999", "", domain.KindInvalid},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			allowed := []string{"git.example.com", "192.168.1.50:3021"}
			got, err := reposource.NormalizeForgejoURL(tc.raw, allowed)
			if domain.KindOf(err) != tc.kind || got != tc.want {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
}

func TestCheckTokenRefusesUnusualCharacters(t *testing.T) {
	for _, token := range []string{"", "has space", "tab\there", "newline\nx", "nul\x00x", "ünïcode"} {
		if _, err := reposource.CheckToken(token); domain.KindOf(err) != domain.KindInvalid {
			t.Errorf("%q accepted", token)
		}
	}
	if _, err := reposource.CheckToken("  ghp_Valid-token_123  "); err != nil {
		t.Fatalf("valid token refused: %v", err)
	}
}
