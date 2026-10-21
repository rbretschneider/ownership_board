// Package backup takes consistent copies of the SQLite database on a schedule
// and prunes old ones.
package backup

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/rbretschneider/ownership_board/internal/clock"
)

// Schedule settings.
const (
	backupEvery = 24 * time.Hour
	// checkEvery is how often the scheduler looks; a box that slept through a
	// backup catches up within this long of waking.
	checkEvery     = time.Hour
	fileTimeLayout = "20060102-150405"
	filePrefix     = "ownership-"
	fileSuffix     = ".db"
)

var backupName = regexp.MustCompile(`^ownership-\d{8}-\d{6}\.db$`)

// Backups writes VACUUM INTO snapshots: a consistent copy taken while the
// board keeps running, restorable by copying the file back.
type Backups struct {
	db     *sql.DB
	dir    string
	keep   int
	clock  clock.Clock
	logger *slog.Logger
}

// New builds the backup job. keep is how many snapshots to retain.
func New(db *sql.DB, dir string, keep int, clk clock.Clock, logger *slog.Logger) *Backups {
	return &Backups{db: db, dir: dir, keep: keep, clock: clk, logger: logger}
}

// Run backs up whenever the newest snapshot is a day old, until ctx ends.
func (b *Backups) Run(ctx context.Context) {
	ticker := time.NewTicker(checkEvery)
	defer ticker.Stop()
	for {
		if err := b.backUpIfDue(ctx); err != nil {
			b.logger.Error("database backup failed", "dir", b.dir, "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (b *Backups) backUpIfDue(ctx context.Context) error {
	files, err := b.snapshots()
	if err != nil {
		return err
	}
	if len(files) > 0 && b.clock.Now().Sub(files[len(files)-1].at) < backupEvery {
		return nil
	}
	if err := b.backUp(ctx); err != nil {
		return err
	}
	return b.prune()
}

func (b *Backups) backUp(ctx context.Context) error {
	if err := os.MkdirAll(b.dir, 0o750); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}
	path := filepath.Join(b.dir, filePrefix+b.clock.Now().Format(fileTimeLayout)+fileSuffix)
	if _, err := b.db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return fmt.Errorf("vacuum into %s: %w", path, err)
	}
	b.logger.Info("database backed up", "path", path)
	return nil
}

func (b *Backups) prune() error {
	files, err := b.snapshots()
	if err != nil {
		return err
	}
	for len(files) > b.keep {
		if err := os.Remove(files[0].path); err != nil {
			return fmt.Errorf("prune %s: %w", files[0].path, err)
		}
		files = files[1:]
	}
	return nil
}

type snapshot struct {
	path string
	at   time.Time
}

// snapshots lists this job's backup files, oldest first. Only names this job
// writes are considered, so pruning never touches anything else in the dir.
func (b *Backups) snapshots() ([]snapshot, error) {
	entries, err := os.ReadDir(b.dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read backup dir: %w", err)
	}
	var files []snapshot
	for _, entry := range entries {
		if entry.IsDir() || !backupName.MatchString(entry.Name()) {
			continue
		}
		stamp := entry.Name()[len(filePrefix) : len(entry.Name())-len(fileSuffix)]
		at, err := time.ParseInLocation(fileTimeLayout, stamp, b.clock.Now().Location())
		if err == nil {
			files = append(files, snapshot{path: filepath.Join(b.dir, entry.Name()), at: at})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].at.Before(files[j].at) })
	return files, nil
}
