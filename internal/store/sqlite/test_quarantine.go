package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// testQuarantineRepo persists failed-test observations and quarantine lists.
type testQuarantineRepo struct{ q querier }

// RecordFailures stores failed tests; a repeated row is ignored.
func (r testQuarantineRepo) RecordFailures(ctx context.Context, failures []domain.TestFailure) error {
	for _, f := range failures {
		if _, err := r.q.ExecContext(ctx, `INSERT OR IGNORE INTO test_failures(run_id,repo,ticket_id,sha,test,recorded_at) VALUES(?,?,?,?,?,?)`,
			f.RunID, f.Repo, f.TicketID, f.SHA, f.Test, formatTime(f.RecordedAt)); err != nil {
			return err
		}
	}
	return nil
}

func (r testQuarantineRepo) failures(ctx context.Context, query string, args ...any) ([]domain.TestFailure, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT run_id,repo,ticket_id,sha,test,recorded_at FROM test_failures `+query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.TestFailure{}
	for rows.Next() {
		var f domain.TestFailure
		var at string
		if err = rows.Scan(&f.RunID, &f.Repo, &f.TicketID, &f.SHA, &f.Test, &at); err != nil {
			return nil, err
		}
		if f.RecordedAt, err = parseTime(at); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// FailuresOf returns a test's failures in a repo since a time, oldest first.
func (r testQuarantineRepo) FailuresOf(ctx context.Context, repo, test string, since time.Time) ([]domain.TestFailure, error) {
	return r.failures(ctx, `WHERE repo=? AND test=? AND recorded_at>=? ORDER BY recorded_at,run_id`, repo, test, formatTime(since))
}

// FailuresAt returns every failure recorded on a commit.
func (r testQuarantineRepo) FailuresAt(ctx context.Context, repo, sha string) ([]domain.TestFailure, error) {
	return r.failures(ctx, `WHERE repo=? AND sha=? ORDER BY recorded_at,run_id`, repo, sha)
}

// CountFailures returns how many failure rows exist.
func (r testQuarantineRepo) CountFailures(ctx context.Context) (int, error) {
	var n int
	err := r.q.QueryRowContext(ctx, `SELECT count(*) FROM test_failures`).Scan(&n)
	return n, err
}

const quarantineColumns = `repo_id,test,status,source,added_by,reason,evidence,fix_ticket_id,passes,created_at,updated_at,released_at`

func scanQuarantine(row interface{ Scan(...any) error }) (domain.QuarantinedTest, error) {
	var q domain.QuarantinedTest
	var evidence, created, updated string
	var fix sql.NullInt64
	var released sql.NullString
	if err := row.Scan(&q.RepoID, &q.Test, &q.Status, &q.Source, &q.AddedBy, &q.Reason, &evidence, &fix, &q.Passes, &created, &updated, &released); err != nil {
		return q, notFoundOr(err, "test is not on the quarantine list")
	}
	q.Evidence = []byte(evidence)
	if fix.Valid {
		q.FixTicketID = &fix.Int64
	}
	var err error
	if q.CreatedAt, err = parseTime(created); err != nil {
		return q, err
	}
	if q.UpdatedAt, err = parseTime(updated); err != nil {
		return q, err
	}
	if released.Valid {
		at, err := parseTime(released.String)
		if err != nil {
			return q, err
		}
		q.ReleasedAt = &at
	}
	return q, nil
}

// Get returns one test's quarantine entry.
func (r testQuarantineRepo) Get(ctx context.Context, repoID int64, test string) (domain.QuarantinedTest, error) {
	return scanQuarantine(r.q.QueryRowContext(ctx, `SELECT `+quarantineColumns+` FROM test_quarantine WHERE repo_id=? AND test=?`, repoID, test))
}

// List returns a repo's entries, released ones too, newest first.
func (r testQuarantineRepo) List(ctx context.Context, repoID int64) ([]domain.QuarantinedTest, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT `+quarantineColumns+` FROM test_quarantine WHERE repo_id=? ORDER BY status='released', created_at DESC, test`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.QuarantinedTest{}
	for rows.Next() {
		q, err := scanQuarantine(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// Save inserts or replaces an entry.
func (r testQuarantineRepo) Save(ctx context.Context, q domain.QuarantinedTest) error {
	evidence := string(q.Evidence)
	if evidence == "" {
		evidence = "{}"
	}
	_, err := r.q.ExecContext(ctx, `INSERT INTO test_quarantine(`+quarantineColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
 ON CONFLICT(repo_id,test) DO UPDATE SET status=excluded.status,source=excluded.source,added_by=excluded.added_by,reason=excluded.reason,
 evidence=excluded.evidence,fix_ticket_id=excluded.fix_ticket_id,passes=excluded.passes,updated_at=excluded.updated_at,released_at=excluded.released_at`,
		q.RepoID, q.Test, q.Status, q.Source, q.AddedBy, q.Reason, evidence, nullableID64(q.FixTicketID), q.Passes,
		formatTime(q.CreatedAt), formatTime(q.UpdatedAt), formatOptionalTime(q.ReleasedAt))
	return err
}
