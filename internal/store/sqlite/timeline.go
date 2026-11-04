package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

type timelineRepo struct {
	q querier
}

const timelineColumns = `id, ticket_id, type, actor_kind, actor_name, body, data, created_at`

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
		`SELECT `+timelineColumns+` FROM timeline WHERE ticket_id = ? ORDER BY id`, ticketID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: timeline of ticket %d: %w", ticketID, err)
	}
	return scanEntries(rows)
}

func (r timelineRepo) OfTypes(ctx context.Context, types []domain.TimelineType) ([]domain.TimelineEntry, error) {
	if len(types) == 0 {
		return nil, nil
	}
	args := make([]any, len(types))
	for i, kind := range types {
		args[i] = string(kind)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(types)), ",")
	rows, err := r.q.QueryContext(ctx,
		`SELECT `+timelineColumns+` FROM timeline WHERE type IN (`+placeholders+`) ORDER BY ticket_id, id`, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: timeline of types %v: %w", types, err)
	}
	return scanEntries(rows)
}

func (r timelineRepo) StageSignals(ctx context.Context) ([]domain.TimelineEntry, error) {
	rows, err := r.q.QueryContext(ctx,
		`SELECT `+timelineColumns+` FROM timeline
		 WHERE type = ?
		    OR (type = ? AND json_extract(data, '$.model') IS NOT NULL)
		    OR (type = ? AND actor_kind = ? AND body LIKE 'Windows verification queued for commit %')
		 ORDER BY ticket_id, id`,
		string(domain.TimelineStage), string(domain.TimelineEdited), string(domain.TimelineComment), string(domain.ActorAgent))
	if err != nil {
		return nil, fmt.Errorf("sqlite: stage signals: %w", err)
	}
	return scanEntries(rows)
}

func scanEntries(rows *sql.Rows) ([]domain.TimelineEntry, error) {
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
