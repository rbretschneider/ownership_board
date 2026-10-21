package sqlite

import (
	"context"
	"fmt"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

type sourceStatusRepo struct {
	q querier
}

func (r sourceStatusRepo) All(ctx context.Context) (map[domain.Provider]app.SyncOutcome, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT provider, last_synced_at, last_error FROM repo_source_status`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: source statuses: %w", err)
	}
	defer rows.Close()
	outcomes := map[domain.Provider]app.SyncOutcome{}
	for rows.Next() {
		var provider, at string
		var outcome app.SyncOutcome
		if err := rows.Scan(&provider, &at, &outcome.Error); err != nil {
			return nil, err
		}
		if outcome.At, err = parseTime(at); err != nil {
			return nil, err
		}
		outcomes[domain.Provider(provider)] = outcome
	}
	return outcomes, rows.Err()
}

func (r sourceStatusRepo) Save(ctx context.Context, provider domain.Provider, outcome app.SyncOutcome) error {
	_, err := r.q.ExecContext(ctx,
		`INSERT INTO repo_source_status (provider, last_synced_at, last_error) VALUES (?, ?, ?)
		 ON CONFLICT (provider) DO UPDATE SET last_synced_at = excluded.last_synced_at, last_error = excluded.last_error`,
		string(provider), formatTime(outcome.At), outcome.Error)
	if err != nil {
		return fmt.Errorf("sqlite: save source status %s: %w", provider, err)
	}
	return nil
}
