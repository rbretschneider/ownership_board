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
	for _, field := range []struct {
		key    string
		target *int
		max    int
	}{
		{domain.SettingClaudeWorkers, &settings.ClaudeWorkers, domain.MaxClaudeWorkers},
		{domain.SettingCodexWeeklyLimit, &settings.CodexWeeklyLimit, 100},
		{domain.SettingClaudeWeeklyLimit, &settings.ClaudeWeeklyLimit, 100},
	} {
		err = r.q.QueryRowContext(ctx, `SELECT value FROM board_settings WHERE key = ?`, field.key).Scan(&raw)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return domain.BoardSettings{}, fmt.Errorf("sqlite: read account settings: %w", err)
		}
		if err == nil {
			if n, e := strconv.Atoi(raw); e == nil && n >= 0 && n <= field.max {
				*field.target = n
			}
		}
	}
	// Review settings: an unreadable stored value keeps the default.
	err = r.q.QueryRowContext(ctx, `SELECT value FROM board_settings WHERE key = ?`, domain.SettingReviewBy).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.BoardSettings{}, fmt.Errorf("sqlite: read review settings: %w", err)
	}
	if err == nil {
		if value, parseErr := domain.ParseReviewBy(raw); parseErr == nil {
			settings.ReviewBy = value
		}
	}
	for _, field := range []struct {
		key    string
		target *bool
	}{
		{domain.SettingReviewFreshContext, &settings.ReviewFreshContext},
		{domain.SettingReviewAdversarial, &settings.ReviewAdversarial},
	} {
		err = r.q.QueryRowContext(ctx, `SELECT value FROM board_settings WHERE key = ?`, field.key).Scan(&raw)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return domain.BoardSettings{}, fmt.Errorf("sqlite: read review settings: %w", err)
		}
		if err == nil {
			if value, parseErr := strconv.ParseBool(raw); parseErr == nil {
				*field.target = value
			}
		}
	}
	// Pre-build-host Windows values are read but never written here; the
	// application decides whether they apply.
	err = r.q.QueryRowContext(ctx, `SELECT value FROM board_settings WHERE key = ?`, domain.SettingWindowsRunnerEnabled).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.BoardSettings{}, fmt.Errorf("sqlite: read legacy Windows setting: %w", err)
	}
	if err == nil {
		settings.LegacyWindowsRunnerEnabled, _ = strconv.ParseBool(raw)
	}
	err = r.q.QueryRowContext(ctx, `SELECT value FROM board_settings WHERE key = ?`, domain.SettingWindowsParallelBuilds).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.BoardSettings{}, fmt.Errorf("sqlite: read legacy Windows setting: %w", err)
	}
	if err == nil {
		if n, e := strconv.Atoi(raw); e == nil && n >= 1 && n <= domain.MaxLegacyWindowsParallelBuilds {
			settings.LegacyWindowsParallelBuilds = n
		}
	}
	return settings, nil
}

func (r settingsRepo) Save(ctx context.Context, settings domain.BoardSettings, actor domain.Actor, now time.Time) error {
	if _, err := r.q.ExecContext(ctx,
		`INSERT INTO board_settings (key, value, updated_by, updated_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_by = excluded.updated_by, updated_at = excluded.updated_at`,
		domain.SettingCodexWorkers, strconv.Itoa(settings.CodexWorkers), actor.Name, formatTime(now)); err != nil {
		return fmt.Errorf("sqlite: save worker settings: %w", err)
	}
	for key, value := range map[string]int{domain.SettingClaudeWorkers: settings.ClaudeWorkers, domain.SettingCodexWeeklyLimit: settings.CodexWeeklyLimit, domain.SettingClaudeWeeklyLimit: settings.ClaudeWeeklyLimit} {
		if _, err := r.q.ExecContext(ctx, `INSERT INTO board_settings (key,value,updated_by,updated_at) VALUES (?,?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_by=excluded.updated_by,updated_at=excluded.updated_at`, key, strconv.Itoa(value), actor.Name, formatTime(now)); err != nil {
			return fmt.Errorf("sqlite: save account settings: %w", err)
		}
	}
	for key, value := range map[string]string{
		domain.SettingReviewBy:           string(settings.ReviewBy),
		domain.SettingReviewFreshContext: strconv.FormatBool(settings.ReviewFreshContext),
		domain.SettingReviewAdversarial:  strconv.FormatBool(settings.ReviewAdversarial),
	} {
		if _, err := r.q.ExecContext(ctx, `INSERT INTO board_settings (key,value,updated_by,updated_at) VALUES (?,?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_by=excluded.updated_by,updated_at=excluded.updated_at`, key, value, actor.Name, formatTime(now)); err != nil {
			return fmt.Errorf("sqlite: save review settings: %w", err)
		}
	}
	// The legacy Windows switch is only updated where it already exists: a
	// board that never had the pre-build-host switch gets Windows capacity
	// from approved build hosts only.
	if _, err := r.q.ExecContext(ctx,
		`UPDATE board_settings SET value = ?, updated_by = ?, updated_at = ? WHERE key = ? AND value <> ?`,
		strconv.FormatBool(settings.LegacyWindowsRunnerEnabled), actor.Name, formatTime(now), domain.SettingWindowsRunnerEnabled, strconv.FormatBool(settings.LegacyWindowsRunnerEnabled)); err != nil {
		return fmt.Errorf("sqlite: save legacy Windows setting: %w", err)
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
