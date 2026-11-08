package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

type buildHostRepo struct{ q querier }

const hostColumns = `id,name,os,arch,status,capabilities,max_parallel,enabled,public_key,facts,agent_version,last_seen_at,created_at,approved_at,approved_by,revoked_at,running_job_ids,free_disk_bytes`

func scanBuildHost(row interface{ Scan(...any) error }) (domain.BuildHost, error) {
	var h domain.BuildHost
	var caps, facts, created, jobs string
	var free sql.NullInt64
	var seen, approved, revoked sql.NullString
	var by sql.NullInt64
	if err := row.Scan(&h.ID, &h.Name, &h.OS, &h.Arch, &h.Status, &caps, &h.MaxParallel, &h.Enabled, &h.PublicKey, &facts, &h.AgentVersion, &seen, &created, &approved, &by, &revoked, &jobs, &free); err != nil {
		return h, err
	}
	if err := json.Unmarshal([]byte(caps), &h.Capabilities); err != nil {
		return h, fmt.Errorf("sqlite: invalid host capabilities: %w", err)
	}
	if h.Capabilities == nil {
		return h, fmt.Errorf("sqlite: host capabilities must be an array")
	}
	h.Facts = json.RawMessage(facts)
	if err := json.Unmarshal([]byte(jobs), &h.RunningJobIDs); err != nil || h.RunningJobIDs == nil {
		return h, fmt.Errorf("sqlite: corrupt host telemetry")
	}
	if free.Valid {
		h.FreeDiskBytes = &free.Int64
	}
	if err := domain.ValidateBuildHostTelemetry(h.RunningJobIDs, h.FreeDiskBytes); err != nil {
		return h, fmt.Errorf("sqlite: corrupt host telemetry")
	}
	var err error
	if h.CreatedAt, err = parseTime(created); err != nil {
		return h, err
	}
	// Unlike legacy optional fields, empty strings here indicate corrupt data.
	for _, v := range []struct {
		raw sql.NullString
		dst **time.Time
	}{{seen, &h.LastSeenAt}, {approved, &h.ApprovedAt}, {revoked, &h.RevokedAt}} {
		if v.raw.Valid {
			at, err := parseTime(v.raw.String)
			if err != nil {
				return h, err
			}
			*v.dst = &at
		}
	}
	if by.Valid {
		h.ApprovedBy = &by.Int64
	}
	if err := h.Validate(); err != nil {
		return h, fmt.Errorf("sqlite: corrupt build host: %v", err)
	}
	return h, nil
}

func (r buildHostRepo) List(ctx context.Context) ([]domain.BuildHost, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT `+hostColumns+` FROM build_hosts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.BuildHost{}
	for rows.Next() {
		h, err := scanBuildHost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (r buildHostRepo) Get(ctx context.Context, id int64) (domain.BuildHost, error) {
	h, err := scanBuildHost(r.q.QueryRowContext(ctx, `SELECT `+hostColumns+` FROM build_hosts WHERE id=?`, id))
	return h, notFoundOr(err, "build host not found")
}

func (r buildHostRepo) Insert(ctx context.Context, h *domain.BuildHost) error {
	if err := h.Validate(); err != nil {
		return err
	}
	caps, err := json.Marshal(h.Capabilities)
	if err != nil {
		return err
	}
	jobs := h.RunningJobIDs
	if jobs == nil {
		jobs = []int64{}
	}
	encodedJobs, _ := json.Marshal(jobs)
	result, err := r.q.ExecContext(ctx, `INSERT INTO build_hosts (name,os,arch,status,capabilities,max_parallel,enabled,public_key,facts,agent_version,last_seen_at,created_at,approved_at,approved_by,revoked_at,running_job_ids,free_disk_bytes) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, h.Name, h.OS, h.Arch, h.Status, string(caps), h.MaxParallel, h.Enabled, h.PublicKey, string(h.Facts), h.AgentVersion, formatOptionalTime(h.LastSeenAt), formatTime(h.CreatedAt), formatOptionalTime(h.ApprovedAt), nullableID64(h.ApprovedBy), formatOptionalTime(h.RevokedAt), string(encodedJobs), nullableID64(h.FreeDiskBytes))
	if err != nil {
		return conflictOr(err, "build host identity already exists")
	}
	h.ID, err = result.LastInsertId()
	return err
}

func (r buildHostRepo) Approve(ctx context.Context, h domain.BuildHost) error {
	if err := h.Validate(); err != nil {
		return err
	}
	if h.Status != domain.BuildHostApproved {
		return domain.Conflictf("host is not approved")
	}
	caps, _ := json.Marshal(h.Capabilities)
	result, err := r.q.ExecContext(ctx, `UPDATE build_hosts SET name=?,capabilities=?,max_parallel=?,approved_at=?,approved_by=?,status='approved' WHERE id=? AND status='pending'`, h.Name, string(caps), h.MaxParallel, formatOptionalTime(h.ApprovedAt), nullableID64(h.ApprovedBy), h.ID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		if _, err := r.Get(ctx, h.ID); err != nil {
			return err
		}
		return domain.Conflictf("host is no longer pending")
	}
	return nil
}

func (r buildHostRepo) Update(ctx context.Context, h domain.BuildHost) error {
	if err := h.Validate(); err != nil {
		return err
	}
	caps, _ := json.Marshal(h.Capabilities)
	result, err := r.q.ExecContext(ctx, `UPDATE build_hosts SET name=?,capabilities=?,max_parallel=?,enabled=? WHERE id=? AND (status!='revoked' OR ?=0)`, h.Name, string(caps), h.MaxParallel, h.Enabled, h.ID, h.Enabled)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		_, err := r.Get(ctx, h.ID)
		if err != nil {
			return err
		}
		return domain.Conflictf("revoked hosts cannot be enabled")
	}
	return nil
}

func (r buildHostRepo) Revoke(ctx context.Context, id int64, at time.Time) error {
	result, err := r.q.ExecContext(ctx, `UPDATE build_hosts SET status='revoked',enabled=0,revoked_at=? WHERE id=? AND status!='revoked'`, formatTime(at), id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		_, err = r.Get(ctx, id)
	}
	return err
}

type buildHostPairingCodeRepo struct{ q querier }

func (r buildHostPairingCodeRepo) Insert(ctx context.Context, c domain.BuildHostPairingCode) error {
	_, err := r.q.ExecContext(ctx, `INSERT INTO build_host_pairing_codes (code_hash,created_by,created_at,expires_at,used_at,used_by_host) VALUES (?,?,?,?,?,?)`, c.CodeHash, c.CreatedBy, formatTime(c.CreatedAt), formatTime(c.ExpiresAt), formatOptionalTime(c.UsedAt), nullableID64(c.UsedByHost))
	return conflictOr(err, "pairing code collision")
}

func (r buildHostPairingCodeRepo) GetByHash(ctx context.Context, hash string) (domain.BuildHostPairingCode, error) {
	var c domain.BuildHostPairingCode
	var created, expires string
	var used sql.NullString
	var host sql.NullInt64
	e := r.q.QueryRowContext(ctx, `SELECT code_hash,created_by,created_at,expires_at,used_at,used_by_host FROM build_host_pairing_codes WHERE code_hash=?`, hash).Scan(&c.CodeHash, &c.CreatedBy, &created, &expires, &used, &host)
	if e != nil {
		return c, notFoundOr(e, "pairing code not found")
	}
	if c.CreatedAt, e = parseTime(created); e != nil {
		return c, e
	}
	if c.ExpiresAt, e = parseTime(expires); e != nil {
		return c, e
	}
	if used.Valid {
		at, e := parseTime(used.String)
		if e != nil {
			return c, e
		}
		c.UsedAt = &at
	}
	if host.Valid {
		c.UsedByHost = &host.Int64
	}
	return c, nil
}
func (r buildHostPairingCodeRepo) Consume(ctx context.Context, hash string, id int64, now time.Time) error {
	res, e := r.q.ExecContext(ctx, `UPDATE build_host_pairing_codes SET used_at=?,used_by_host=? WHERE code_hash=? AND used_at IS NULL AND expires_at>?`, formatTime(now), id, hash, formatTime(now))
	if e != nil {
		return e
	}
	n, e := res.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return domain.Conflictf("pairing code unavailable")
	}
	return nil
}
func (r buildHostRepo) RecordHeartbeat(ctx context.Context, id int64, report domain.BuildHostReport, now time.Time) error {
	f, e := domain.ParseBuildHostFacts(report.Facts)
	if e != nil {
		return e
	}
	if e = report.Validate(); e != nil {
		return e
	}
	jobs, _ := json.Marshal(report.RunningJobIDs)
	res, e := r.q.ExecContext(ctx, `UPDATE build_hosts SET facts=?,os=?,arch=?,agent_version=?,running_job_ids=?,free_disk_bytes=?,last_seen_at=? WHERE id=? AND status='approved'`, string(report.Facts), f.OS, f.Arch, f.AgentVersion, string(jobs), report.FreeDiskBytes, formatTime(now), id)
	if e != nil {
		return e
	}
	n, e := res.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return domain.Conflictf("host is not approved")
	}
	return nil
}
