package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

type workLinkRepo struct {
	q querier
}

const workLinkColumns = `id, ticket_id, kind, url, sha, repo_id, added_by_kind, added_by_id, added_by_name, added_at`

func (r workLinkRepo) ForTicket(ctx context.Context, ticketID int64) ([]domain.WorkLink, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT `+workLinkColumns+` FROM work_links WHERE ticket_id = ? ORDER BY id`, ticketID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: work links of ticket %d: %w", ticketID, err)
	}
	defer rows.Close()
	var links []domain.WorkLink
	for rows.Next() {
		link, err := scanWorkLink(rows)
		if err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

func (r workLinkRepo) Insert(ctx context.Context, link *domain.WorkLink) error {
	result, err := r.q.ExecContext(ctx,
		`INSERT INTO work_links (ticket_id, kind, url, sha, repo_id, added_by_kind, added_by_id, added_by_name, added_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		link.TicketID, string(link.Kind), link.URL, link.SHA, nullableID(link.RepoID),
		string(link.AddedBy.Kind), link.AddedBy.ID, link.AddedBy.Name, formatTime(link.AddedAt))
	if err != nil {
		return conflictOr(err, "that work link is already on the ticket")
	}
	link.ID, err = result.LastInsertId()
	return err
}

func (r workLinkRepo) Delete(ctx context.Context, ticketID, linkID int64) (domain.WorkLink, error) {
	link, err := scanWorkLink(r.q.QueryRowContext(ctx, `SELECT `+workLinkColumns+` FROM work_links WHERE id = ? AND ticket_id = ?`, linkID, ticketID))
	if err != nil {
		return domain.WorkLink{}, notFoundOr(err, "work link %d is not on ticket %d", linkID, ticketID)
	}
	if _, err := r.q.ExecContext(ctx, `DELETE FROM work_links WHERE id = ?`, linkID); err != nil {
		return domain.WorkLink{}, fmt.Errorf("sqlite: delete work link %d: %w", linkID, err)
	}
	return link, nil
}

func (r workLinkRepo) CountByAgents(ctx context.Context, ticketID int64) (int, error) {
	return countRows(ctx, r.q, `SELECT COUNT(*) FROM work_links WHERE ticket_id = ? AND added_by_kind = 'agent'`, ticketID)
}

// TicketsFor matches shas by prefix in either direction (a short query finds a
// full stored sha, a full query finds a short stored one), with substr rather
// than LIKE so no input character can act as a wildcard.
func (r workLinkRepo) TicketsFor(ctx context.Context, shaPrefix, prURL string, limit int) ([]int64, error) {
	rows, err := r.q.QueryContext(ctx,
		`SELECT DISTINCT ticket_id FROM work_links
		 WHERE (? <> '' AND kind = 'commit' AND (substr(sha, 1, length(?)) = ? OR substr(?, 1, length(sha)) = sha))
		    OR (? <> '' AND kind = 'pr' AND url = ?)
		 ORDER BY ticket_id LIMIT ?`,
		shaPrefix, shaPrefix, shaPrefix, shaPrefix, prURL, prURL, limit)
	if err != nil {
		return nil, fmt.Errorf("sqlite: work lookup: %w", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func scanWorkLink(row scanner) (domain.WorkLink, error) {
	var link domain.WorkLink
	var kind, addedByKind, addedAt string
	var repoID sql.NullInt64
	err := row.Scan(&link.ID, &link.TicketID, &kind, &link.URL, &link.SHA, &repoID,
		&addedByKind, &link.AddedBy.ID, &link.AddedBy.Name, &addedAt)
	if err != nil {
		return domain.WorkLink{}, err
	}
	link.Kind, link.RepoID, link.AddedBy.Kind = domain.WorkLinkKind(kind), repoID.Int64, domain.ActorKind(addedByKind)
	link.AddedAt, err = parseTime(addedAt)
	return link, err
}

type decisionRepo struct {
	q querier
}

const decisionColumns = `id, ticket_id, summary, rationale, alternatives, source, weight, decided_by_kind, decided_by_id,
	decided_by_name, asked_by, question_id, supersedes_id, superseded_by_id, is_removed, created_at`

func (r decisionRepo) ForTicket(ctx context.Context, ticketID int64) ([]domain.Decision, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT `+decisionColumns+` FROM decisions WHERE ticket_id = ? ORDER BY id`, ticketID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: decisions of ticket %d: %w", ticketID, err)
	}
	defer rows.Close()
	var decisions []domain.Decision
	for rows.Next() {
		decision, err := scanDecision(rows)
		if err != nil {
			return nil, err
		}
		decisions = append(decisions, decision)
	}
	return decisions, rows.Err()
}

func (r decisionRepo) Get(ctx context.Context, ticketID, id int64) (domain.Decision, error) {
	decision, err := scanDecision(r.q.QueryRowContext(ctx, `SELECT `+decisionColumns+` FROM decisions WHERE id = ? AND ticket_id = ?`, id, ticketID))
	return decision, notFoundOr(err, "decision %d is not on ticket %d", id, ticketID)
}

func (r decisionRepo) Insert(ctx context.Context, d *domain.Decision) error {
	alternatives, err := json.Marshal(nonNilStrings(d.Alternatives))
	if err != nil {
		return fmt.Errorf("sqlite: encode alternatives: %w", err)
	}
	result, err := r.q.ExecContext(ctx,
		`INSERT INTO decisions (ticket_id, summary, rationale, alternatives, source, weight, decided_by_kind, decided_by_id,
			decided_by_name, asked_by, question_id, supersedes_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.TicketID, d.Summary, d.Rationale, string(alternatives), string(d.Source), string(d.Weight),
		string(d.DecidedBy.Kind), d.DecidedBy.ID, d.DecidedBy.Name, d.AskedBy, nullableID64(d.QuestionID), nullableID64(d.SupersedesID),
		formatTime(d.At))
	if err != nil {
		return fmt.Errorf("sqlite: insert decision: %w", err)
	}
	d.ID, err = result.LastInsertId()
	return err
}

func (r decisionRepo) SetSupersededBy(ctx context.Context, id int64, by *int64) error {
	if _, err := r.q.ExecContext(ctx, `UPDATE decisions SET superseded_by_id = ? WHERE id = ?`, nullableID64(by), id); err != nil {
		return fmt.Errorf("sqlite: link decision %d: %w", id, err)
	}
	return nil
}

// Tombstone also blanks the decision's text in the timeline entry that
// recorded it, so a removed decision leaves no copy behind.
func (r decisionRepo) Tombstone(ctx context.Context, id int64) error {
	_, err := r.q.ExecContext(ctx,
		`UPDATE decisions SET summary = '', rationale = '', alternatives = '[]', is_removed = 1,
			supersedes_id = NULL, superseded_by_id = NULL WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("sqlite: remove decision %d: %w", id, err)
	}
	_, err = r.q.ExecContext(ctx,
		`UPDATE timeline SET body = '' WHERE type = 'decision' AND json_extract(data, '$.decision_id') = ?`, id)
	if err != nil {
		return fmt.Errorf("sqlite: blank timeline for decision %d: %w", id, err)
	}
	return nil
}

func (r decisionRepo) SetSupersedes(ctx context.Context, id int64, previous *int64) error {
	if _, err := r.q.ExecContext(ctx, `UPDATE decisions SET supersedes_id = ? WHERE id = ?`, nullableID64(previous), id); err != nil {
		return fmt.Errorf("sqlite: link decision %d: %w", id, err)
	}
	return nil
}

func (r decisionRepo) CountByAgents(ctx context.Context, ticketID int64) (int, error) {
	return countRows(ctx, r.q, `SELECT COUNT(*) FROM decisions WHERE ticket_id = ? AND source = 'agent'`, ticketID)
}

func scanDecision(row scanner) (domain.Decision, error) {
	var d domain.Decision
	var alternatives, source, weight, kind, at string
	var questionID, supersedes, supersededBy sql.NullInt64
	var isRemoved int
	err := row.Scan(&d.ID, &d.TicketID, &d.Summary, &d.Rationale, &alternatives, &source, &weight, &kind, &d.DecidedBy.ID,
		&d.DecidedBy.Name, &d.AskedBy, &questionID, &supersedes, &supersededBy, &isRemoved, &at)
	if err != nil {
		return domain.Decision{}, err
	}
	if err := json.Unmarshal([]byte(alternatives), &d.Alternatives); err != nil {
		return domain.Decision{}, fmt.Errorf("sqlite: decode alternatives of decision %d: %w", d.ID, err)
	}
	d.Source, d.Weight, d.DecidedBy.Kind = domain.DecisionSource(source), domain.DecisionWeight(weight), domain.ActorKind(kind)
	d.QuestionID, d.SupersedesID, d.SupersededByID = id64Pointer(questionID), id64Pointer(supersedes), id64Pointer(supersededBy)
	d.IsRemoved = isRemoved == 1
	d.At, err = parseTime(at)
	return d, err
}

type relationRepo struct {
	q querier
}

const relationColumns = `from_id, to_id, kind, added_by_kind, added_by_id, added_by_name, added_at`

func (r relationRepo) ForTicket(ctx context.Context, ticketID int64) ([]domain.Relation, error) {
	rows, err := r.q.QueryContext(ctx,
		`SELECT `+relationColumns+` FROM relations WHERE from_id = ? OR to_id = ? ORDER BY added_at`, ticketID, ticketID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: relations of ticket %d: %w", ticketID, err)
	}
	defer rows.Close()
	var relations []domain.Relation
	for rows.Next() {
		var relation domain.Relation
		var kind, addedByKind, addedAt string
		if err := rows.Scan(&relation.FromID, &relation.ToID, &kind, &addedByKind, &relation.AddedBy.ID, &relation.AddedBy.Name, &addedAt); err != nil {
			return nil, err
		}
		relation.Kind, relation.AddedBy.Kind = domain.RelationKind(kind), domain.ActorKind(addedByKind)
		if relation.AddedAt, err = parseTime(addedAt); err != nil {
			return nil, err
		}
		relations = append(relations, relation)
	}
	return relations, rows.Err()
}

func (r relationRepo) Insert(ctx context.Context, relation domain.Relation) error {
	_, err := r.q.ExecContext(ctx,
		`INSERT INTO relations (`+relationColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		relation.FromID, relation.ToID, string(relation.Kind), string(relation.AddedBy.Kind), relation.AddedBy.ID,
		relation.AddedBy.Name, formatTime(relation.AddedAt))
	return conflictOr(err, "ticket %d is already linked to ticket %d that way", relation.FromID, relation.ToID)
}

func (r relationRepo) Delete(ctx context.Context, relation domain.Relation) error {
	_, err := r.q.ExecContext(ctx, `DELETE FROM relations WHERE from_id = ? AND to_id = ? AND kind = ?`,
		relation.FromID, relation.ToID, string(relation.Kind))
	if err != nil {
		return fmt.Errorf("sqlite: delete relation: %w", err)
	}
	return nil
}

func (r relationRepo) CountOutgoing(ctx context.Context, ticketID int64) (int, error) {
	return countRows(ctx, r.q, `SELECT COUNT(*) FROM relations WHERE from_id = ?`, ticketID)
}

// waitingQuery lists depends_on links whose target is not merged yet.
const waitingQuery = `SELECT r.from_id, r.to_id FROM relations r JOIN tickets dep ON dep.id = r.to_id
	WHERE r.kind = 'depends_on' AND dep.stage <> 'done'`

func (r relationRepo) Waiting(ctx context.Context) (map[int64][]int64, error) {
	return r.edges(ctx, waitingQuery+` ORDER BY r.from_id, r.to_id`)
}

func (r relationRepo) WaitingOn(ctx context.Context, ticketID int64) ([]int64, error) {
	edges, err := r.edges(ctx, waitingQuery+` AND r.from_id = ? ORDER BY r.to_id`, ticketID)
	return edges[ticketID], err
}

func (r relationRepo) DependencyEdges(ctx context.Context) (map[int64][]int64, error) {
	return r.edges(ctx, `SELECT from_id, to_id FROM relations WHERE kind = 'depends_on' ORDER BY from_id, to_id`)
}

// edges reads (from_id, to_id) rows into a map from each from_id to its
// to_ids.
func (r relationRepo) edges(ctx context.Context, query string, args ...any) (map[int64][]int64, error) {
	rows, err := r.q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: dependencies: %w", err)
	}
	defer rows.Close()
	edges := map[int64][]int64{}
	for rows.Next() {
		var from, to int64
		if err := rows.Scan(&from, &to); err != nil {
			return nil, err
		}
		edges[from] = append(edges[from], to)
	}
	return edges, rows.Err()
}

func countRows(ctx context.Context, q querier, query string, args ...any) (int, error) {
	var count int
	if err := q.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("sqlite: count: %w", err)
	}
	return count, nil
}

func nullableID64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func id64Pointer(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	v := value.Int64
	return &v
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
