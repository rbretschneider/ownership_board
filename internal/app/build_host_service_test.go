package app_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestBuildHostService(t *testing.T) {
	f := newFixture(t)
	var logs bytes.Buffer
	s := app.NewBuildHostService(f.store, f.clock, slog.New(slog.NewTextHandler(&logs, nil)))
	if _, err := f.store.SQL().Exec(`INSERT INTO users (id,name,email,created_at) VALUES (1,'Owner','owner@example.com','2026-01-01T00:00:00.000Z')`); err != nil {
		t.Fatal(err)
	}
	for _, actor := range []domain.Actor{agent, domain.SystemActor, {}, {Kind: domain.ActorHuman}} {
		_, err := s.List(f.ctx, actor)
		requireErrorKind(t, err, domain.KindForbidden)
		_, err = s.Get(f.ctx, actor, 1)
		requireErrorKind(t, err, domain.KindForbidden)
		_, err = s.CreatePairingCode(f.ctx, actor)
		requireErrorKind(t, err, domain.KindForbidden)
		_, err = s.Approve(f.ctx, actor, 1, domain.BuildHostEdit{})
		requireErrorKind(t, err, domain.KindForbidden)
		_, err = s.Update(f.ctx, actor, 1, domain.BuildHostEdit{})
		requireErrorKind(t, err, domain.KindForbidden)
		_, err = s.Revoke(f.ctx, actor, 1)
		requireErrorKind(t, err, domain.KindForbidden)
	}
	if logs.Len() != 0 {
		t.Fatal(logs.String())
	}
	code, err := s.CreatePairingCode(f.ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(code.Code.Reveal()) != 26 || !code.ExpiresAt.Equal(f.clock.Now().Add(15*time.Minute)) {
		t.Fatal("invalid issued code")
	}
	for _, r := range code.Code.Reveal() {
		if !strings.ContainsRune("23456789ABCDEFGHJKLMNPQRSTUVWXYZ", r) {
			t.Fatal("ambiguous alphabet")
		}
	}
	encoded, err := json.Marshal(code)
	if err != nil {
		t.Fatal(err)
	}
	for _, rendered := range []string{string(encoded), fmt.Sprintf("%+v", code), fmt.Sprintf("%#v", code)} {
		if strings.Contains(rendered, code.Code.Reveal()) {
			t.Fatal("issuance debug formatting exposed secret")
		}
	}
	var hash, created, expiry string
	var used, host any
	if err := f.store.SQL().QueryRow(`SELECT code_hash,created_at,expires_at,used_at,used_by_host FROM build_host_pairing_codes`).Scan(&hash, &created, &expiry, &used, &host); err != nil {
		t.Fatal(err)
	}
	if created != f.clock.Now().Format("2006-01-02T15:04:05.000Z") || expiry != code.ExpiresAt.Format("2006-01-02T15:04:05.000Z") {
		t.Fatal("stored expiry differs from response")
	}
	if hash != fmt.Sprintf("%x", sha256.Sum256([]byte(code.Code.Reveal()))) || used != nil || host != nil {
		t.Fatal("incorrect persisted code")
	}
	if strings.Contains(logs.String(), code.Code.Reveal()) || strings.Contains(logs.String(), hash) {
		t.Fatal("secret in audit")
	}
	if !strings.Contains(logs.String(), "build_host.pairing_code_issued") {
		t.Fatal("missing audit")
	}
	h, err := domain.NewBuildHost(domain.BuildHostDraft{Name: "Builder", OS: "windows", Arch: "amd64", PublicKey: base64.StdEncoding.EncodeToString(make([]byte, 32))}, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.Write(f.ctx, func(r app.Repos) error { return r.BuildHosts().Insert(f.ctx, &h) }); err != nil {
		t.Fatal(err)
	}
	list, err := s.List(f.ctx, owner)
	if err != nil || len(list) != 1 {
		t.Fatal(list, err)
	}
	logs.Reset()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.Approve(f.ctx, owner, h.ID, domain.BuildHostEdit{}); errs <- err }()
	}
	wg.Wait()
	close(errs)
	successes, conflicts := 0, 0
	for err := range errs {
		if err == nil {
			successes++
		} else if domain.KindOf(err) == domain.KindConflict {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 || strings.Count(logs.String(), "build_host.approved") != 1 {
		t.Fatal("approval concurrency", logs.String())
	}
	got, err := s.Get(f.ctx, owner, h.ID)
	if err != nil || *got.ApprovedBy != owner.ID || !got.ApprovedAt.Equal(f.clock.Now()) {
		t.Fatal(got, err)
	}
	logs.Reset()
	if _, err := s.Update(f.ctx, owner, h.ID, domain.BuildHostEdit{}); err != nil {
		t.Fatal(err)
	}
	if logs.Len() != 0 {
		t.Fatal("no-op audited")
	}
	caps := []string{"gpu", "msvc"}
	disabled := false
	n := 8
	if _, err := s.Update(f.ctx, owner, h.ID, domain.BuildHostEdit{Capabilities: &caps, Enabled: &disabled, MaxParallel: &n}); err != nil {
		t.Fatal(err)
	}
	if strings.Count(logs.String(), "build_host.updated") != 1 {
		t.Fatal(logs.String())
	}
	if _, err := s.Revoke(f.ctx, owner, h.ID); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Hour)
	logs.Reset()
	got, err = s.Revoke(f.ctx, owner, h.ID)
	if err != nil || logs.Len() != 0 || got.Enabled {
		t.Fatal(got, err, logs.String())
	}
	enabled := true
	_, err = s.Update(f.ctx, owner, h.ID, domain.BuildHostEdit{Enabled: &enabled})
	requireErrorKind(t, err, domain.KindConflict)
	_, err = s.Approve(f.ctx, owner, h.ID, domain.BuildHostEdit{})
	requireErrorKind(t, err, domain.KindConflict)
	_, err = s.Get(f.ctx, owner, 999)
	requireErrorKind(t, err, domain.KindNotFound)
}

func TestBuildHostFailedCommitHasNoAuditOrCode(t *testing.T) {
	f := newFixture(t)
	var logs bytes.Buffer
	s := app.NewBuildHostService(f.store, f.clock, slog.New(slog.NewTextHandler(&logs, nil)))
	// The actor does not exist, so the foreign key prevents persistence.
	code, err := s.CreatePairingCode(f.ctx, owner)
	if err == nil || code.Code.Reveal() != "" || logs.Len() != 0 {
		t.Fatal("failed issuance leaked success")
	}
	var n int
	if err := f.store.SQL().QueryRow(`SELECT count(*) FROM build_host_pairing_codes`).Scan(&n); err != nil || n != 0 {
		t.Fatal(n, err)
	}
}

func TestBuildHostMutationRollbackAndSafeAudit(t *testing.T) {
	f := newFixture(t)
	var logs bytes.Buffer
	s := app.NewBuildHostService(f.store, f.clock, slog.New(slog.NewTextHandler(&logs, nil)))
	if _, err := f.store.SQL().Exec(`INSERT INTO users (id,name,email,created_at) VALUES (1,'Owner','owner@example.com','2026-01-01T00:00:00.000Z')`); err != nil {
		t.Fatal(err)
	}
	h, err := domain.NewBuildHost(domain.BuildHostDraft{Name: "Builder", OS: "macos", Arch: "arm64", PublicKey: base64.StdEncoding.EncodeToString(make([]byte, 32)), Facts: []byte(`{"private_report":"sensitive_fact"}`)}, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.Write(f.ctx, func(r app.Repos) error { return r.BuildHosts().Insert(f.ctx, &h) }); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []func() error{
		func() error { _, err := s.Approve(f.ctx, owner, h.ID, domain.BuildHostEdit{}); return err },
		func() error {
			name := "Changed"
			_, err := s.Update(f.ctx, owner, h.ID, domain.BuildHostEdit{Name: &name})
			return err
		},
		func() error { _, err := s.Revoke(f.ctx, owner, h.ID); return err },
	} {
		if _, err := f.store.SQL().Exec(`CREATE TRIGGER fail_host_update BEFORE UPDATE ON build_hosts BEGIN SELECT RAISE(ABORT,'write refused'); END`); err != nil {
			t.Fatal(err)
		}
		if err := operation(); err == nil {
			t.Fatal("expected write failure")
		}
		if logs.Len() != 0 {
			t.Fatal("failed mutation audited", logs.String())
		}
		got, err := s.Get(f.ctx, owner, h.ID)
		if err != nil || got.Status != domain.BuildHostPending || got.Name != "Builder" {
			t.Fatal("rollback failed", got, err)
		}
		if _, err := f.store.SQL().Exec(`DROP TRIGGER fail_host_update`); err != nil {
			t.Fatal(err)
		}
	}
	n := 0
	_, err = s.Approve(f.ctx, owner, h.ID, domain.BuildHostEdit{MaxParallel: &n})
	requireErrorKind(t, err, domain.KindInvalid)
	_, err = s.Revoke(f.ctx, owner, 999)
	requireErrorKind(t, err, domain.KindNotFound)
	if logs.Len() != 0 {
		t.Fatal("invalid mutation audited")
	}
	if _, err := s.Approve(f.ctx, owner, h.ID, domain.BuildHostEdit{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs.String(), h.PublicKey) || strings.Contains(logs.String(), "sensitive_fact") || !strings.Contains(logs.String(), "actor_id=1") || !strings.Contains(logs.String(), "host_id=1") {
		t.Fatal("unsafe or incomplete audit", logs.String())
	}
}
