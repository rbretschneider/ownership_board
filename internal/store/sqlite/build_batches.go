package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// buildBatchRepo persists batched verification runs and their member jobs.
type buildBatchRepo struct{ q querier }

const batchColumns = `id,repo,preset,target,status,COALESCE(parent_id,''),host_id,attempt,created_at,started_at,finished_at,assignment_released_at,phase,detail,result`

func scanBatch(row interface{ Scan(...any) error }) (domain.BuildBatch, error) {
	var b domain.BuildBatch
	var target, created string
	var started, finished, released, result sql.NullString
	var host sql.NullInt64
	err := row.Scan(&b.ID, &b.Repo, &b.Preset, &target, &b.Status, &b.ParentID, &host, &b.Attempt, &created, &started, &finished, &released, &b.Phase, &b.Detail, &result)
	if err != nil {
		return b, notFoundOr(err, "build batch not found")
	}
	if err = json.Unmarshal([]byte(target), &b.Target); err != nil {
		return b, err
	}
	if b.CreatedAt, err = parseTime(created); err != nil {
		return b, err
	}
	for _, p := range []struct {
		src sql.NullString
		dst **time.Time
	}{{started, &b.StartedAt}, {finished, &b.FinishedAt}, {released, &b.AssignmentReleasedAt}} {
		if p.src.Valid {
			at, e := parseTime(p.src.String)
			if e != nil {
				return b, e
			}
			*p.dst = &at
		}
	}
	if host.Valid {
		b.HostID = &host.Int64
	}
	if result.Valid {
		b.Result = []byte(result.String)
	}
	return b, nil
}

func (r buildBatchRepo) members(ctx context.Context, b *domain.BuildBatch) error {
	rows, err := r.q.QueryContext(ctx, `SELECT m.job_id,j.ticket_id,j.sha,m.position,m.outcome,j.bundle_bytes,j.bundle_sha256
 FROM build_batch_members m JOIN build_jobs j ON j.id=m.job_id WHERE m.batch_id=? ORDER BY m.position`, b.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	b.Members = []domain.BuildBatchMember{}
	for rows.Next() {
		var m domain.BuildBatchMember
		if err = rows.Scan(&m.JobID, &m.TicketID, &m.SHA, &m.Position, &m.Outcome, &m.BundleBytes, &m.BundleSHA256); err != nil {
			return err
		}
		b.Members = append(b.Members, m)
	}
	return rows.Err()
}

func (r buildBatchRepo) list(ctx context.Context, query string, args ...any) ([]domain.BuildBatch, error) {
	rows, err := r.q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	out := []domain.BuildBatch{}
	for rows.Next() {
		b, e := scanBatch(rows)
		if e != nil {
			rows.Close()
			return nil, e
		}
		out = append(out, b)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for i := range out {
		if err = r.members(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Get returns a batch with its members in merge order.
func (r buildBatchRepo) Get(ctx context.Context, id string) (domain.BuildBatch, error) {
	b, err := scanBatch(r.q.QueryRowContext(ctx, `SELECT `+batchColumns+` FROM build_batches WHERE id=?`, id))
	if err != nil {
		return b, err
	}
	return b, r.members(ctx, &b)
}

// Insert stores a batch and its members, and holds every member job for it.
func (r buildBatchRepo) Insert(ctx context.Context, b domain.BuildBatch) error {
	var parent any
	if b.ParentID != "" {
		parent = b.ParentID
	}
	_, err := r.q.ExecContext(ctx, `INSERT INTO build_batches(id,repo,preset,target,target_os,status,parent_id,host_id,attempt,created_at,started_at,phase,detail) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		b.ID, b.Repo, b.Preset, b.Target.Canonical(), b.Target.OS, b.Status, parent, nullableID64(b.HostID), b.Attempt, formatTime(b.CreatedAt), formatOptionalTime(b.StartedAt), b.Phase, b.Detail)
	if err != nil {
		return conflictOr(err, "build batch already exists")
	}
	for i, m := range b.Members {
		if _, err = r.q.ExecContext(ctx, `INSERT INTO build_batch_members(batch_id,job_id,position,outcome) VALUES(?,?,?,?)`, b.ID, m.JobID, i, m.Outcome); err != nil {
			return err
		}
		res, err := r.q.ExecContext(ctx, `UPDATE build_jobs SET batch_id=? WHERE id=? AND status='queued' AND batch_id IS NULL AND host_id IS NULL`, b.ID, m.JobID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return domain.Conflictf("job is no longer free to batch")
		}
	}
	return nil
}

// Save writes the batch's run state and each member's outcome.
func (r buildBatchRepo) Save(ctx context.Context, b domain.BuildBatch) error {
	var result any
	if b.Result != nil {
		result = string(b.Result)
	}
	_, err := r.q.ExecContext(ctx, `UPDATE build_batches SET status=?,host_id=?,attempt=?,started_at=?,finished_at=?,assignment_released_at=?,phase=?,detail=?,result=? WHERE id=?`,
		b.Status, nullableID64(b.HostID), b.Attempt, formatOptionalTime(b.StartedAt), formatOptionalTime(b.FinishedAt), formatOptionalTime(b.AssignmentReleasedAt), b.Phase, b.Detail, result, b.ID)
	if err != nil {
		return err
	}
	for _, m := range b.Members {
		if _, err = r.q.ExecContext(ctx, `UPDATE build_batch_members SET outcome=? WHERE batch_id=? AND job_id=?`, m.Outcome, b.ID, m.JobID); err != nil {
			return err
		}
	}
	return nil
}

const batchEligible = ` FROM build_batches WHERE status='queued' AND target_os=? AND NOT EXISTS (
 SELECT 1 FROM json_each(build_batches.target,'$.required_capabilities') required
 WHERE NOT EXISTS (SELECT 1 FROM json_each(?) allowed WHERE allowed.value=required.value)
 ) ORDER BY (SELECT min(j.created_at) FROM build_batch_members m JOIN build_jobs j ON j.id=m.job_id WHERE m.batch_id=build_batches.id),created_at,id LIMIT 20`

// QueuedForHost returns the queued batches (bisection halves) a host can run, in
// queue order of their oldest commit.
func (r buildBatchRepo) QueuedForHost(ctx context.Context, h domain.BuildHost) ([]domain.BuildBatch, error) {
	caps, err := json.Marshal(h.Capabilities)
	if err != nil {
		return nil, err
	}
	return r.list(ctx, `SELECT `+batchColumns+batchEligible, h.OS, string(caps))
}

// ActiveAssignments returns every batch a host holds and has not released.
func (r buildBatchRepo) ActiveAssignments(ctx context.Context) ([]domain.BuildBatch, error) {
	return r.list(ctx, `SELECT `+batchColumns+` FROM build_batches WHERE host_id IS NOT NULL AND assignment_released_at IS NULL ORDER BY host_id,started_at,id`)
}

// AssignmentCount is how many host slots the host's unreleased batches take.
func (r buildBatchRepo) AssignmentCount(ctx context.Context, hostID int64) (int, error) {
	var n int
	err := r.q.QueryRowContext(ctx, `SELECT count(*) FROM build_batches WHERE host_id=? AND assignment_released_at IS NULL`, hostID).Scan(&n)
	return n, err
}

// ForTicket returns the batches that held any of the ticket's jobs, newest first.
func (r buildBatchRepo) ForTicket(ctx context.Context, ticketID int64, limit int) ([]domain.BuildBatch, error) {
	return r.list(ctx, `SELECT `+batchColumns+` FROM build_batches WHERE id IN (
 SELECT m.batch_id FROM build_batch_members m JOIN build_jobs j ON j.id=m.job_id WHERE j.ticket_id=?) ORDER BY created_at DESC, id DESC LIMIT ?`, ticketID, limit)
}
