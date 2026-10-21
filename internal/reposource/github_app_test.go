package reposource_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/reposource"
	"github.com/rbretschneider/ownership_board/internal/secretbox"
)

var fixedNow = time.Date(2026, 1, 2, 9, 0, 0, 0, time.UTC)

func fakeGitHub(t *testing.T, handler http.HandlerFunc) *reposource.GitHubApp {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return reposource.NewGitHubApp(server.Client(), "Iv1.test", server.URL, server.URL, func() time.Time { return fixedNow })
}

func TestStartDeviceRefusesAnUnexpectedVerificationPage(t *testing.T) {
	app := fakeGitHub(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"device_code":"d","user_code":"ABCD-1234","verification_uri":"https://evil.example/login","expires_in":900,"interval":5}`)
	})
	if _, err := app.StartDevice(context.Background()); err == nil {
		t.Fatal("accepted a verification page other than github.com/login/device")
	}
}

func TestStartDeviceAsksForExpiringTokens(t *testing.T) {
	var scope string
	app := fakeGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		scope = r.Form.Get("scope")
		fmt.Fprint(w, `{"device_code":"d","user_code":"ABCD-1234","verification_uri":"https://github.com/login/device","expires_in":900,"interval":5}`)
	})
	device, err := app.StartDevice(context.Background())
	if err != nil || scope != "offline_access" || device.UserCode != "ABCD-1234" || !device.ExpiresAt.Equal(fixedNow.Add(15*time.Minute)) {
		t.Fatalf("device %+v err %v scope %q", device, err, scope)
	}
}

func TestPollOutcomes(t *testing.T) {
	cases := map[string]struct {
		body         string
		wantOutcome  string
		wantInterval time.Duration
	}{
		"pending":   {`{"error":"authorization_pending"}`, reposource.PollPending, 5 * time.Second},
		"slow down": {`{"error":"slow_down","interval":10}`, reposource.PollPending, 10 * time.Second},
		"expired":   {`{"error":"expired_token"}`, reposource.PollExpired, 5 * time.Second},
		"denied":    {`{"error":"access_denied"}`, reposource.PollDenied, 5 * time.Second},
		"connected": {`{"access_token":"ghu_x","expires_in":28800,"refresh_token":"ghr_y","refresh_token_expires_in":15897600}`, reposource.PollConnected, 5 * time.Second},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			app := fakeGitHub(t, func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, tc.body) })
			result, err := app.Poll(context.Background(), reposource.DeviceCode{DeviceCode: secretbox.NewSecret("d"), Interval: 5 * time.Second})
			if err != nil || result.Outcome != tc.wantOutcome || result.Interval != tc.wantInterval {
				t.Fatalf("result %+v err %v", result, err)
			}
		})
	}
}

func TestConnectedPollCarriesExpiries(t *testing.T) {
	app := fakeGitHub(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"access_token":"ghu_x","expires_in":28800,"refresh_token":"ghr_y","refresh_token_expires_in":15897600}`)
	})
	result, _ := app.Poll(context.Background(), reposource.DeviceCode{DeviceCode: secretbox.NewSecret("d"), Interval: 5 * time.Second})
	tokens := result.Tokens
	if tokens.RefreshToken.Reveal() != "ghr_y" || !tokens.AccessExpiresAt.Equal(fixedNow.Add(8*time.Hour)) || tokens.RefreshExpiresAt == nil {
		t.Fatalf("tokens %+v", tokens)
	}
}

func TestErrorsNeverEchoTheResponseBody(t *testing.T) {
	app := fakeGitHub(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"access_token":"ghu_leaked"}`)
	})
	_, err := app.Refresh(context.Background(), secretbox.NewSecret("ghr_old"))
	if err == nil || strings.Contains(err.Error(), "ghu_leaked") || strings.Contains(err.Error(), "ghr_old") {
		t.Fatalf("error %v", err)
	}
}

func TestListReposWalksInstallationsAndSkipsSuspended(t *testing.T) {
	app := fakeGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user/installations":
			fmt.Fprint(w, `{"installations":[{"id":1},{"id":2,"suspended_at":"2026-01-01T00:00:00Z"}]}`)
		case "/user/installations/1/repositories":
			fmt.Fprint(w, `{"repositories":[{"full_name":"me/a","private":true},{"full_name":"org/b"}]}`)
		default:
			t.Errorf("unexpected call %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	listings, err := app.ListRepos(context.Background(), secretbox.NewSecret("ghu_x"))
	if err != nil || len(listings) != 2 || listings[0].FullName != "me/a" || !listings[0].IsPrivate {
		t.Fatalf("listings %+v err %v", listings, err)
	}
}

func TestTokenRequestsDoNotFollowRedirects(t *testing.T) {
	elsewhere := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("redirect followed with Authorization %q", r.Header.Get("Authorization"))
	}))
	defer elsewhere.Close()
	app := fakeGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/steal", http.StatusFound)
	})
	if _, err := app.User(context.Background(), secretbox.NewSecret("ghu_x")); err == nil {
		t.Fatal("a redirect answer was treated as success")
	}
}
