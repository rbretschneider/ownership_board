package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// querier is what repositories need from either a *sql.DB or a *sql.Tx.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// timeFormat is fixed-width UTC, so stored times also sort correctly as text.
const timeFormat = "2006-01-02T15:04:05.000Z"

func formatTime(t time.Time) string {
	return t.UTC().Format(timeFormat)
}

func formatOptionalTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatTime(*t)
}

func parseTime(raw string) (time.Time, error) {
	parsed, err := time.Parse(timeFormat, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("sqlite: stored time %q: %w", raw, err)
	}
	return parsed, nil
}

func parseOptionalTime(raw sql.NullString) (*time.Time, error) {
	if !raw.Valid || raw.String == "" {
		return nil, nil
	}
	parsed, err := parseTime(raw.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

// notFoundOr maps a missing row to a domain not-found error.
func notFoundOr(err error, format string, args ...any) error {
	if errors.Is(err, sql.ErrNoRows) {
		return domain.NotFoundf(format, args...)
	}
	return err
}

// conflictOr maps a unique-constraint violation to a domain conflict.
func conflictOr(err error, format string, args ...any) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return domain.Conflictf(format, args...)
	}
	return err
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func intPointer(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	converted := int(value.Int64)
	return &converted
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
