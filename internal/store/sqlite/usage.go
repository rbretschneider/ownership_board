package sqlite

import (
	"context"
	"fmt"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

type usageRepo struct {
	q querier
}

const usageSums = `COUNT(*), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(cache_write_tokens), 0),
	COALESCE(SUM(cache_read_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(cost_usd), 0)`

func (r usageRepo) Record(ctx context.Context, report domain.UsageReport) (bool, error) {
	result, err := r.q.ExecContext(ctx,
		`INSERT INTO ticket_usage (ticket_id, call_key, provider, model, input_tokens, cache_write_tokens,
		 cache_read_tokens, output_tokens, cost_usd, reported_by, reported_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (call_key) DO NOTHING`,
		report.TicketID, report.CallKey, report.Provider, report.Model, report.InputTokens, report.CacheWriteTokens,
		report.CacheReadTokens, report.OutputTokens, report.CostUSD, report.ReportedBy, formatTime(report.ReportedAt))
	if err != nil {
		return false, fmt.Errorf("sqlite: record usage of ticket %d: %w", report.TicketID, err)
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func (r usageRepo) ForTicket(ctx context.Context, ticketID int64) (domain.TokenUsage, error) {
	var u domain.TokenUsage
	err := r.q.QueryRowContext(ctx, `SELECT `+usageSums+` FROM ticket_usage WHERE ticket_id = ?`, ticketID).
		Scan(&u.Calls, &u.InputTokens, &u.CacheWriteTokens, &u.CacheReadTokens, &u.OutputTokens, &u.CostUSD)
	if err != nil {
		return domain.TokenUsage{}, fmt.Errorf("sqlite: usage of ticket %d: %w", ticketID, err)
	}
	return u, nil
}

func (r usageRepo) Totals(ctx context.Context) (map[int64]domain.TokenUsage, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT ticket_id, `+usageSums+` FROM ticket_usage GROUP BY ticket_id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: usage totals: %w", err)
	}
	defer rows.Close()
	totals := make(map[int64]domain.TokenUsage)
	for rows.Next() {
		var id int64
		var u domain.TokenUsage
		if err := rows.Scan(&id, &u.Calls, &u.InputTokens, &u.CacheWriteTokens, &u.CacheReadTokens, &u.OutputTokens, &u.CostUSD); err != nil {
			return nil, fmt.Errorf("sqlite: scan usage total: %w", err)
		}
		totals[id] = u
	}
	return totals, rows.Err()
}
