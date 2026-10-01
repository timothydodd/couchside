package db

import (
	"context"
	"database/sql"
)

// Setting returns a UI setting, or "" when it was never set.
func (d *DB) Setting(ctx context.Context, key string) (string, error) {
	var v string
	err := d.sql.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

func (d *DB) SetSetting(ctx context.Context, key, value string) error {
	_, err := d.sql.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = unixepoch()`, key, value)
	return err
}

func (d *DB) SetRecordingPath(ctx context.Context, id int64, path string) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE recordings SET path = ? WHERE id = ?`, path, id)
	return err
}

// LibraryContaining returns the library whose folder contains path (or is it).
func (d *DB) LibraryContaining(ctx context.Context, path string) (*Library, error) {
	libs, err := d.Libraries(ctx)
	if err != nil {
		return nil, err
	}
	var best *Library
	for i := range libs {
		l := libs[i]
		if path == l.Path || (len(path) > len(l.Path) && path[:len(l.Path)] == l.Path && path[len(l.Path)] == '/') {
			if best == nil || len(l.Path) > len(best.Path) {
				best = &l
			}
		}
	}
	return best, nil
}

// SetScheduledPadding applies new padding to recordings that haven't started.
func (d *DB) SetScheduledPadding(ctx context.Context, before, after int64) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE recordings SET pad_before = ?, pad_after = ? WHERE status = 'scheduled'`, before, after)
	return err
}
