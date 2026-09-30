package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Profile is one person using Couchside. There are no passwords: profiles keep
// watch history, favourite channels and preferences apart.
type Profile struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	Color     string          `json:"color"`
	Prefs     json.RawMessage `json:"prefs"` // a JSON object owned by the UI
	CreatedAt int64           `json:"createdAt"`
}

var (
	ErrProfileName   = errors.New("that name is already taken")
	ErrLastProfile   = errors.New("the last profile can't be deleted")
	ErrPrefsNotAnObj = errors.New("preferences must be a JSON object")
)

type profileKey struct{}

// WithProfile scopes watch state and favourites in queries made with ctx.
func WithProfile(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, profileKey{}, id)
}

// ProfileID is the profile ctx is scoped to, or 0 (background work), which
// matches no watch state.
func ProfileID(ctx context.Context) int64 {
	id, _ := ctx.Value(profileKey{}).(int64)
	return id
}

// watchJoin is the join condition for the current profile's watch state on
// files f. The id is an int64, so formatting it into the SQL is safe, and it
// keeps callers' positional arguments unchanged.
func watchJoin(ctx context.Context) string {
	return fmt.Sprintf(" LEFT JOIN watch_state w ON w.file_id = f.id AND w.profile_id = %d ", ProfileID(ctx))
}

const profileCols = `id, name, color, prefs, created_at`

func scanProfile(r interface{ Scan(...any) error }) (Profile, error) {
	var p Profile
	var prefs string
	err := r.Scan(&p.ID, &p.Name, &p.Color, &prefs, &p.CreatedAt)
	p.Prefs = json.RawMessage(prefs)
	return p, err
}

// Profiles lists profiles, oldest first.
func (d *DB) Profiles(ctx context.Context) ([]Profile, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT `+profileCols+` FROM profiles ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Profile{}
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (d *DB) Profile(ctx context.Context, id int64) (Profile, error) {
	p, err := scanProfile(d.sql.QueryRowContext(ctx, `SELECT `+profileCols+` FROM profiles WHERE id = ?`, id))
	return p, notFound(err)
}

func (d *DB) CreateProfile(ctx context.Context, name, color string) (Profile, error) {
	p, err := scanProfile(d.sql.QueryRowContext(ctx, `INSERT INTO profiles (name, color) VALUES (?, ?) RETURNING `+profileCols,
		name, color))
	return p, uniqueName(err)
}

func (d *DB) UpdateProfile(ctx context.Context, id int64, name, color string) (Profile, error) {
	p, err := scanProfile(d.sql.QueryRowContext(ctx, `UPDATE profiles SET name = ?, color = ? WHERE id = ? RETURNING `+profileCols,
		name, color, id))
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, uniqueName(err)
}

// MergeProfilePrefs sets the given top-level preference keys (null removes one).
func (d *DB) MergeProfilePrefs(ctx context.Context, id int64, patch json.RawMessage) (Profile, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(patch, &obj); err != nil || obj == nil {
		return Profile{}, ErrPrefsNotAnObj
	}
	p, err := scanProfile(d.sql.QueryRowContext(ctx, `UPDATE profiles SET prefs = json_patch(prefs, ?) WHERE id = ? RETURNING `+profileCols,
		string(patch), id))
	return p, notFound(err)
}

// DeleteProfile removes a profile with its watch history and favourites.
func (d *DB) DeleteProfile(ctx context.Context, id int64) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles`).Scan(&n); err != nil {
		return err
	}
	if n <= 1 {
		return ErrLastProfile
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM profiles WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if k, _ := res.RowsAffected(); k == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func uniqueName(err error) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed: profiles.name") {
		return ErrProfileName
	}
	return err
}
