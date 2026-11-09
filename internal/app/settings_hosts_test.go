package app_test

import (
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func settingsHost(t *testing.T, f *fixture, os domain.BuildHostOS, status domain.BuildHostStatus, enabled bool, capacity int) domain.BuildHost {
	t.Helper()
	hosts := []domain.BuildHost{}
	if err := f.store.Read(f.ctx, func(r app.Repos) error { var err error; hosts, err = r.BuildHosts().List(f.ctx); return err }); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	key[0] = byte(len(hosts) + 1)
	h, err := domain.NewBuildHost(domain.BuildHostDraft{Name: fmt.Sprintf("Builder %d", len(hosts)+1), OS: os, Arch: "amd64", PublicKey: base64.StdEncoding.EncodeToString(key), MaxParallel: &capacity}, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.Write(f.ctx, func(r app.Repos) error {
		if err := r.BuildHosts().Insert(f.ctx, &h); err != nil {
			return err
		}
		if status == domain.BuildHostApproved {
			if err := h.Approve(1, f.clock.Now(), domain.BuildHostEdit{}); err != nil {
				return err
			}
			if err := r.BuildHosts().Approve(f.ctx, h); err != nil {
				return err
			}
			if err := h.ApplyEdit(domain.BuildHostEdit{Enabled: &enabled}); err != nil {
				return err
			}
			return r.BuildHosts().Update(f.ctx, h)
		}
		if status == domain.BuildHostRevoked {
			h.Revoke(f.clock.Now())
			return r.BuildHosts().Revoke(f.ctx, h.ID, f.clock.Now())
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return h
}
func settingsFixture(t *testing.T) *fixture {
	f := newFixture(t)
	if _, err := f.store.SQL().Exec(`INSERT INTO users(id,name,email,created_at) VALUES(1,'Owner','owner@example.com','2026-01-01T00:00:00.000Z')`); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestWindowsSettingsRegistryProjection(t *testing.T) {
	for _, tc := range []struct {
		name    string
		os      domain.BuildHostOS
		status  domain.BuildHostStatus
		enabled bool
		want    int
	}{
		{"pending", "windows", domain.BuildHostPending, true, 0}, {"revoked", "windows", domain.BuildHostRevoked, true, 0},
		{"linux", "linux", domain.BuildHostApproved, true, 0}, {"macos", "macos", domain.BuildHostApproved, true, 0},
		{"disabled", "windows", domain.BuildHostApproved, false, 0}, {"enabled offline", "windows", domain.BuildHostApproved, true, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := settingsFixture(t)
			settingsHost(t, f, tc.os, tc.status, tc.enabled, 8)
			got, err := f.board.Settings(f.ctx)
			if err != nil || got.WindowsParallelBuilds != tc.want || got.WindowsRunnerEnabled != (tc.want > 0) {
				t.Fatal(got, err)
			}
		})
	}
}
func TestWindowsSettingsDistributeAndRollback(t *testing.T) {
	f := settingsFixture(t)
	first := settingsHost(t, f, "windows", domain.BuildHostApproved, true, 2)
	second := settingsHost(t, f, "windows", domain.BuildHostApproved, false, 4)
	ignored := settingsHost(t, f, "linux", domain.BuildHostApproved, true, 8)
	for _, tc := range []struct {
		total          int
		enabled        *bool
		want, one, two int
	}{{5, nil, 3, 3, 2}, {16, boolPointer(true), 16, 8, 8}, {3, boolPointer(false), 0, 2, 1}, {2, boolPointer(true), 2, 1, 1}} {
		got, err := f.board.UpdateSettings(f.ctx, owner, domain.SettingsEdit{WindowsParallelBuilds: &tc.total, WindowsRunnerEnabled: tc.enabled})
		if err != nil || got.WindowsParallelBuilds != tc.want {
			t.Fatal(got, err)
		}
		s := app.NewBuildHostService(f.store, f.clock, nil, nil)
		a, _ := s.Get(f.ctx, owner, first.ID)
		b, _ := s.Get(f.ctx, owner, second.ID)
		c, _ := s.Get(f.ctx, owner, ignored.ID)
		if a.MaxParallel != tc.one || b.MaxParallel != tc.two || c.MaxParallel != 8 {
			t.Fatal(a, b, c)
		}
	}
	for _, bad := range []int{-1, 0, 1, 17} {
		_, err := f.board.UpdateSettings(f.ctx, owner, domain.SettingsEdit{WindowsParallelBuilds: &bad})
		requireErrorKind(t, err, domain.KindInvalid)
	}
	for _, trigger := range []string{`CREATE TRIGGER fail_settings BEFORE INSERT ON board_settings BEGIN SELECT RAISE(ABORT,'refused'); END`, `CREATE TRIGGER fail_settings BEFORE UPDATE ON build_hosts WHEN NEW.id=2 BEGIN SELECT RAISE(ABORT,'refused'); END`} {
		if _, err := f.store.SQL().Exec(trigger); err != nil {
			t.Fatal(err)
		}
		total, workers := 10, 5
		_, err := f.board.UpdateSettings(f.ctx, owner, domain.SettingsEdit{WindowsParallelBuilds: &total, CodexWorkers: &workers})
		if err == nil {
			t.Fatal("expected rollback")
		}
		got, err := f.board.Settings(f.ctx)
		if err != nil || got.WindowsParallelBuilds != 2 || got.CodexWorkers != 3 {
			t.Fatal(got, err)
		}
		if _, err := f.store.SQL().Exec(`DROP TRIGGER fail_settings`); err != nil {
			t.Fatal(err)
		}
	}
}
func boolPointer(v bool) *bool { return &v }
func intPointer(v int) *int    { return &v }
func TestWindowsSettingsWithoutHosts(t *testing.T) {
	f := settingsFixture(t)
	total := 1
	for _, edit := range []domain.SettingsEdit{{WindowsRunnerEnabled: boolPointer(true)}, {WindowsParallelBuilds: &total}} {
		_, err := f.board.UpdateSettings(f.ctx, owner, edit)
		requireErrorKind(t, err, domain.KindConflict)
	}
	if got, err := f.board.UpdateSettings(f.ctx, owner, domain.SettingsEdit{WindowsRunnerEnabled: boolPointer(false)}); err != nil || got.WindowsRunnerEnabled || got.WindowsParallelBuilds != 0 {
		t.Fatal(got, err)
	}
}

func seedLegacyWindows(t *testing.T, f *fixture, enabled, parallel string) {
	t.Helper()
	for key, value := range map[string]string{"windows_runner_enabled": enabled, "windows_parallel_builds": parallel} {
		if _, err := f.store.SQL().Exec(`INSERT INTO board_settings(key,value,updated_by,updated_at) VALUES(?,?,'Owner','2026-01-01T00:00:00.000Z')`, key, value); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWindowsLegacySettingsApplyUntilAHostIsApproved(t *testing.T) {
	f := settingsFixture(t)
	seedLegacyWindows(t, f, "true", "2")
	workers := 4
	got, err := f.board.UpdateSettings(f.ctx, owner, domain.SettingsEdit{CodexWorkers: &workers})
	if err != nil || !got.WindowsRunnerEnabled || got.WindowsParallelBuilds != 2 || got.WindowsParallelBuildLimit != 0 {
		t.Fatal(got, err)
	}
	if got, err := f.board.Settings(f.ctx); err != nil || !got.WindowsRunnerEnabled || got.WindowsParallelBuilds != 2 {
		t.Fatal(got, err)
	}
	for key, want := range map[string]string{"windows_runner_enabled": "true", "windows_parallel_builds": "2"} {
		var value string
		if err := f.store.SQL().QueryRow(`SELECT value FROM board_settings WHERE key=?`, key).Scan(&value); err != nil || value != want {
			t.Fatal(key, value, err)
		}
	}
	// Once a Windows host is approved, hosts are authoritative and the legacy values no longer count.
	settingsHost(t, f, "windows", domain.BuildHostApproved, false, 3)
	if got, err := f.board.Settings(f.ctx); err != nil || got.WindowsRunnerEnabled || got.WindowsParallelBuilds != 0 || got.WindowsParallelBuildLimit != 8 {
		t.Fatal(got, err)
	}
}

func TestWindowsLegacySwitchTurnsOffAndBackOnUntilAHostIsApproved(t *testing.T) {
	f := settingsFixture(t)
	seedLegacyWindows(t, f, "true", "1")
	got, err := f.board.UpdateSettings(f.ctx, owner, domain.SettingsEdit{WindowsRunnerEnabled: boolPointer(false)})
	if err != nil || got.WindowsRunnerEnabled || got.WindowsParallelBuilds != 0 {
		t.Fatal(got, err)
	}
	if got, err := f.board.Settings(f.ctx); err != nil || got.WindowsRunnerEnabled {
		t.Fatal(got, err)
	}
	got, err = f.board.UpdateSettings(f.ctx, owner, domain.SettingsEdit{WindowsRunnerEnabled: boolPointer(true)})
	if err != nil || !got.WindowsRunnerEnabled || got.WindowsParallelBuilds != 1 {
		t.Fatal(got, err)
	}
	if got, err := f.board.Settings(f.ctx); err != nil || !got.WindowsRunnerEnabled || got.WindowsParallelBuilds != 1 {
		t.Fatal(got, err)
	}
	if _, err := f.board.UpdateSettings(f.ctx, owner, domain.SettingsEdit{WindowsParallelBuilds: intPointer(2)}); err == nil {
		t.Fatal("changing legacy capacity without an approved host must stay refused")
	}
}

func TestWindowsCannotBeEnabledWithoutLegacyCapacityOrHost(t *testing.T) {
	f := settingsFixture(t)
	seedLegacyWindows(t, f, "false", "0")
	if _, err := f.board.UpdateSettings(f.ctx, owner, domain.SettingsEdit{WindowsRunnerEnabled: boolPointer(true)}); err == nil {
		t.Fatal("enabling with no stored capacity and no approved host must be refused")
	}
}

func TestWindowsLegacySettingsIgnoreInvalidValues(t *testing.T) {
	f := settingsFixture(t)
	seedLegacyWindows(t, f, "true", "99")
	if got, err := f.board.Settings(f.ctx); err != nil || got.WindowsRunnerEnabled || got.WindowsParallelBuilds != 0 {
		t.Fatal(got, err)
	}
}
