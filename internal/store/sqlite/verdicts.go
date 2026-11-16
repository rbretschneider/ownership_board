package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

type verdictRepo struct{ q querier }

const verdictColumns = `repo, target, sha, status, merge_of, job_id, ticket_id, source, reported_by, recorded_at`

func scanVerdict(row scanner) (domain.Verdict, error) {
	var v domain.Verdict
	var status, source, recorded string
	if err := row.Scan(&v.Repo, &v.Target, &v.SHA, &status, &v.MergeOf, &v.JobID, &v.TicketID, &source, &v.ReportedBy, &recorded); err != nil {
		return v, notFoundOr(err, "no verdict for that commit and target")
	}
	v.Status, v.Source = domain.VerdictStatus(status), domain.VerdictSource(source)
	var err error
	v.RecordedAt, err = parseTime(recorded)
	return v, err
}

func scanVerdicts(rows *sql.Rows, err error) ([]domain.Verdict, error) {
	if err != nil {
		return nil, fmt.Errorf("sqlite: read verdicts: %w", err)
	}
	defer rows.Close()
	out := []domain.Verdict{}
	for rows.Next() {
		v, err := scanVerdict(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Record implements app.VerdictRepository.
func (r verdictRepo) Record(ctx context.Context, v domain.Verdict) error {
	_, err := r.q.ExecContext(ctx, `INSERT OR REPLACE INTO verdicts (`+verdictColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		v.Repo, v.Target, v.SHA, string(v.Status), v.MergeOf, v.JobID, v.TicketID, string(v.Source), v.ReportedBy, formatTime(v.RecordedAt))
	if err != nil {
		return fmt.Errorf("sqlite: record verdict: %w", err)
	}
	return nil
}

// Get implements app.VerdictRepository.
func (r verdictRepo) Get(ctx context.Context, repo, target, sha string) (domain.Verdict, error) {
	return scanVerdict(r.q.QueryRowContext(ctx, `SELECT `+verdictColumns+` FROM verdicts WHERE repo=? AND target=? AND sha=?`, repo, target, sha))
}

// ForCommit implements app.VerdictRepository.
func (r verdictRepo) ForCommit(ctx context.Context, repo, sha string) ([]domain.Verdict, error) {
	return scanVerdicts(r.q.QueryContext(ctx, `SELECT `+verdictColumns+` FROM verdicts WHERE repo=? AND sha=? ORDER BY target`, repo, sha))
}

// ForTicket implements app.VerdictRepository: newest first.
func (r verdictRepo) ForTicket(ctx context.Context, ticketID int64, limit int) ([]domain.Verdict, error) {
	return scanVerdicts(r.q.QueryContext(ctx, `SELECT `+verdictColumns+` FROM verdicts WHERE ticket_id=?
		ORDER BY recorded_at DESC, rowid DESC LIMIT ?`, ticketID, limit))
}

// Latest implements app.VerdictRepository: each ticket's newest verdict per target.
func (r verdictRepo) Latest(ctx context.Context, ticketIDs []int64) (map[int64][]domain.Verdict, error) {
	out := map[int64][]domain.Verdict{}
	// Chunk to stay well inside SQLite's bound-parameter limit.
	for start := 0; start < len(ticketIDs); start += 500 {
		chunk := ticketIDs[start:min(start+500, len(ticketIDs))]
		args := make([]any, len(chunk))
		for i, id := range chunk {
			args[i] = id
		}
		verdicts, err := scanVerdicts(r.q.QueryContext(ctx, `SELECT `+verdictColumns+` FROM (
			SELECT *, row_number() OVER (PARTITION BY ticket_id, target ORDER BY recorded_at DESC, rowid DESC) AS rn
			FROM verdicts WHERE ticket_id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")+`)
		) WHERE rn=1 ORDER BY ticket_id, target`, args...))
		if err != nil {
			return nil, err
		}
		for _, v := range verdicts {
			out[v.TicketID] = append(out[v.TicketID], v)
		}
	}
	return out, nil
}
