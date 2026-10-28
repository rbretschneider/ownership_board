package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

type codeRepoRepo struct {
	q querier
}

const repoColumns = `id, provider, full_name, url, description, color, default_branch, is_private, is_archived, synced_at, created_at, missing_since, treat_as_public,
	require_spec_approval, require_design_approval, allow_autonomous`

func (r codeRepoRepo) List(ctx context.Context) ([]domain.Repo, error) {
	return r.query(ctx, `SELECT `+repoColumns+` FROM repos ORDER BY full_name COLLATE NOCASE, provider`)
}

func (r codeRepoRepo) Get(ctx context.Context, id int64) (domain.Repo, error) {
	repo, err := scanRepo(r.q.QueryRowContext(ctx, `SELECT `+repoColumns+` FROM repos WHERE id = ?`, id))
	return repo, notFoundOr(err, "repo %d does not exist", id)
}

func (r codeRepoRepo) ByFullName(ctx context.Context, fullName string) ([]domain.Repo, error) {
	return r.query(ctx, `SELECT `+repoColumns+` FROM repos WHERE full_name = ? COLLATE NOCASE ORDER BY provider`, fullName)
}

func (r codeRepoRepo) ByProvider(ctx context.Context, provider domain.Provider) ([]domain.Repo, error) {
	return r.query(ctx, `SELECT `+repoColumns+` FROM repos WHERE provider = ?`, string(provider))
}

func (r codeRepoRepo) Insert(ctx context.Context, repo *domain.Repo) error {
	result, err := r.q.ExecContext(ctx,
		`INSERT INTO repos (provider, full_name, url, description, color, default_branch, is_private, is_archived, synced_at, created_at, missing_since, treat_as_public,
			require_spec_approval, require_design_approval, allow_autonomous)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(repo.Provider), repo.FullName, repo.URL, repo.Description, repo.Color, repo.DefaultBranch,
		boolToInt(repo.IsPrivate), boolToInt(repo.IsArchived), formatOptionalTime(repo.SyncedAt), formatTime(repo.CreatedAt), formatOptionalTime(repo.MissingSince), boolToInt(repo.TreatAsPublic),
		boolToInt(repo.RequireSpecApproval), boolToInt(repo.RequireDesignApproval), boolToInt(repo.AllowAutonomous))
	if err != nil {
		return conflictOr(err, "repo %s is already on the board", repo.FullName)
	}
	repo.ID, err = result.LastInsertId()
	return err
}

func (r codeRepoRepo) Update(ctx context.Context, repo domain.Repo) error {
	_, err := r.q.ExecContext(ctx,
		`UPDATE repos SET full_name = ?, url = ?, description = ?, color = ?, default_branch = ?,
			is_private = ?, is_archived = ?, synced_at = ?, missing_since = ?, treat_as_public = ?
		 WHERE id = ?`,
		repo.FullName, repo.URL, repo.Description, repo.Color, repo.DefaultBranch,
		boolToInt(repo.IsPrivate), boolToInt(repo.IsArchived), formatOptionalTime(repo.SyncedAt), formatOptionalTime(repo.MissingSince), boolToInt(repo.TreatAsPublic), repo.ID)
	if err != nil {
		return conflictOr(err, "repo %s is already on the board", repo.FullName)
	}
	return nil
}

// SetGates writes the checkpoint settings and whether autonomous tickets may
// skip them. Update never touches them, so a repo sync can never switch a
// checkpoint off or let autonomous tickets in.
func (r codeRepoRepo) SetGates(ctx context.Context, id int64, gates domain.Gates, allowAutonomous bool) error {
	_, err := r.q.ExecContext(ctx, `UPDATE repos SET require_spec_approval = ?, require_design_approval = ?, allow_autonomous = ? WHERE id = ?`,
		boolToInt(gates.Spec), boolToInt(gates.Design), boolToInt(allowAutonomous), id)
	if err != nil {
		return fmt.Errorf("sqlite: set gates of repo %d: %w", id, err)
	}
	return nil
}

func (r codeRepoRepo) query(ctx context.Context, query string, args ...any) ([]domain.Repo, error) {
	rows, err := r.q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query repos: %w", err)
	}
	defer rows.Close()
	var repos []domain.Repo
	for rows.Next() {
		repo, err := scanRepo(rows)
		if err != nil {
			return nil, err
		}
		repos = append(repos, repo)
	}
	return repos, rows.Err()
}

func scanRepo(row scanner) (domain.Repo, error) {
	var repo domain.Repo
	var provider, created string
	var isPrivate, isArchived, treatAsPublic, requireSpec, requireDesign, allowAutonomous int
	var synced, missing sql.NullString
	err := row.Scan(&repo.ID, &provider, &repo.FullName, &repo.URL, &repo.Description, &repo.Color, &repo.DefaultBranch,
		&isPrivate, &isArchived, &synced, &created, &missing, &treatAsPublic, &requireSpec, &requireDesign, &allowAutonomous)
	if err != nil {
		return domain.Repo{}, err
	}
	repo.Provider, repo.IsPrivate, repo.IsArchived = domain.Provider(provider), isPrivate == 1, isArchived == 1
	repo.TreatAsPublic = treatAsPublic == 1
	repo.RequireSpecApproval, repo.RequireDesignApproval = requireSpec == 1, requireDesign == 1
	repo.AllowAutonomous = allowAutonomous == 1
	if repo.SyncedAt, err = parseOptionalTime(synced); err != nil {
		return domain.Repo{}, err
	}
	if repo.MissingSince, err = parseOptionalTime(missing); err != nil {
		return domain.Repo{}, err
	}
	repo.CreatedAt, err = parseTime(created)
	return repo, err
}
