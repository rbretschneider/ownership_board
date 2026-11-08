package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

type buildJobRepo struct{ q querier }

const jobColumns = `id,ticket_id,repo,sha,preset,target,status,host_id,attempt,created_at,started_at,finished_at,assignment_released_at,phase,detail,result,bundle_bytes,bundle_sha256`

func scanJob(row interface{ Scan(...any) error }) (domain.BuildJob, error) {
	var j domain.BuildJob
	var target, created string
	var started, finished, released, result sql.NullString
	var host sql.NullInt64
	err := row.Scan(&j.ID, &j.TicketID, &j.Repo, &j.SHA, &j.Preset, &target, &j.Status, &host, &j.Attempt, &created, &started, &finished, &released, &j.Phase, &j.Detail, &result, &j.BundleBytes, &j.BundleSHA256)
	if err != nil {
		return j, notFoundOr(err, "build job not found")
	}
	if err = json.Unmarshal([]byte(target), &j.Target); err != nil {
		return j, err
	}
	if j.CreatedAt, err = parseTime(created); err != nil {
		return j, err
	}
	for _, p := range []struct {
		src sql.NullString
		dst **time.Time
	}{{started, &j.StartedAt}, {finished, &j.FinishedAt}, {released, &j.AssignmentReleasedAt}} {
		if p.src.Valid {
			at, e := parseTime(p.src.String)
			if e != nil {
				return j, e
			}
			*p.dst = &at
		}
	}
	if host.Valid {
		j.HostID = &host.Int64
	}
	if result.Valid {
		j.Result = []byte(result.String)
	}
	return j, nil
}
func (r buildJobRepo) Get(ctx context.Context, id string) (domain.BuildJob, error) {
	return scanJob(r.q.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM build_jobs WHERE id=?`, id))
}
func (r buildJobRepo) List(ctx context.Context) ([]domain.BuildJob, error) {
	rows, e := r.q.QueryContext(ctx, `SELECT `+jobColumns+` FROM build_jobs ORDER BY rowid`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.BuildJob{}
	for rows.Next() {
		j, e := scanJob(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
func (r buildJobRepo) Insert(ctx context.Context, j domain.BuildJob) error {
	_, e := r.q.ExecContext(ctx, `INSERT INTO build_jobs(id,ticket_id,repo,sha,preset,target,target_os,status,attempt,created_at,bundle_bytes,bundle_sha256) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, j.ID, j.TicketID, j.Repo, j.SHA, j.Preset, j.Target.Canonical(), j.Target.OS, j.Status, j.Attempt, formatTime(j.CreatedAt), j.BundleBytes, j.BundleSHA256)
	return conflictOr(e, "build job already exists")
}
func jobArgs(j domain.BuildJob) []any {
	var result any
	if j.Result != nil {
		result = string(j.Result)
	}
	return []any{j.Status, nullableID64(j.HostID), j.Attempt, formatOptionalTime(j.StartedAt), formatOptionalTime(j.FinishedAt), formatOptionalTime(j.AssignmentReleasedAt), j.Phase, j.Detail, result, j.ID}
}

const jobUpdate = `UPDATE build_jobs SET status=?,host_id=?,attempt=?,started_at=?,finished_at=?,assignment_released_at=?,phase=?,detail=?,result=? WHERE id=?`

func (r buildJobRepo) Save(ctx context.Context, j domain.BuildJob) error {
	_, e := r.q.ExecContext(ctx, jobUpdate, jobArgs(j)...)
	return e
}
func (r buildJobRepo) Assign(ctx context.Context, j domain.BuildJob) error {
	res, e := r.q.ExecContext(ctx, jobUpdate+` AND status='queued' AND host_id IS NULL`, jobArgs(j)...)
	if e != nil {
		return e
	}
	n, e := res.RowsAffected()
	if e == nil && n != 1 {
		return domain.Conflictf("job is no longer queued")
	}
	return e
}
func (r buildJobRepo) Uploads(ctx context.Context) ([]app.BuildJobUpload, error) {
	rows, e := r.q.QueryContext(ctx, `SELECT token,job_id,metadata,created_at FROM build_job_uploads WHERE state='active'`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []app.BuildJobUpload{}
	for rows.Next() {
		var u app.BuildJobUpload
		var raw, at string
		if e = rows.Scan(&u.Token, &u.JobID, &raw, &at); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(raw), &u.Metadata); e != nil {
			return nil, e
		}
		if u.CreatedAt, e = parseTime(at); e != nil {
			return nil, e
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
func (r buildJobRepo) Reserve(ctx context.Context, u app.BuildJobUpload) error {
	raw, _ := json.Marshal(u.Metadata)
	_, e := r.q.ExecContext(ctx, `INSERT INTO build_job_uploads(token,job_id,metadata,reserved_bytes,created_at,state) VALUES(?,?,?,?,?,'active')`, u.Token, u.JobID, string(raw), u.Metadata.BundleBytes, formatTime(u.CreatedAt))
	return conflictOr(e, "upload_in_progress")
}
func (r buildJobRepo) FinishUpload(ctx context.Context, token string) error {
	_, e := r.q.ExecContext(ctx, `UPDATE build_job_uploads SET state='abandoned' WHERE token=?`, token)
	return e
}
