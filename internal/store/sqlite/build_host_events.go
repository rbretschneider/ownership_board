package sqlite

import (
	"context"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

type buildHostEventRepo struct{ q querier }

func (r buildHostEventRepo) Append(ctx context.Context, e *domain.BuildHostEvent) error {
	res, err := r.q.ExecContext(ctx, `INSERT INTO build_host_events(host_id,event_type,data,created_at) VALUES(?,?,?,?)`, e.HostID, e.Type, string(e.Data), formatTime(e.CreatedAt))
	if err != nil {
		return err
	}
	e.ID, err = res.LastInsertId()
	return err
}
func (r buildHostEventRepo) ForHost(ctx context.Context, id, after int64, limit int) ([]domain.BuildHostEvent, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT id,host_id,event_type,data,created_at FROM build_host_events WHERE host_id=? AND id>? ORDER BY id LIMIT ?`, id, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.BuildHostEvent{}
	for rows.Next() {
		var e domain.BuildHostEvent
		var raw, at string
		if err = rows.Scan(&e.ID, &e.HostID, &e.Type, &raw, &at); err != nil {
			return nil, err
		}
		e.Data = []byte(raw)
		if e.CreatedAt, err = parseTime(at); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
