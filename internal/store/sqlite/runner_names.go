package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

type runnerNameRepo struct{ q querier }

const runnerNameColumns = `family, position, name, worker, assigned_at`

func scanRunnerName(row scanner) (domain.RunnerName, error) {
	var n domain.RunnerName
	var worker, assigned sql.NullString
	if err := row.Scan(&n.Family, &n.Position, &n.Name, &worker, &assigned); err != nil {
		return n, notFoundOr(err, "no runner name for that worker")
	}
	n.Worker = worker.String
	if !assigned.Valid {
		return n, nil
	}
	var err error
	n.AssignedAt, err = parseTime(assigned.String)
	return n, err
}

func scanRunnerNames(rows *sql.Rows, err error) ([]domain.RunnerName, error) {
	if err != nil {
		return nil, fmt.Errorf("sqlite: read runner names: %w", err)
	}
	defer rows.Close()
	out := []domain.RunnerName{}
	for rows.Next() {
		n, err := scanRunnerName(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Assigned implements app.RunnerNameRepository.
func (r runnerNameRepo) Assigned(ctx context.Context) ([]domain.RunnerName, error) {
	return scanRunnerNames(r.q.QueryContext(ctx,
		`SELECT `+runnerNameColumns+` FROM runner_names WHERE worker IS NOT NULL ORDER BY family, position`))
}

// ForWorker implements app.RunnerNameRepository.
func (r runnerNameRepo) ForWorker(ctx context.Context, workerID string) (domain.RunnerName, error) {
	return scanRunnerName(r.q.QueryRowContext(ctx,
		`SELECT `+runnerNameColumns+` FROM runner_names WHERE worker=?`, workerID))
}

// Pool implements app.RunnerNameRepository.
func (r runnerNameRepo) Pool(ctx context.Context, family string) ([]domain.RunnerName, error) {
	return scanRunnerNames(r.q.QueryContext(ctx,
		`SELECT `+runnerNameColumns+` FROM runner_names WHERE family=? ORDER BY position`, family))
}

// Assign implements app.RunnerNameRepository. The WHERE clause and the unique
// worker index make a concurrent second assignment fail instead of overwrite.
func (r runnerNameRepo) Assign(ctx context.Context, family string, position int, workerID string, at time.Time) error {
	result, err := r.q.ExecContext(ctx,
		`UPDATE runner_names SET worker=?, assigned_at=? WHERE family=? AND position=? AND worker IS NULL`,
		workerID, formatTime(at), family, position)
	if err != nil {
		return conflictOr(err, "worker %s already has a runner name", workerID)
	}
	return requireOneRow(result, "runner name %s/%d is not free", family, position)
}

// SetName implements app.RunnerNameRepository.
func (r runnerNameRepo) SetName(ctx context.Context, family string, position int, name string) error {
	result, err := r.q.ExecContext(ctx, `UPDATE runner_names SET name=? WHERE family=? AND position=?`, name, family, position)
	if err != nil {
		return conflictOr(err, "the %s family already has a runner named %s", family, name)
	}
	return requireOneRow(result, "no runner name %s/%d", family, position)
}

// requireOneRow turns an update that matched no row into a conflict.
func requireOneRow(result sql.Result, format string, args ...any) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: rows affected: %w", err)
	}
	if affected == 0 {
		return domain.Conflictf(format, args...)
	}
	return nil
}
