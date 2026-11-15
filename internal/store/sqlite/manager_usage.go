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
