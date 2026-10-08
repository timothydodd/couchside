package db

import (
	"context"
	"database/sql"
	"errors"
)

// MediaLocation is a folder, drive or network share media can be in.
// Secret is a share's password, sealed with netshare.Protect.
type MediaLocation struct {
	ID     int64
	Path   string
	User   string
	Secret []byte
}

func (d *DB) MediaLocations(ctx context.Context) ([]MediaLocation, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT id, path, username, secret FROM media_locations ORDER BY path COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MediaLocation
	for rows.Next() {
		var l MediaLocation
		if err := rows.Scan(&l.ID, &l.Path, &l.User, &l.Secret); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (d *DB) MediaLocation(ctx context.Context, id int64) (MediaLocation, error) {
	var l MediaLocation
	err := d.sql.QueryRowContext(ctx, `SELECT id, path, username, secret FROM media_locations WHERE id = ?`, id).
		Scan(&l.ID, &l.Path, &l.User, &l.Secret)
	if errors.Is(err, sql.ErrNoRows) {
		return l, ErrNotFound
	}
	return l, err
}

// AddMediaLocation adds one; a path that's already there is left as it is.
func (d *DB) AddMediaLocation(ctx context.Context, path, user string, secret []byte) error {
	_, err := d.sql.ExecContext(ctx, `INSERT INTO media_locations (path, username, secret) VALUES (?, ?, ?)
		ON CONFLICT (path) DO NOTHING`, path, user, secret)
	return err
}

// SetMediaLocationLogin changes a share's sign-in. A nil secret keeps the
// saved password; an empty one clears it.
func (d *DB) SetMediaLocationLogin(ctx context.Context, id int64, user string, secret []byte) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE media_locations SET username = ?, secret = COALESCE(?, secret) WHERE id = ?`, user, secret, id)
	return err
}

func (d *DB) DeleteMediaLocation(ctx context.Context, id int64) error {
	_, err := d.sql.ExecContext(ctx, `DELETE FROM media_locations WHERE id = ?`, id)
	return err
}
