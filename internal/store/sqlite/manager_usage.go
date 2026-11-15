package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

type managerUsageRepo struct{ q querier }

// Add implements app.ManagerUsageRepository: it adds one session's spend to its UTC day.
func (r managerUsageRepo) Add(ctx context.Context, day string, sessions int, costUSD float64, now time.Time) error {
	_, err := r.q.ExecContext(ctx,
		`INSERT INTO manager_usage (day, sessions, cost_usd, updated_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT (day) DO UPDATE SET sessions = sessions + excluded.sessions, cost_usd = cost_usd + excluded.cost_usd, updated_at = excluded.updated_at`,
		day, sessions, costUSD, formatTime(now))
	if err != nil {
		return fmt.Errorf("sqlite: record manager usage: %w", err)
	}
	return nil
}

// Day implements app.ManagerUsageRepository.
func (r managerUsageRepo) Day(ctx context.Context, day string) (domain.ManagerUsage, error) {
	usage := domain.ManagerUsage{Day: day}
	err := r.q.QueryRowContext(ctx, `SELECT sessions, cost_usd FROM manager_usage WHERE day = ?`, day).Scan(&usage.Sessions, &usage.CostUSD)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.ManagerUsage{}, fmt.Errorf("sqlite: read manager usage: %w", err)
	}
	return usage, nil
}

// AddSession implements app.ManagerUsageRepository.
func (r managerUsageRepo) AddSession(ctx context.Context, session domain.ManagerSession) error {
	_, err := r.q.ExecContext(ctx, `INSERT INTO manager_sessions (at, model, cost_usd, summary) VALUES (?, ?, ?, ?)`,
		formatTime(session.At), session.Model, session.CostUSD, session.Summary)
	if err != nil {
		return fmt.Errorf("sqlite: record manager session: %w", err)
	}
	return nil
}

// RecentSessions implements app.ManagerUsageRepository: newest first.
func (r managerUsageRepo) RecentSessions(ctx context.Context, limit int) ([]domain.ManagerSession, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT at, model, cost_usd, summary FROM manager_sessions ORDER BY at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list manager sessions: %w", err)
	}
	defer rows.Close()
	sessions := []domain.ManagerSession{}
	for rows.Next() {
		var session domain.ManagerSession
		var at string
		if err := rows.Scan(&at, &session.Model, &session.CostUSD, &session.Summary); err != nil {
			return nil, err
		}
		if session.At, err = parseTime(at); err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}
