package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

type settingsRepo struct {
	q querier
}

func (r settingsRepo) Get(ctx context.Context) (domain.BoardSettings, error) {
	settings := domain.DefaultBoardSettings()
	var raw string
	err := r.q.QueryRowContext(ctx, `SELECT value FROM board_settings WHERE key = ?`, domain.SettingAutonomousInFlightCap).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.BoardSettings{}, fmt.Errorf("sqlite: read settings: %w", err)
	}
	if err == nil {
		settings.AutonomousInFlightCap, _ = domain.ParseInFlightCap(raw)
	}
	err = r.q.QueryRowContext(ctx, `SELECT value FROM board_settings WHERE key = ?`, domain.SettingCodexWorkers).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.BoardSettings{}, fmt.Errorf("sqlite: read worker settings: %w", err)
	}
	if err == nil {
		if count, parseErr := strconv.Atoi(raw); parseErr == nil && count >= 0 && count <= domain.MaxCodexWorkers {
			settings.CodexWorkers = count
		}
	}
	err = r.q.QueryRowContext(ctx, `SELECT value FROM board_settings WHERE key = ?`, domain.SettingWindowsRunnerEnabled).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.BoardSettings{}, fmt.Errorf("sqlite: read Windows runner setting: %w", err)
	}
	if err == nil {
		settings.WindowsRunnerEnabled, _ = strconv.ParseBool(raw)
	}
	return settings, nil
}

func (r settingsRepo) Save(ctx context.Context, settings domain.BoardSettings, actor domain.Actor, now time.Time) error {
	if _, err := r.q.ExecContext(ctx,
		`INSERT INTO board_settings (key, value, updated_by, updated_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_by = excluded.updated_by, updated_at = excluded.updated_at`,
		domain.SettingWindowsRunnerEnabled, strconv.FormatBool(settings.WindowsRunnerEnabled), actor.Name, formatTime(now)); err != nil {
		return fmt.Errorf("sqlite: save Windows runner setting: %w", err)
	}
	if _, err := r.q.ExecContext(ctx,
		`INSERT INTO board_settings (key, value, updated_by, updated_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_by = excluded.updated_by, updated_at = excluded.updated_at`,
		domain.SettingCodexWorkers, strconv.Itoa(settings.CodexWorkers), actor.Name, formatTime(now)); err != nil {
		return fmt.Errorf("sqlite: save worker settings: %w", err)
	}
	_, err := r.q.ExecContext(ctx,
		`INSERT INTO board_settings (key, value, updated_by, updated_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_by = excluded.updated_by, updated_at = excluded.updated_at`,
		domain.SettingAutonomousInFlightCap, strconv.Itoa(settings.AutonomousInFlightCap), actor.Name, formatTime(now))
	if err != nil {
		return fmt.Errorf("sqlite: save settings: %w", err)
	}
	return nil
}
