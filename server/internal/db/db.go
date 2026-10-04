// Package db owns the SQLite database: connection, migrations and queries.
package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// ErrNotFound is returned by single-row lookups that match nothing.
var ErrNotFound = errors.New("not found")

type DB struct {
	sql *sql.DB
}

// Open opens (creating if needed) the database at path and applies any
// pending migrations. WAL + busy_timeout lets the API read while the worker
// writes; _txlock=immediate avoids deadlocking lock upgrades.
func Open(path string) (*DB, error) {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(10000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Set("_txlock", "immediate")
	s, err := sql.Open("sqlite", "file:"+path+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	s.SetMaxOpenConns(8)
	if err := migrate(s, path); err != nil {
		s.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &DB{sql: s}, nil
}

func (d *DB) Close() error { return d.sql.Close() }

func (d *DB) Ping(ctx context.Context) error { return d.sql.PingContext(ctx) }

// BackupDir is the folder, beside the database, that backups go in.
const BackupDir = "backups"

// UpgradeBackup is the kind (in its file name) of the copy made before migrations.
const UpgradeBackup = "upgrade"

// VacuumInto writes a consistent copy of the database to path, which mustn't
// exist. It can run while the server is in use.
func (d *DB) VacuumInto(ctx context.Context, path string) error {
	_, err := d.sql.ExecContext(ctx, `VACUUM INTO ?`, path)
	return err
}

// Check reports whether the file at path is a SQLite database that passes a
// quick integrity check, without changing it.
func Check(path string) error {
	s, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return err
	}
	defer s.Close()
	var res string
	if err := s.QueryRow(`PRAGMA quick_check`).Scan(&res); err != nil {
		return err
	}
	if res != "ok" {
		return errors.New(res)
	}
	return nil
}

// backupBeforeUpgrade copies a database that's about to be migrated to
// backups/couchside-upgrade-<time>.db, so a release that goes wrong can be
// undone with `couchside restore`. Failing to (a full disk) is logged, not
// fatal: refusing to start would be worse.
func backupBeforeUpgrade(s *sql.DB, path string) {
	dir := filepath.Join(filepath.Dir(path), BackupDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		slog.Warn("no backup before upgrading the database", "err", err)
		return
	}
	dst := filepath.Join(dir, fmt.Sprintf("couchside-%s-%s.db", UpgradeBackup, time.Now().Format("20060102-150405")))
	if _, err := s.Exec(`VACUUM INTO ?`, dst); err != nil {
		slog.Warn("no backup before upgrading the database", "err", err)
		return
	}
	slog.Info("backed up the database before upgrading it", "file", dst)
}

func migrate(s *sql.DB, path string) error {
	if _, err := s.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at INTEGER NOT NULL DEFAULT (unixepoch()))`); err != nil {
		return err
	}
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	var applied int
	if err := s.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&applied); err != nil {
		return err
	}
	backedUp := applied == 0 // a new database has nothing to keep
	for _, name := range names {
		var n int
		if err := s.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, name).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		if !backedUp {
			backupBeforeUpgrade(s, path)
			backedUp = true
		}
		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := s.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("%s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, name); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// notFoundOK maps "no rows" to a nil error for optional lookups.
func notFoundOK(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
