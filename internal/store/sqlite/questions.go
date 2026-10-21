package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

type questionRepo struct {
	q querier
}

func (r questionRepo) ForTicket(ctx context.Context, ticketID int64) ([]domain.Question, error) {
	rows, err := r.q.QueryContext(ctx,
		`SELECT id, ticket_id, round, position, prompt, options, recommended, asked_by, asked_at,
			answer_choice, answer_text, answer_deferred, answered_by, answered_at
		 FROM questions WHERE ticket_id = ? ORDER BY round, position`, ticketID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: questions of ticket %d: %w", ticketID, err)
	}
	defer rows.Close()
	var questions []domain.Question
	for rows.Next() {
		question, err := scanQuestion(rows)
		if err != nil {
			return nil, err
		}
		questions = append(questions, question)
	}
	return questions, rows.Err()
}

func (r questionRepo) Insert(ctx context.Context, question *domain.Question) error {
	options, err := json.Marshal(question.Options)
	if err != nil {
		return fmt.Errorf("sqlite: encode options: %w", err)
	}
	result, err := r.q.ExecContext(ctx,
		`INSERT INTO questions (ticket_id, round, position, prompt, options, recommended, asked_by, asked_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		question.TicketID, question.Round, question.Position, question.Prompt, string(options),
		nullableInt(question.Recommended), question.AskedBy, formatTime(question.AskedAt))
	if err != nil {
		return fmt.Errorf("sqlite: insert question: %w", err)
	}
	question.ID, err = result.LastInsertId()
	return err
}

func (r questionRepo) SaveAnswer(ctx context.Context, question domain.Question) error {
	answer := question.Answer
	if answer == nil {
		return fmt.Errorf("sqlite: question %d has no answer to save", question.ID)
	}
	_, err := r.q.ExecContext(ctx,
		`UPDATE questions SET answer_choice = ?, answer_text = ?, answer_deferred = ?, answered_by = ?, answered_at = ?
		 WHERE id = ?`,
		nullableInt(answer.Choice), answer.Text, boolToInt(answer.IsDeferred), answer.By, formatTime(answer.At), question.ID)
	if err != nil {
		return fmt.Errorf("sqlite: save answer %d: %w", question.ID, err)
	}
	return nil
}

func (r questionRepo) OpenCounts(ctx context.Context) (map[int64]int, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT ticket_id, COUNT(*) FROM questions WHERE answered_at IS NULL GROUP BY ticket_id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open question counts: %w", err)
	}
	defer rows.Close()
	counts := map[int64]int{}
	for rows.Next() {
		var ticketID int64
		var count int
		if err := rows.Scan(&ticketID, &count); err != nil {
			return nil, err
		}
		counts[ticketID] = count
	}
	return counts, rows.Err()
}

func (r questionRepo) LatestRound(ctx context.Context, ticketID int64) (int, error) {
	var round int
	err := r.q.QueryRowContext(ctx, `SELECT COALESCE(MAX(round), 0) FROM questions WHERE ticket_id = ?`, ticketID).Scan(&round)
	if err != nil {
		return 0, fmt.Errorf("sqlite: latest round of ticket %d: %w", ticketID, err)
	}
	return round, nil
}

func scanQuestion(row scanner) (domain.Question, error) {
	var q domain.Question
	var options, askedAt, answerText, answeredBy string
	var recommended, choice sql.NullInt64
	var isDeferred int
	var answeredAt sql.NullString
	err := row.Scan(&q.ID, &q.TicketID, &q.Round, &q.Position, &q.Prompt, &options, &recommended, &q.AskedBy, &askedAt,
		&choice, &answerText, &isDeferred, &answeredBy, &answeredAt)
	if err != nil {
		return domain.Question{}, err
	}
	if err := json.Unmarshal([]byte(options), &q.Options); err != nil {
		return domain.Question{}, fmt.Errorf("sqlite: decode options of question %d: %w", q.ID, err)
	}
	q.Recommended = intPointer(recommended)
	if q.AskedAt, err = parseTime(askedAt); err != nil {
		return domain.Question{}, err
	}
	at, err := parseOptionalTime(answeredAt)
	if err != nil || at == nil {
		return q, err
	}
	q.Answer = &domain.Answer{Choice: intPointer(choice), Text: answerText, IsDeferred: isDeferred == 1, By: answeredBy, At: *at}
	return q, nil
}
