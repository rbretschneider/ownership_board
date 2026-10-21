package sqlite

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

type timelineRepo struct {
	q querier
}

func (r timelineRepo) Append(ctx context.Context, entry *domain.TimelineEntry) error {
	data, err := json.Marshal(entry.Data)
	if err != nil {
		return fmt.Errorf("sqlite: encode timeline data: %w", err)
	}
	result, err := r.q.ExecContext(ctx,
		`INSERT INTO timeline (ticket_id, type, actor_kind, actor_name, body, data, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.TicketID, string(entry.Type), string(entry.Actor.Kind), entry.Actor.Name, entry.Body, string(data), formatTime(entry.At))
	if err != nil {
		return fmt.Errorf("sqlite: append timeline: %w", err)
	}
	entry.ID, err = result.LastInsertId()
	return err
}

func (r timelineRepo) ForTicket(ctx context.Context, ticketID int64) ([]domain.TimelineEntry, error) {
	rows, err := r.q.QueryContext(ctx,
		`SELECT id, ticket_id, type, actor_kind, actor_name, body, data, created_at
		 FROM timeline WHERE ticket_id = ? ORDER BY id`, ticketID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: timeline of ticket %d: %w", ticketID, err)
	}
	defer rows.Close()
	var entries []domain.TimelineEntry
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func scanEntry(row scanner) (domain.TimelineEntry, error) {
	var e domain.TimelineEntry
	var kind, actorKind, data, at string
	if err := row.Scan(&e.ID, &e.TicketID, &kind, &actorKind, &e.Actor.Name, &e.Body, &data, &at); err != nil {
		return domain.TimelineEntry{}, err
	}
	e.Type, e.Actor.Kind = domain.TimelineType(kind), domain.ActorKind(actorKind)
	if err := json.Unmarshal([]byte(data), &e.Data); err != nil {
		return domain.TimelineEntry{}, fmt.Errorf("sqlite: decode timeline %d: %w", e.ID, err)
	}
	var err error
	e.At, err = parseTime(at)
	return e, err
}
