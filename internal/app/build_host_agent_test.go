package app_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/auth"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

const machineFacts = `{"os":"linux","os_version":"","arch":"amd64","hostname":"builder","lan_ip":"","cpu_model":"","cores":2,"threads":4,"ram_bytes":0,"disks":[],"gpus":[],"toolchains":{"cmake":"3"},"interactive_session":false,"build_user_is_admin":false,"agent_version":"1"}`

type hostNotifier struct {
	mu sync.Mutex
	n  int
}

func (n *hostNotifier) Notify(context.Context, app.Notification) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.n++
	return errors.New("transport failure")
}
func TestBuildHostEnrollmentLifecycle(t *testing.T) {
	f := newFixture(t)
	f.store.SQL().Exec(`INSERT INTO users(id,name,email,created_at) VALUES(1,'Owner','o@example.com','2026-01-01T00:00:00.000Z')`)
	notify := &hostNotifier{}
	s := app.NewBuildHostService(f.store, f.clock, nil, notify)
	code, e := s.CreatePairingCode(f.ctx, owner)
	if e != nil {
		t.Fatal(e)
	}
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	input := app.BuildHostPairInput{Code: code.Code.Reveal(), Name: "Builder", PublicKey: base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey)), Facts: []byte(machineFacts)}
	h, e := s.Pair(f.ctx, input)
	if e != nil {
		t.Fatal(e)
	}
	if h.Status != domain.BuildHostPending || len(h.Capabilities) != 1 {
		t.Fatal(h)
	}
	if _, e = s.Pair(f.ctx, input); !errors.Is(e, auth.ErrUnauthorized) {
		t.Fatal(e)
	}
	if notify.n != 1 {
		t.Fatal(notify.n)
	}
	if _, e = s.Heartbeat(f.ctx, app.AuthenticatedBuildHost{ID: h.ID}, domain.BuildHostReport{Facts: []byte(machineFacts), RunningJobIDs: []int64{}, FreeDiskBytes: 1}); !errors.Is(e, auth.ErrUnauthorized) {
		t.Fatal(e)
	}
	if _, e = s.Approve(f.ctx, owner, h.ID, domain.BuildHostEdit{}); e != nil {
		t.Fatal(e)
	}
	report := domain.BuildHostReport{Facts: []byte(machineFacts), RunningJobIDs: []int64{7}, FreeDiskBytes: 42}
	h, e = s.Heartbeat(f.ctx, app.AuthenticatedBuildHost{ID: h.ID}, report)
	if e != nil {
		t.Fatal(e)
	}
	if h.LastSeenAt == nil || *h.FreeDiskBytes != 42 || len(h.RunningJobIDs) != 1 {
		t.Fatal(h)
	}
	events, e := s.Events(f.ctx, owner, h.ID, 0, 100)
	if e != nil || len(events) != 0 {
		t.Fatal(events, e)
	}
	if _, e = s.Revoke(f.ctx, owner, h.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Heartbeat(f.ctx, app.AuthenticatedBuildHost{ID: h.ID}, report); !errors.Is(e, auth.ErrUnauthorized) {
		t.Fatal(e)
	}
}

func TestEnrollmentConcurrencyAndRollback(t *testing.T) {
	f := newFixture(t)
	if _, e := f.store.SQL().Exec(`INSERT INTO users(id,name,email,created_at) VALUES(1,'Owner','o@example.com','2026-01-01T00:00:00.000Z')`); e != nil {
		t.Fatal(e)
	}
	notify := &hostNotifier{}
	var logs bytes.Buffer
	s := app.NewBuildHostService(f.store, f.clock, slog.New(slog.NewTextHandler(&logs, nil)), notify)
	code, e := s.CreatePairingCode(f.ctx, owner)
	if e != nil {
		t.Fatal(e)
	}
	input := func(code string, seed byte) app.BuildHostPairInput {
		data := make([]byte, 32)
		data[0] = seed
		key := ed25519.NewKeyFromSeed(data)
		return app.BuildHostPairInput{Code: code, Name: "Builder", PublicKey: base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey)), Facts: []byte(machineFacts)}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); _, e := s.Pair(f.ctx, input(code.Code.Reveal(), byte(i))); errs <- e }(i)
	}
	wg.Wait()
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		} else if !errors.Is(e, auth.ErrUnauthorized) {
			t.Fatal(e)
		}
	}
	if success != 1 || notify.n != 1 {
		t.Fatal(success, notify.n)
	}
	hosts, e := s.List(f.ctx, owner)
	if e != nil || len(hosts) != 1 {
		t.Fatal(hosts, e)
	}
	code2, e := s.CreatePairingCode(f.ctx, owner)
	if e != nil {
		t.Fatal(e)
	}
	duplicate := input(code2.Code.Reveal(), 90)
	duplicate.PublicKey = hosts[0].PublicKey
	if _, e = s.Pair(f.ctx, duplicate); domain.KindOf(e) != domain.KindConflict {
		t.Fatal(e)
	}
	malformed := input(code2.Code.Reveal(), 90)
	malformed.Facts = []byte(`{}`)
	if _, e = s.Pair(f.ctx, malformed); domain.KindOf(e) != domain.KindInvalid {
		t.Fatal(e)
	}
	if _, e = s.Pair(f.ctx, input(code2.Code.Reveal(), 90)); e != nil {
		t.Fatal("code consumed on rejected metadata", e)
	}
	code3, e := s.CreatePairingCode(f.ctx, owner)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.SQL().Exec(`CREATE TRIGGER refuse_pair BEFORE UPDATE ON build_host_pairing_codes BEGIN SELECT RAISE(ABORT,'injected failure'); END`); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Pair(f.ctx, input(code3.Code.Reveal(), 91)); e == nil {
		t.Fatal("injected failure ignored")
	}
	hosts, e = s.List(f.ctx, owner)
	if e != nil || len(hosts) != 2 || notify.n != 2 {
		t.Fatal(hosts, e, notify.n)
	}
	if _, e = f.store.SQL().Exec(`DROP TRIGGER refuse_pair`); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Pair(f.ctx, input(code3.Code.Reveal(), 91)); e != nil {
		t.Fatal(e)
	}
	for _, secret := range []string{code.Code.Reveal(), code2.Code.Reveal(), hosts[0].PublicKey, "transport failure", "builder"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("unsafe notification log", logs.String())
		}
	}
	code4, e := s.CreatePairingCode(f.ctx, owner)
	if e != nil {
		t.Fatal(e)
	}
	f.clock.Advance(15 * time.Minute)
	for _, c := range []string{code4.Code.Reveal(), "unknown", strings.ToLower(code4.Code.Reveal()), " " + code4.Code.Reveal()} {
		if _, e = s.Pair(f.ctx, input(c, 92)); !errors.Is(e, auth.ErrUnauthorized) {
			t.Fatal(c, e)
		}
	}
}

func TestHeartbeatHistoryAndManagementIsolation(t *testing.T) {
	f := newFixture(t)
	if _, e := f.store.SQL().Exec(`INSERT INTO users(id,name,email,created_at) VALUES(1,'Owner','o@example.com','2026-01-01T00:00:00.000Z')`); e != nil {
		t.Fatal(e)
	}
	s := app.NewBuildHostService(f.store, f.clock, nil, nil)
	code, e := s.CreatePairingCode(f.ctx, owner)
	if e != nil {
		t.Fatal(e)
	}
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	h, e := s.Pair(f.ctx, app.BuildHostPairInput{Code: code.Code.Reveal(), Name: "Builder", PublicKey: base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey)), Facts: []byte(machineFacts)})
	if e != nil {
		t.Fatal(e)
	}
	who := app.AuthenticatedBuildHost{ID: h.ID}
	authenticate := func(allow bool, body []byte) error {
		ts := strconv.FormatInt(f.clock.Now().Unix(), 10)
		sig := base64.StdEncoding.EncodeToString(ed25519.Sign(key, auth.HostSignatureMessage("GET", "/api/v1/build-host/status", ts, body)))
		_, e := s.AuthenticateHost(f.ctx, h.ID, "GET", "/api/v1/build-host/status", ts, sig, body, allow)
		return e
	}
	if e = authenticate(true, nil); e != nil {
		t.Fatal(e)
	}
	if e = authenticate(false, nil); !errors.Is(e, auth.ErrUnauthorized) {
		t.Fatal(e)
	}
	h, e = s.Approve(f.ctx, owner, h.ID, domain.BuildHostEdit{})
	if e != nil {
		t.Fatal(e)
	}
	disabled := false
	caps := []string{"custom"}
	h, e = s.Update(f.ctx, owner, h.ID, domain.BuildHostEdit{Enabled: &disabled, Capabilities: &caps})
	if e != nil {
		t.Fatal(e)
	}
	if e = authenticate(false, nil); e != nil {
		t.Fatal(e)
	}
	report := domain.BuildHostReport{Facts: []byte(strings.Replace(machineFacts, `"cmake":"3"`, `"cmake":"4","ninja":"1"`, 1)), RunningJobIDs: []int64{4}, FreeDiskBytes: 99}
	if _, e = f.store.SQL().Exec(`CREATE TRIGGER refuse_report BEFORE UPDATE OF facts ON build_hosts BEGIN SELECT RAISE(ABORT,'injected failure'); END`); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Heartbeat(f.ctx, who, report); e == nil {
		t.Fatal("injected report failure ignored")
	}
	events, e := s.Events(f.ctx, owner, h.ID, 0, 100)
	if e != nil || len(events) != 0 {
		t.Fatal(events, e)
	}
	stored, e := s.Get(f.ctx, owner, h.ID)
	if e != nil || stored.LastSeenAt != nil || string(stored.Facts) != machineFacts {
		t.Fatal(stored, e)
	}
	if _, e = f.store.SQL().Exec(`DROP TRIGGER refuse_report`); e != nil {
		t.Fatal(e)
	}
	f.clock.Advance(time.Second)
	h, e = s.Heartbeat(f.ctx, who, report)
	if e != nil {
		t.Fatal(e)
	}
	if h.Enabled || len(h.Capabilities) != 1 || h.Capabilities[0] != "custom" || !h.Online(f.clock.Now()) {
		t.Fatal(h)
	}
	events, e = s.Events(f.ctx, owner, h.ID, 0, 1)
	if e != nil || len(events) != 1 {
		t.Fatal(events, e)
	}
	first := events[0].ID
	report.Facts = []byte(strings.Replace(string(report.Facts), `"cmake":"4","ninja":"1"`, `"ninja":"1","cmake":"4"`, 1))
	if _, e = s.Heartbeat(f.ctx, who, report); e != nil {
		t.Fatal(e)
	}
	events, e = s.Events(f.ctx, owner, h.ID, 0, 100)
	if e != nil || len(events) != 1 {
		t.Fatal(events, e)
	}
	report.Facts = []byte(machineFacts)
	if _, e = s.Heartbeat(f.ctx, who, report); e != nil {
		t.Fatal(e)
	}
	events, e = s.Events(f.ctx, owner, h.ID, first, 100)
	if e != nil || len(events) != 1 || events[0].ID <= first {
		t.Fatal(events, e)
	}
	if _, e = s.Events(f.ctx, agent, h.ID, 0, 100); domain.KindOf(e) != domain.KindForbidden {
		t.Fatal(e)
	}
	f.clock.Advance(3 * time.Minute)
	h, e = s.Get(f.ctx, owner, h.ID)
	if e != nil || h.Online(f.clock.Now()) {
		t.Fatal(h, e)
	}
}

func TestPairInputDoesNotFormatCredentials(t *testing.T) {
	input := app.BuildHostPairInput{Code: "sensitive-code", PublicKey: "public-key", Facts: []byte(`{}`)}
	raw, e := json.Marshal(input)
	if e != nil {
		t.Fatal(e)
	}
	for _, rendered := range []string{fmt.Sprintf("%+v", input), fmt.Sprintf("%#v", input), fmt.Sprintf("%s", input), string(raw)} {
		if strings.Contains(rendered, input.Code) {
			t.Fatal("pair input exposed credential")
		}
	}
}
