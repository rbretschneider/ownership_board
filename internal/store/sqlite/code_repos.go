package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

type codeRepoRepo struct {
	q querier
}

const repoColumns = `id, provider, full_name, url, description, color, default_branch, is_private, is_archived, synced_at, created_at, missing_since, treat_as_public,
	require_spec_approval, require_design_approval, allow_agent_merge,
	health_check_every, health_checked_through_ticket, health_checked_at, merge_sequence, health_checked_through_merge_sequence, deploy_mode, build_targets, overlap_ignore`

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
	patterns, err := domain.NormalizeRepoOverlapIgnore(repo.OverlapIgnore)
	if err != nil {
		return err
	}
	ignored, err := json.Marshal(patterns)
	if err != nil {
		return err
	}
	repo.OverlapIgnore = patterns
	targets, err := domain.NormalizeRepoBuildTargets(repo.BuildTargets)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(targets)
	if err != nil {
		return err
	}
	repo.BuildTargets = targets
	result, err := r.q.ExecContext(ctx,
		`INSERT INTO repos (provider, full_name, url, description, color, default_branch, is_private, is_archived, synced_at, created_at, missing_since, treat_as_public,
			require_spec_approval, require_design_approval, allow_agent_merge, health_check_every, deploy_mode, build_targets, overlap_ignore)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(repo.Provider), repo.FullName, repo.URL, repo.Description, repo.Color, repo.DefaultBranch,
		boolToInt(repo.IsPrivate), boolToInt(repo.IsArchived), formatOptionalTime(repo.SyncedAt), formatTime(repo.CreatedAt), formatOptionalTime(repo.MissingSince), boolToInt(repo.TreatAsPublic),
		boolToInt(repo.RequireSpecApproval), boolToInt(repo.RequireDesignApproval), boolToInt(repo.AllowAgentMerge), repo.HealthCheckEvery, repo.DeployMode, string(raw), string(ignored))
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

// SetGates writes the checkpoint, agent-merge and deployment settings. Update never
// touches them, so a repo sync can never switch one.
func (r codeRepoRepo) SetGates(ctx context.Context, repo domain.Repo) error {
	_, err := r.q.ExecContext(ctx, `UPDATE repos SET require_spec_approval = ?, require_design_approval = ?, allow_agent_merge = ?, deploy_mode = ? WHERE id = ?`,
		boolToInt(repo.RequireSpecApproval), boolToInt(repo.RequireDesignApproval), boolToInt(repo.AllowAgentMerge), repo.DeployMode, repo.ID)
	if err != nil {
		return fmt.Errorf("sqlite: set gates of repo %d: %w", repo.ID, err)
	}
	return nil
}

// SetHealthCheck writes the health check interval. Update never touches it.
func (r codeRepoRepo) SetHealthCheck(ctx context.Context, repo domain.Repo) error {
	_, err := r.q.ExecContext(ctx, `UPDATE repos SET health_check_every = ? WHERE id = ?`, repo.HealthCheckEvery, repo.ID)
	if err != nil {
		return fmt.Errorf("sqlite: set health check interval of repo %d: %w", repo.ID, err)
	}
	return nil
}

// RecordHealthCheck stores where the latest health check ended.
func (r codeRepoRepo) RecordHealthCheck(ctx context.Context, repoID, throughTicket, throughSequence int64, at time.Time) error {
	_, err := r.q.ExecContext(ctx, `UPDATE repos SET health_checked_through_ticket = ?, health_checked_at = ?, health_checked_through_merge_sequence = ? WHERE id = ?`,
		throughTicket, formatTime(at), throughSequence, repoID)
	if err != nil {
		return fmt.Errorf("sqlite: record health check of repo %d: %w", repoID, err)
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
	var provider, created, targets, ignored string
	var isPrivate, isArchived, treatAsPublic, requireSpec, requireDesign, allowAgentMerge int
	var synced, missing, checkedAt sql.NullString
	var checkedThrough sql.NullInt64
	err := row.Scan(&repo.ID, &provider, &repo.FullName, &repo.URL, &repo.Description, &repo.Color, &repo.DefaultBranch,
		&isPrivate, &isArchived, &synced, &created, &missing, &treatAsPublic, &requireSpec, &requireDesign, &allowAgentMerge,
		&repo.HealthCheckEvery, &checkedThrough, &checkedAt, &repo.MergeSequence, &repo.HealthCheckedThroughMergeSequence, &repo.DeployMode, &targets, &ignored)
	if err != nil {
		return domain.Repo{}, err
	}
	if err := json.Unmarshal([]byte(ignored), &repo.OverlapIgnore); err != nil {
		return domain.Repo{}, fmt.Errorf("sqlite: repo %d overlap ignore: %w", repo.ID, err)
	}
	if repo.OverlapIgnore, err = domain.NormalizeRepoOverlapIgnore(repo.OverlapIgnore); err != nil {
		return domain.Repo{}, fmt.Errorf("sqlite: repo %d overlap ignore: %w", repo.ID, err)
	}
	if err := json.Unmarshal([]byte(targets), &repo.BuildTargets); err != nil {
		return domain.Repo{}, fmt.Errorf("sqlite: repo %d build targets: %w", repo.ID, err)
	}
	if repo.BuildTargets, err = domain.NormalizeRepoBuildTargets(repo.BuildTargets); err != nil {
		return domain.Repo{}, fmt.Errorf("sqlite: repo %d build targets: %w", repo.ID, err)
	}
	repo.Provider, repo.IsPrivate, repo.IsArchived = domain.Provider(provider), isPrivate == 1, isArchived == 1
	repo.TreatAsPublic = treatAsPublic == 1
	repo.RequireSpecApproval, repo.RequireDesignApproval = requireSpec == 1, requireDesign == 1
	repo.AllowAgentMerge = allowAgentMerge == 1
	if checkedThrough.Valid {
		repo.HealthCheckedThroughTicket = &checkedThrough.Int64
	}
	if repo.HealthCheckedAt, err = parseOptionalTime(checkedAt); err != nil {
		return domain.Repo{}, err
	}
	if repo.SyncedAt, err = parseOptionalTime(synced); err != nil {
		return domain.Repo{}, err
	}
	if repo.MissingSince, err = parseOptionalTime(missing); err != nil {
		return domain.Repo{}, err
	}
	repo.CreatedAt, err = parseTime(created)
	return repo, err
}

func (r codeRepoRepo) NextMergeSequence(ctx context.Context, repoID int64) (int64, error) {
	var sequence int64
	err := r.q.QueryRowContext(ctx, `UPDATE repos SET merge_sequence = merge_sequence + 1 WHERE id = ? RETURNING merge_sequence`, repoID).Scan(&sequence)
	return sequence, err
}

func (r codeRepoRepo) SetBuildTargets(ctx context.Context, repo domain.Repo) error {
	targets, err := domain.NormalizeRepoBuildTargets(repo.BuildTargets)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(targets)
	if err != nil {
		return err
	}
	_, err = r.q.ExecContext(ctx, `UPDATE repos SET build_targets = ? WHERE id = ?`, string(raw), repo.ID)
	return err
}

func (r codeRepoRepo) SetOverlapIgnore(ctx context.Context, repo domain.Repo) error {
	patterns, err := domain.NormalizeRepoOverlapIgnore(repo.OverlapIgnore)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(patterns)
	if err != nil {
		return err
	}
	_, err = r.q.ExecContext(ctx, `UPDATE repos SET overlap_ignore = ? WHERE id = ?`, string(raw), repo.ID)
	return err
}
