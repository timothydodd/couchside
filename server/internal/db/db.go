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
	"runtime"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/timothydodd/couchside/internal/diskfree"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// migrations is the set migrate applies; tests swap in extra ones.
var migrations fs.FS = migrationFS

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
	// 16 MB of page cache per connection (SQLite's default is 2 MB): the
	// library lists and scans reread the same pages. modernc's cache is Go
	// heap, so it counts against GOMEMLIMIT; 8 connections stay under 128 MB.
	q.Add("_pragma", "cache_size(-16384)")
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
	tightenFiles(path, path+"-wal", path+"-shm")
	return &DB{sql: s}, nil
}

// tightenFiles makes database files readable by the server's user only: they
// hold password hashes, session hashes and API keys. SQLite creates the
// -wal and -shm files with the main file's mode, so once is enough. Windows
// has no POSIX modes.
func tightenFiles(paths ...string) {
	if runtime.GOOS == "windows" {
		return
	}
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil && st.Mode().Perm() != 0o600 {
			if err := os.Chmod(p, 0o600); err != nil {
				slog.Warn("couldn't restrict a database file to Couchside's user", "file", p, "err", err)
			}
		}
	}
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
	need := diskfree.FileSizes(path, path+"-wal")
	if sp, err := diskfree.Of(dir); err == nil && sp.Free < need {
		slog.Warn("not enough free space for a copy of the database; upgrading it without one",
			"dir", dir, "free", diskfree.Human(sp.Free), "needed", diskfree.Human(need))
		return
	}
	if _, err := s.Exec(`VACUUM INTO ?`, dst); err != nil {
		slog.Warn("no backup before upgrading the database", "err", err)
		return
	}
	tightenFiles(dst)
	slog.Info("backed up the database before upgrading it", "file", dst)
}

func migrate(s *sql.DB, path string) error {
	if _, err := s.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at INTEGER NOT NULL DEFAULT (unixepoch()))`); err != nil {
		return err
	}
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	applied, err := appliedMigrations(s)
	if err != nil {
		return err
	}
	if err := checkMigrations(names, applied, filepath.Join(filepath.Dir(path), BackupDir)); err != nil {
		return err
	}
	backedUp := len(applied) == 0 // a new database has nothing to keep
	for _, name := range names {
		if applied[name] {
			continue
		}
		if !backedUp {
			backupBeforeUpgrade(s, path)
			backedUp = true
		}
		body, err := fs.ReadFile(migrations, "migrations/"+name)
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

// ErrNewerDatabase means the database was last migrated by a newer
// Couchside than this one, which would misread it.
type ErrNewerDatabase struct{ Newest, BackupDir string }

func (e *ErrNewerDatabase) Error() string {
	return fmt.Sprintf("this database was upgraded by a newer version of Couchside (migration %s), so this version can't use it. "+
		"Run the newer version, or stop Couchside and restore the copy saved before that upgrade "+
		"(couchside-upgrade-<time>.db in %s) with `couchside restore <file>`", e.Newest, e.BackupDir)
}

func appliedMigrations(s *sql.DB) (map[string]bool, error) {
	rows, err := s.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}

// checkMigrations refuses a database this build would misread: one with a
// migration it doesn't know (a newer Couchside ran on it), or one missing a
// migration older than another it has (the table was edited by hand).
// names must be sorted.
func checkMigrations(names []string, applied map[string]bool, backupDir string) error {
	known := make(map[string]bool, len(names))
	for _, n := range names {
		known[n] = true
	}
	var unknown []string
	newest := ""
	for v := range applied {
		if !known[v] {
			unknown = append(unknown, v)
		}
		newest = max(newest, v)
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return &ErrNewerDatabase{Newest: unknown[len(unknown)-1], BackupDir: backupDir}
	}
	var missing []string
	for _, n := range names {
		if n < newest && !applied[n] {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("the database has migration %s but not the earlier %s, so it was changed by hand; restore a backup from %s with `couchside restore <file>`",
			newest, strings.Join(missing, ", "), backupDir)
	}
	return nil
}

// Maintain is the database's daily housekeeping: PRAGMA optimize refreshes
// the planner's statistics where tables have changed, and a TRUNCATE
// checkpoint folds the write-ahead log back in and shrinks it, which long
// reads during a scan can stop the automatic checkpoints from doing.
func (d *DB) Maintain(ctx context.Context) error {
	if _, err := d.sql.ExecContext(ctx, `PRAGMA optimize`); err != nil {
		return err
	}
	var busy, logPages, done int
	if err := d.sql.QueryRowContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &logPages, &done); err != nil {
		return err
	}
	if busy != 0 {
		slog.Info("database checkpoint couldn't finish while a read was open; it runs again tomorrow", "pages", logPages, "done", done)
	}
	return nil
}

// MigrationReport lists, for diagnostics, the migrations this build has and
// the ones the database has had applied.
func (d *DB) MigrationReport(ctx context.Context) (embedded, applied []string, err error) {
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return nil, nil, err
	}
	for _, e := range entries {
		embedded = append(embedded, e.Name())
	}
	sort.Strings(embedded)
	set, err := appliedMigrations(d.sql)
	if err != nil {
		return embedded, nil, err
	}
	for v := range set {
		applied = append(applied, v)
	}
	sort.Strings(applied)
	return embedded, applied, nil
}

// Health is the live database's integrity check (up to 20 problems; "ok"
// when there are none) and its size in bytes.
func (d *DB) Health(ctx context.Context) (check string, size int64, err error) {
	rows, err := d.sql.QueryContext(ctx, `PRAGMA quick_check(20)`)
	if err != nil {
		return "", 0, err
	}
	var lines []string
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			rows.Close()
			return "", 0, err
		}
		lines = append(lines, l)
	}
	rows.Close()
	if err := d.sql.QueryRowContext(ctx, `SELECT page_count * page_size FROM pragma_page_count(), pragma_page_size()`).Scan(&size); err != nil {
		return strings.Join(lines, "\n"), 0, err
	}
	return strings.Join(lines, "\n"), size, nil
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
