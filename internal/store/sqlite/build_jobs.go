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
func (r buildJobRepo) ActiveAssignments(ctx context.Context) ([]domain.BuildJob, error) {
	rows, e := r.q.QueryContext(ctx, `SELECT `+jobColumns+` FROM build_jobs WHERE host_id IS NOT NULL AND assignment_released_at IS NULL`)
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
	_, e := r.q.ExecContext(ctx, `INSERT INTO build_jobs(id,ticket_id,repo,sha,preset,target,target_os,status,attempt,created_at,finished_at,bundle_bytes,bundle_sha256) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, j.ID, j.TicketID, j.Repo, j.SHA, j.Preset, j.Target.Canonical(), j.Target.OS, j.Status, j.Attempt, formatTime(j.CreatedAt), formatOptionalTime(j.FinishedAt), j.BundleBytes, j.BundleSHA256)
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

func (r buildJobRepo) AssignmentCount(ctx context.Context, hostID int64) (int, error) {
	var n int
	err := r.q.QueryRowContext(ctx, `SELECT count(*) FROM build_jobs WHERE host_id=? AND assignment_released_at IS NULL`, hostID).Scan(&n)
	return n, err
}

const eligibleQueue = ` FROM build_jobs WHERE status='queued' AND target_os=? AND NOT EXISTS (
 SELECT 1 FROM json_each(build_jobs.target,'$.required_capabilities') required
 WHERE NOT EXISTS (SELECT 1 FROM json_each(?) allowed WHERE allowed.value=required.value)
 ) ORDER BY created_at,id LIMIT ?`

func (r buildJobRepo) QueuedForHost(ctx context.Context, h domain.BuildHost, limit int) ([]domain.BuildJob, error) {
	if limit < 1 || limit > 40 {
		return nil, domain.Invalidf("queue limit must be between 1 and 40")
	}
	caps, err := json.Marshal(h.Capabilities)
	if err != nil {
		return nil, err
	}
	rows, err := r.q.QueryContext(ctx, `SELECT `+jobColumns+eligibleQueue, h.OS, string(caps), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.BuildJob{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
func (r buildJobRepo) Candidate(ctx context.Context, h domain.BuildHost) (*domain.BuildJob, error) {
	jobs, err := r.QueuedForHost(ctx, h, 1)
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return nil, nil
	}
	return &jobs[0], nil
}

// CommitState returns publication order without loading retained job bodies.
func (r buildJobRepo) CommitState(ctx context.Context, ticketID int64, target domain.BuildJobTarget, sha string) (known bool, latest string, err error) {
	err = r.q.QueryRowContext(ctx, `SELECT
 EXISTS(SELECT 1 FROM build_jobs WHERE ticket_id=? AND target=? AND sha=?),
 COALESCE((SELECT sha FROM build_jobs WHERE ticket_id=? AND target=? GROUP BY sha ORDER BY min(rowid) DESC LIMIT 1),'')`,
		ticketID, target.Canonical(), sha, ticketID, target.Canonical()).Scan(&known, &latest)
	return
}
func (r buildJobRepo) Supersede(ctx context.Context, ticketID int64, target domain.BuildJobTarget, sha string, now time.Time) error {
	_, err := r.q.ExecContext(ctx, `UPDATE build_jobs SET status='superseded',finished_at=? WHERE ticket_id=? AND target_os=? AND target=? AND sha<>? AND status IN ('queued','running')`, formatTime(now), ticketID, target.OS, target.Canonical(), sha)
	return err
}
func (r buildJobRepo) SupersedeCompleted(ctx context.Context, now time.Time) error {
	_, err := r.q.ExecContext(ctx, `UPDATE build_jobs SET status='superseded',finished_at=? WHERE status IN ('queued','running') AND ticket_id IN (SELECT id FROM tickets WHERE stage IN ('done','deploying','deployed'))`, formatTime(now))
	return err
}
func (r buildJobRepo) Bundles(ctx context.Context) ([]app.BuildJobBundle, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT id,bundle_bytes,bundle_sha256 FROM build_jobs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []app.BuildJobBundle
	for rows.Next() {
		var b app.BuildJobBundle
		if err := rows.Scan(&b.ID, &b.Bytes, &b.SHA256); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r buildJobRepo) RunnerAssignments(ctx context.Context) ([]app.RunnerAssignment, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT host_id,id,ticket_id,repo,sha,preset,status,phase,detail,started_at,attempt FROM build_jobs WHERE host_id IS NOT NULL AND assignment_released_at IS NULL ORDER BY host_id,started_at,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []app.RunnerAssignment{}
	for rows.Next() {
		var a app.RunnerAssignment
		var started sql.NullString
		if err = rows.Scan(&a.HostID, &a.ID, &a.TicketID, &a.Repo, &a.SHA, &a.Preset, &a.Status, &a.Phase, &a.Detail, &started, &a.Attempt); err != nil {
			return nil, err
		}
		if started.Valid {
			at, e := parseTime(started.String)
			if e != nil {
				return nil, e
			}
			a.StartedAt = &at
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (r buildJobRepo) QueuedCountsByOS(ctx context.Context) (map[domain.BuildHostOS]int, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT target_os,count(*) FROM build_jobs WHERE status='queued' GROUP BY target_os`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[domain.BuildHostOS]int{}
	for rows.Next() {
		var os domain.BuildHostOS
		var n int
		if err = rows.Scan(&os, &n); err != nil {
			return nil, err
		}
		out[os] = n
	}
	return out, rows.Err()
}
