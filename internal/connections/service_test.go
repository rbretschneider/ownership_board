package connections_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/auth"
	"github.com/rbretschneider/ownership_board/internal/clock/clocktest"
	"github.com/rbretschneider/ownership_board/internal/connections"
	"github.com/rbretschneider/ownership_board/internal/domain"
	"github.com/rbretschneider/ownership_board/internal/notify"
	"github.com/rbretschneider/ownership_board/internal/reposource"
	"github.com/rbretschneider/ownership_board/internal/store/sqlite"
)

const (
	masterSecret  = "0123456789abcdef0123456789abcdef"
	accessCanary  = "ghu_ACCESS_CANARY"
	refreshCanary = "ghr_REFRESH_CANARY"
)

// fakeGitHub stands in for github.com and api.github.com.
type fakeGitHub struct {
	server        *httptest.Server
	tokenAnswer   atomic.Value // string: the device-grant answer
	refreshStatus atomic.Int32 // HTTP status for refresh calls
	refreshBody   atomic.Value // string
	refreshCalls  atomic.Int32
	pollCalls     atomic.Int32
	login         string
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	t.Helper()
	fake := &fakeGitHub{login: "octocat"}
	fake.tokenAnswer.Store(`{"error":"authorization_pending"}`)
	fake.refreshStatus.Store(http.StatusOK)
	fake.refreshBody.Store(`{"access_token":"ghu_NEW","expires_in":28800,"refresh_token":"ghr_NEW","refresh_token_expires_in":15897600}`)
	fake.server = httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeGitHub) serve(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	switch r.URL.Path {
	case "/login/device/code":
		fmt.Fprint(w, `{"device_code":"DEVICE_SECRET","user_code":"WDJB-MJHT","verification_uri":"https://github.com/login/device","expires_in":900,"interval":5}`)
	case "/login/oauth/access_token":
		f.token(w, r)
	case "/user":
		fmt.Fprintf(w, `{"id":583231,"login":%q}`, f.login)
	case "/user/installations":
		fmt.Fprint(w, `{"installations":[{"id":7}]}`)
	case "/user/installations/7/repositories":
		fmt.Fprint(w, `{"repositories":[{"full_name":"octocat/hello","private":true}]}`)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeGitHub) token(w http.ResponseWriter, r *http.Request) {
	if r.Form.Get("grant_type") == "refresh_token" {
		f.refreshCalls.Add(1)
		time.Sleep(20 * time.Millisecond)
		w.WriteHeader(int(f.refreshStatus.Load()))
		fmt.Fprint(w, f.refreshBody.Load().(string))
		return
	}
	f.pollCalls.Add(1)
	fmt.Fprint(w, f.tokenAnswer.Load().(string))
}

func (f *fakeGitHub) approve(expiresIn int) {
	f.tokenAnswer.Store(fmt.Sprintf(`{"access_token":%q,"expires_in":%d,"refresh_token":%q,"refresh_token_expires_in":15897600}`,
		accessCanary, expiresIn, refreshCanary))
}

type harness struct {
	t       *testing.T
	ctx     context.Context
	clock   *clocktest.Clock
	github  *fakeGitHub
	db      *sqlite.DB
	repos   *app.RepoService
	catalog *connections.Catalog
	service *connections.Service
	admin   connections.Admin
	logger  *slog.Logger
}

func newHarness(t *testing.T, policy connections.Policy) *harness {
	return newHarnessLogging(t, policy, discard())
}

func newHarnessLogging(t *testing.T, policy connections.Policy, logger *slog.Logger) *harness {
	t.Helper()
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "board.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	owner := auth.User{Name: "Owner", Email: "owner@example.com", IsAdmin: true, CreatedAt: time.Now()}
	if err := sqlite.NewAuthStore(db).CreateFirstUser(ctx, &owner); err != nil {
		t.Fatalf("create user: %v", err)
	}
	h := &harness{t: t, ctx: ctx, clock: clocktest.New(), github: newFakeGitHub(t), db: db}
	h.admin = connections.Admin{ID: owner.ID, Name: owner.Name, IsAdmin: true}
	h.logger = logger
	h.catalog = h.newCatalog(masterSecret)
	h.repos = app.NewRepoService(db, h.clock, h.catalog, logger)
	h.service = connections.NewService(h.catalog, h.repos, notify.Noop{}, policy)
	return h
}

func (h *harness) newCatalog(secret string) *connections.Catalog {
	h.t.Helper()
	client := h.github.server.Client()
	catalog, err := connections.NewCatalog(connections.CatalogDeps{
		Store: sqlite.NewConnectionStore(h.db), MasterSecret: secret,
		GitHub: reposource.NewGitHubApp(client, "Iv1.test", h.github.server.URL, h.github.server.URL, h.clock.Now),
		HTTP:   client, Clock: h.clock, Logger: h.logger,
	})
	if err != nil {
		h.t.Fatalf("NewCatalog: %v", err)
	}
	return catalog
}

// connect runs a whole device sign-in and confirms it.
func (h *harness) connect(expiresIn int) {
	h.t.Helper()
	flow, err := h.service.StartGitHub(h.ctx, h.admin)
	if err != nil {
		h.t.Fatalf("StartGitHub: %v", err)
	}
	h.github.approve(expiresIn)
	h.clock.Advance(flow.Interval)
	poll, err := h.service.PollGitHub(h.ctx, h.admin, flow.ID)
	if err != nil || poll.Status != connections.PollConfirm {
		h.t.Fatalf("poll = %+v, %v", poll, err)
	}
	if _, err := h.service.ConfirmGitHub(h.ctx, h.admin, flow.ID); err != nil {
		h.t.Fatalf("ConfirmGitHub: %v", err)
	}
}

func (h *harness) githubView() connections.View {
	h.t.Helper()
	views, err := h.service.List(h.ctx, h.admin)
	if err != nil {
		h.t.Fatalf("List: %v", err)
	}
	return views[0]
}

func discard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestPollBeforeTheIntervalDoesNotCallGitHub(t *testing.T) {
	h := newHarness(t, connections.Policy{})
	flow, _ := h.service.StartGitHub(h.ctx, h.admin)
	poll, err := h.service.PollGitHub(h.ctx, h.admin, flow.ID)
	if err != nil || poll.Status != reposource.PollPending || h.github.pollCalls.Load() != 0 {
		t.Fatalf("poll %+v err %v calls %d", poll, err, h.github.pollCalls.Load())
	}
}

func TestNothingIsStoredUntilTheAdminConfirms(t *testing.T) {
	h := newHarness(t, connections.Policy{})
	flow, _ := h.service.StartGitHub(h.ctx, h.admin)
	h.github.approve(28800)
	h.clock.Advance(flow.Interval)
	poll, err := h.service.PollGitHub(h.ctx, h.admin, flow.ID)
	if err != nil || poll.Status != connections.PollConfirm || poll.Login != "octocat" {
		t.Fatalf("poll %+v err %v", poll, err)
	}
	if status := h.githubView().Status; status != connections.StatusDisconnected {
		t.Fatalf("stored before confirmation: %s", status)
	}
	if _, err := h.service.ConfirmGitHub(h.ctx, h.admin, flow.ID); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if view := h.githubView(); view.Status != connections.StatusConnected || view.Account != "octocat" {
		t.Fatalf("after confirm: %+v", view)
	}
}

func TestTokensAreEncryptedAtRest(t *testing.T) {
	h := newHarness(t, connections.Policy{})
	h.connect(28800)
	var access, refresh string
	row := h.db.SQL().QueryRow(`SELECT access_token_enc, refresh_token_enc FROM provider_connections WHERE provider = 'github'`)
	if err := row.Scan(&access, &refresh); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if access == "" || strings.Contains(access, accessCanary) || strings.Contains(refresh, refreshCanary) {
		t.Fatalf("token stored in the clear: %q %q", access, refresh)
	}
}

func TestAnotherPersonCannotTouchAFlow(t *testing.T) {
	h := newHarness(t, connections.Policy{})
	flow, _ := h.service.StartGitHub(h.ctx, h.admin)
	stranger := connections.Admin{ID: h.admin.ID + 1, Name: "x", IsAdmin: true}
	_, err := h.service.PollGitHub(h.ctx, stranger, flow.ID)
	if domain.KindOf(err) != domain.KindNotFound {
		t.Fatalf("foreign poll: %v", err)
	}
	_, err = h.service.PollGitHub(h.ctx, h.admin, "made-up")
	if domain.KindOf(err) != domain.KindNotFound {
		t.Fatalf("unknown flow: %v", err)
	}
}

func TestOnlyAdminsManageConnections(t *testing.T) {
	h := newHarness(t, connections.Policy{})
	member := connections.Admin{ID: h.admin.ID, Name: "x", IsAdmin: false}
	if _, err := h.service.List(h.ctx, member); domain.KindOf(err) != domain.KindForbidden {
		t.Fatalf("non-admin list: %v", err)
	}
	if _, err := h.service.StartGitHub(h.ctx, member); domain.KindOf(err) != domain.KindForbidden {
		t.Fatalf("non-admin start: %v", err)
	}
}

func TestStartingAgainRightAwayIsRefused(t *testing.T) {
	h := newHarness(t, connections.Policy{})
	if _, err := h.service.StartGitHub(h.ctx, h.admin); err != nil {
		t.Fatalf("first start: %v", err)
	}
	if _, err := h.service.StartGitHub(h.ctx, h.admin); domain.KindOf(err) != domain.KindConflict {
		t.Fatalf("second start: %v", err)
	}
}

func TestLoginOutsideTheAllowListIsDenied(t *testing.T) {
	h := newHarness(t, connections.Policy{AllowedGitHubLogins: []string{"@someone-else"}})
	flow, _ := h.service.StartGitHub(h.ctx, h.admin)
	h.github.approve(28800)
	h.clock.Advance(flow.Interval)
	poll, err := h.service.PollGitHub(h.ctx, h.admin, flow.ID)
	if err != nil || poll.Status != reposource.PollDenied || !strings.Contains(poll.Message, "octocat") {
		t.Fatalf("poll %+v err %v", poll, err)
	}
	if _, err := h.service.ConfirmGitHub(h.ctx, h.admin, flow.ID); domain.KindOf(err) != domain.KindNotFound {
		t.Fatalf("confirm after denial: %v", err)
	}
}

func TestSyncListsRepoThroughTheConnection(t *testing.T) {
	h := newHarness(t, connections.Policy{})
	h.connect(0)
	if err := h.repos.SyncProvider(h.ctx, domain.ProviderGitHub); err != nil {
		t.Fatalf("sync: %v", err)
	}
	repos, _ := h.repos.List(h.ctx)
	if len(repos) != 1 || repos[0].FullName != "octocat/hello" || !repos[0].IsPrivate {
		t.Fatalf("repos %+v", repos)
	}
}

func TestConcurrentSyncsRefreshOnlyOnce(t *testing.T) {
	h := newHarness(t, connections.Policy{})
	h.connect(60)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = h.repos.SyncProvider(h.ctx, domain.ProviderGitHub)
		}()
	}
	wg.Wait()
	if calls := h.github.refreshCalls.Load(); calls != 1 {
		t.Fatalf("refresh token spent %d times", calls)
	}
	if status := h.githubView().Status; status != connections.StatusConnected {
		t.Fatalf("status %s after refresh", status)
	}
}

func TestRefusedRefreshAsksForReconnect(t *testing.T) {
	h := newHarness(t, connections.Policy{})
	h.connect(60)
	h.github.refreshBody.Store(`{"error":"bad_refresh_token"}`)
	_ = h.repos.SyncProvider(h.ctx, domain.ProviderGitHub)
	if status := h.githubView().Status; status != connections.StatusNeedsReconnect {
		t.Fatalf("status %s", status)
	}
}

func TestTransientRefreshFailureKeepsTheConnection(t *testing.T) {
	h := newHarness(t, connections.Policy{})
	h.connect(60)
	h.github.refreshStatus.Store(http.StatusBadGateway)
	_ = h.repos.SyncProvider(h.ctx, domain.ProviderGitHub)
	view := h.githubView()
	if view.Status != connections.StatusConnected || view.LastSync == nil || view.LastSync.Error == "" {
		t.Fatalf("view %+v", view)
	}
	if strings.Contains(view.LastSync.Error, "ghr_") || strings.Contains(view.LastSync.Error, refreshCanary) {
		t.Fatalf("sync error leaks a token: %q", view.LastSync.Error)
	}
}

func TestChangedMasterSecretAsksForReconnect(t *testing.T) {
	h := newHarness(t, connections.Policy{})
	h.connect(28800)
	rotated := h.newCatalog("ffffffffffffffffffffffffffffffff")
	sources, err := rotated.Sources(h.ctx)
	if err != nil {
		t.Fatalf("Sources: %v", err)
	}
	for _, source := range sources {
		if source.Provider() == domain.ProviderGitHub {
			t.Fatal("an undecryptable connection was still offered as a source")
		}
	}
}

// TestTokensNeverLeak drives every path that touches a token, then checks no
// canary appears in any log line or any view the API would serialise.
func TestTokensNeverLeak(t *testing.T) {
	var logged strings.Builder
	var mu sync.Mutex
	logger := slog.New(slog.NewTextHandler(writerFunc(func(p []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		return logged.Write(p)
	}), &slog.HandlerOptions{Level: slog.LevelDebug}))
	h := newHarnessLogging(t, connections.Policy{}, logger)
	h.connect(60)
	h.github.refreshStatus.Store(http.StatusBadGateway)
	_ = h.repos.SyncProvider(h.ctx, domain.ProviderGitHub)
	h.github.refreshStatus.Store(http.StatusOK)
	h.github.refreshBody.Store(`{"error":"bad_refresh_token"}`)
	_ = h.repos.SyncProvider(h.ctx, domain.ProviderGitHub)
	_, _ = h.service.ConnectForgejo(h.ctx, h.admin, h.github.server.URL, "fj_FORGEJO_CANARY")
	views, err := h.service.List(h.ctx, h.admin)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	encoded, _ := json.Marshal(views)
	mu.Lock()
	outputs := logged.String() + string(encoded) + fmt.Sprintf("%+v %#v", views, views)
	mu.Unlock()
	for _, canary := range []string{accessCanary, refreshCanary, "DEVICE_SECRET", "fj_FORGEJO_CANARY", "ghu_NEW", "ghr_NEW"} {
		if strings.Contains(outputs, canary) {
			t.Fatalf("%s leaked into logs or views", canary)
		}
	}
}

func TestFailedValidationGivesOneFixedMessage(t *testing.T) {
	h := newHarness(t, connections.Policy{})
	_, notForgejo := h.service.ConnectForgejo(h.ctx, h.admin, h.github.server.URL, "token")
	_, unreachable := h.service.ConnectForgejo(h.ctx, h.admin, "http://127.0.0.1:1", "token")
	if notForgejo == nil || unreachable == nil || notForgejo.Error() != unreachable.Error() {
		t.Fatalf("messages differ, which reveals what is at the address: %q vs %q", notForgejo, unreachable)
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }
