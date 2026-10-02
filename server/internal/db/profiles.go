package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Profile is one person using Couchside: profiles keep watch history,
// favourite channels and preferences apart. With accounts on (COUCHSIDE_AUTH),
// a profile is also the user that signs in; the account fields mean nothing
// otherwise.
type Profile struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	Color     string          `json:"color"`
	Prefs     json.RawMessage `json:"prefs"` // a JSON object owned by the UI
	CreatedAt int64           `json:"createdAt"`

	Role               string `json:"role"` // admin | user
	CanRecord          bool   `json:"canRecord"`
	Disabled           bool   `json:"disabled"`
	MustChangePassword bool   `json:"mustChangePassword"`
	HasPassword        bool   `json:"hasPassword"`
	// PasswordLocked: only an admin can set or change this account's password.
	PasswordLocked bool `json:"passwordLocked"`
}

var (
	ErrProfileName   = errors.New("that name is already taken")
	ErrLastProfile   = errors.New("the last profile can't be deleted")
	ErrPrefsNotAnObj = errors.New("preferences must be a JSON object")
	ErrLastAdmin     = errors.New("the last admin can't be removed, demoted or disabled")
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

const profileCols = `id, name, color, prefs, created_at, role, can_record, disabled, must_change_password, password_hash <> '', password_locked`

func scanProfile(r interface{ Scan(...any) error }) (Profile, error) {
	var p Profile
	var prefs string
	err := r.Scan(&p.ID, &p.Name, &p.Color, &prefs, &p.CreatedAt, &p.Role, &p.CanRecord, &p.Disabled, &p.MustChangePassword,
		&p.HasPassword, &p.PasswordLocked)
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
	if err := lastAdminGuard(ctx, tx, id, "user", false); err != nil {
		return err
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
