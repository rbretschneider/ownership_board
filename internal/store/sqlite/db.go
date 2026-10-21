// Package sqlite persists the board in a single SQLite file.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"

	"github.com/golang-migrate/migrate/v4"
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"github.com/rbretschneider/ownership_board/internal/app"
)

//go:embed migrations/*.sql
var migrations embed.FS

// driverName is the database/sql name of the pure-Go SQLite driver.
const driverName = "sqlite"

// DB is the board's database. It implements app.Store.
type DB struct {
	db *sql.DB
}

// Open opens (creating if needed) the database file at path and applies every
// pending migration. A migration failure aborts, so a bad deploy stops loudly.
func Open(ctx context.Context, path string) (*DB, error) {
	if path == "" {
		return nil, errors.New("sqlite: database path is required")
	}
	if err := migrateFile(path); err != nil {
		return nil, err
	}
	db, err := sql.Open(driverName, dsn(path))
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %s: %w", path, err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: ping %s: %w", path, err)
	}
	return &DB{db: db}, nil
}

// Close closes the database.
func (d *DB) Close() error {
	return d.db.Close()
}

// Ping checks the database answers, for the health endpoint.
func (d *DB) Ping(ctx context.Context) error {
	var one int
	return d.db.QueryRowContext(ctx, "SELECT 1").Scan(&one)
}

// Read implements app.Store. Reads run outside a transaction: every connection
// opens write transactions with BEGIN IMMEDIATE, which would serialise readers.
func (d *DB) Read(ctx context.Context, fn func(app.Repos) error) error {
	return fn(repos{q: d.db})
}

// Write implements app.Store.
func (d *DB) Write(ctx context.Context, fn func(app.Repos) error) error {
	return d.inTx(ctx, func(tx *sql.Tx) error {
		return fn(repos{q: tx})
	})
}

// SQL exposes the handle for adapters in this package's siblings, such as the
// auth store and backups.
func (d *DB) SQL() *sql.DB {
	return d.db
}

func (d *DB) inTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin: %w", err)
	}
	if err := fn(tx); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("sqlite: rollback: %w", rollbackErr))
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit: %w", err)
	}
	return nil
}

// dsn turns on WAL (readers never block the writer), foreign keys, a busy
// timeout instead of instant SQLITE_BUSY, and BEGIN IMMEDIATE so a transaction
// takes the write lock up front rather than failing to upgrade mid-way.
func dsn(path string) string {
	query := url.Values{}
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "journal_mode(WAL)")
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "synchronous(NORMAL)")
	query.Add("_txlock", "immediate")
	return "file:" + path + "?" + query.Encode()
}

// migrateFile applies migrations on a dedicated connection that is closed
// before the application pool opens.
func migrateFile(path string) error {
	db, err := sql.Open(driverName, dsn(path))
	if err != nil {
		return fmt.Errorf("sqlite: open for migration: %w", err)
	}
	defer db.Close()
	source, err := iofs.New(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("sqlite: migration source: %w", err)
	}
	driver, err := migratesqlite.WithInstance(db, &migratesqlite.Config{})
	if err != nil {
		return fmt.Errorf("sqlite: migration driver: %w", err)
	}
	migrator, err := migrate.NewWithInstance("iofs", source, driverName, driver)
	if err != nil {
		return fmt.Errorf("sqlite: migrator: %w", err)
	}
	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("sqlite: migrate: %w", err)
	}
	return nil
}
