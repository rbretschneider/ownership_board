package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

type ticketRepo struct {
	q querier
}

const ticketColumns = `id, repo_id, kind, title, body, spec, lane, rank, pr_url, branch,
	lease_holder, lease_expires_at, created_by, created_at, updated_at, lane_changed_at`

func (r ticketRepo) Get(ctx context.Context, id int64) (domain.Ticket, error) {
	row := r.q.QueryRowContext(ctx, `SELECT `+ticketColumns+` FROM tickets WHERE id = ?`, id)
	ticket, err := scanTicket(row)
	return ticket, notFoundOr(err, "ticket %d does not exist", id)
}

func (r ticketRepo) List(ctx context.Context, filter app.TicketFilter) ([]domain.Ticket, error) {
	clause, args := filterClause(filter)
	rows, err := r.q.QueryContext(ctx, `SELECT `+ticketColumns+` FROM tickets WHERE `+clause+` ORDER BY lane, rank`, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list tickets: %w", err)
	}
	defer rows.Close()
	var tickets []domain.Ticket
	for rows.Next() {
		ticket, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		tickets = append(tickets, ticket)
	}
	return tickets, rows.Err()
}

func (r ticketRepo) Insert(ctx context.Context, ticket *domain.Ticket) error {
	result, err := r.q.ExecContext(ctx,
		`INSERT INTO tickets (repo_id, kind, title, body, spec, lane, rank, pr_url, branch,
			lease_holder, lease_expires_at, created_by, created_at, updated_at, lane_changed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ticketValues(*ticket)...)
	if err != nil {
		return fmt.Errorf("sqlite: insert ticket: %w", err)
	}
	ticket.ID, err = result.LastInsertId()
	return err
}

func (r ticketRepo) Update(ctx context.Context, ticket domain.Ticket) error {
	args := append(ticketValues(ticket), ticket.ID)
	_, err := r.q.ExecContext(ctx,
		`UPDATE tickets SET repo_id = ?, kind = ?, title = ?, body = ?, spec = ?, lane = ?, rank = ?,
			pr_url = ?, branch = ?, lease_holder = ?, lease_expires_at = ?, created_by = ?,
			created_at = ?, updated_at = ?, lane_changed_at = ?
		 WHERE id = ?`, args...)
	if err != nil {
		return fmt.Errorf("sqlite: update ticket %d: %w", ticket.ID, err)
	}
	return nil
}

func (r ticketRepo) Delete(ctx context.Context, id int64) error {
	if _, err := r.q.ExecContext(ctx, `DELETE FROM tickets WHERE id = ?`, id); err != nil {
		return fmt.Errorf("sqlite: delete ticket %d: %w", id, err)
	}
	return nil
}

func (r ticketRepo) LaneOrder(ctx context.Context, lane domain.Lane) ([]app.RankedTicket, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT id, rank FROM tickets WHERE lane = ? ORDER BY rank, id`, string(lane))
	if err != nil {
		return nil, fmt.Errorf("sqlite: lane order: %w", err)
	}
	defer rows.Close()
	var order []app.RankedTicket
	for rows.Next() {
		var ranked app.RankedTicket
		if err := rows.Scan(&ranked.ID, &ranked.Rank); err != nil {
			return nil, err
		}
		order = append(order, ranked)
	}
	return order, rows.Err()
}

func (r ticketRepo) SetRank(ctx context.Context, id int64, rank float64) error {
	if _, err := r.q.ExecContext(ctx, `UPDATE tickets SET rank = ? WHERE id = ?`, rank, id); err != nil {
		return fmt.Errorf("sqlite: set rank of ticket %d: %w", id, err)
	}
	return nil
}

func (r ticketRepo) NextClaimable(ctx context.Context, lane domain.Lane, filter app.TicketFilter, now time.Time) (domain.Ticket, bool, error) {
	clause, filterArgs := filterClause(filter)
	args := append([]any{string(lane), formatTime(now)}, filterArgs...)
	row := r.q.QueryRowContext(ctx,
		`SELECT `+ticketColumns+` FROM tickets
		 WHERE lane = ? AND (lease_holder = '' OR lease_expires_at <= ?) AND `+clause+`
		 ORDER BY rank, id LIMIT 1`, args...)
	ticket, err := scanTicket(row)
	if err == sql.ErrNoRows {
		return domain.Ticket{}, false, nil
	}
	return ticket, err == nil, err
}

func ticketValues(t domain.Ticket) []any {
	holder, expires := "", ""
	if t.Lease != nil {
		holder, expires = t.Lease.Holder, formatTime(t.Lease.ExpiresAt)
	}
	return []any{
		nullableID(t.RepoID), string(t.Kind), t.Title, t.Body, t.Spec, string(t.Lane), t.Rank, t.PRURL, t.Branch,
		holder, expires, t.CreatedBy, formatTime(t.CreatedAt), formatTime(t.UpdatedAt), formatTime(t.LaneChangedAt),
	}
}

// filterClause turns a filter into a WHERE fragment. Only "?" placeholders are
// generated; every value travels as a bound argument.
func filterClause(filter app.TicketFilter) (string, []any) {
	if !filter.IsScoped {
		return "1 = 1", nil
	}
	var parts []string
	var args []any
	if len(filter.RepoIDs) > 0 {
		parts = append(parts, "repo_id IN ("+strings.TrimSuffix(strings.Repeat("?, ", len(filter.RepoIDs)), ", ")+")")
		for _, id := range filter.RepoIDs {
			args = append(args, id)
		}
	}
	if filter.IncludeUnassigned {
		parts = append(parts, "repo_id IS NULL")
	}
	if len(parts) == 0 {
		return "1 = 0", nil
	}
	return "(" + strings.Join(parts, " OR ") + ")", args
}

// nullableID stores zero, meaning "none", as NULL.
func nullableID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTicket(row scanner) (domain.Ticket, error) {
	var t domain.Ticket
	var kind, lane, holder, expires, created, updated, laneChanged string
	var repoID sql.NullInt64
	err := row.Scan(&t.ID, &repoID, &kind, &t.Title, &t.Body, &t.Spec, &lane, &t.Rank, &t.PRURL, &t.Branch,
		&holder, &expires, &t.CreatedBy, &created, &updated, &laneChanged)
	if err != nil {
		return domain.Ticket{}, err
	}
	t.RepoID = repoID.Int64
	t.Kind, t.Lane = domain.Kind(kind), domain.Lane(lane)
	if t.Lease, err = scanLease(holder, expires); err != nil {
		return domain.Ticket{}, err
	}
	return t, parseTicketTimes(&t, created, updated, laneChanged)
}

func scanLease(holder, expires string) (*domain.Lease, error) {
	if holder == "" {
		return nil, nil
	}
	expiresAt, err := parseTime(expires)
	if err != nil {
		return nil, err
	}
	return &domain.Lease{Holder: holder, ExpiresAt: expiresAt}, nil
}

func parseTicketTimes(t *domain.Ticket, created, updated, laneChanged string) error {
	var err error
	if t.CreatedAt, err = parseTime(created); err != nil {
		return err
	}
	if t.UpdatedAt, err = parseTime(updated); err != nil {
		return err
	}
	t.LaneChangedAt, err = parseTime(laneChanged)
	return err
}
