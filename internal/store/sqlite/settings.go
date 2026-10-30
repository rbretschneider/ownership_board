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
	return settings, nil
}

func (r settingsRepo) Save(ctx context.Context, settings domain.BoardSettings, actor domain.Actor, now time.Time) error {
	_, err := r.q.ExecContext(ctx,
		`INSERT INTO board_settings (key, value, updated_by, updated_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_by = excluded.updated_by, updated_at = excluded.updated_at`,
		domain.SettingAutonomousInFlightCap, strconv.Itoa(settings.AutonomousInFlightCap), actor.Name, formatTime(now))
	if err != nil {
		return fmt.Errorf("sqlite: save settings: %w", err)
	}
	return nil
}
