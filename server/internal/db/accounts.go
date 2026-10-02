package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// --- accounts --------------------------------------------------------------------

// ProfileForLogin finds a profile by name (any case) with its password hash.
func (d *DB) ProfileForLogin(ctx context.Context, name string) (Profile, string, error) {
	var hash string
	p, err := scanProfile(scanExtra(d.sql.QueryRowContext(ctx, `SELECT `+profileCols+`, password_hash FROM profiles
		WHERE name = ?`, strings.TrimSpace(name)), &hash))
	return p, hash, notFound(err)
}

// PasswordHash returns a profile's stored hash ("" when it has none).
func (d *DB) PasswordHash(ctx context.Context, id int64) (string, error) {
	var h string
	err := d.sql.QueryRowContext(ctx, `SELECT password_hash FROM profiles WHERE id = ?`, id).Scan(&h)
	return h, notFound(err)
}

// SetPassword stores a new hash. mustChange makes it a temporary password the
// user has to replace at their next sign-in.
func (d *DB) SetPassword(ctx context.Context, id int64, hash string, mustChange bool) error {
	res, err := d.sql.ExecContext(ctx, `UPDATE profiles SET password_hash = ?, must_change_password = ? WHERE id = ?`,
		hash, mustChange, id)
	return affected(res, err)
}

// SetPasswordLocked sets whether a profile may change its own password.
// Locking also drops "must change password": the account couldn't.
func (d *DB) SetPasswordLocked(ctx context.Context, id int64, locked bool) error {
	res, err := d.sql.ExecContext(ctx, `UPDATE profiles SET password_locked = ?,
		must_change_password = CASE WHEN ? THEN 0 ELSE must_change_password END WHERE id = ?`, locked, locked, id)
	return affected(res, err)
}

// SetAccess changes what a profile may do. The last enabled admin can't be
// demoted or disabled.
func (d *DB) SetAccess(ctx context.Context, id int64, role string, canRecord, disabled bool) (Profile, error) {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return Profile{}, err
	}
	defer tx.Rollback()
	if err := lastAdminGuard(ctx, tx, id, role, disabled); err != nil {
		return Profile{}, err
	}
	p, err := scanProfile(tx.QueryRowContext(ctx, `UPDATE profiles SET role = ?, can_record = ?, disabled = ? WHERE id = ?
		RETURNING `+profileCols, role, canRecord, disabled, id))
	if err != nil {
		return p, notFound(err)
	}
	return p, tx.Commit()
}

// lastAdminGuard refuses a change that would leave no enabled admin: profile
// id becoming role/disabled (deleting it counts as becoming a disabled user).
func lastAdminGuard(ctx context.Context, tx *sql.Tx, id int64, role string, disabled bool) error {
	var wasAdmin bool
	err := tx.QueryRowContext(ctx, `SELECT role = 'admin' AND disabled = 0 FROM profiles WHERE id = ?`, id).Scan(&wasAdmin)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil || !wasAdmin || (role == "admin" && !disabled) {
		return err
	}
	var others int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles WHERE role = 'admin' AND disabled = 0 AND id <> ?`, id).
		Scan(&others); err != nil {
		return err
	}
	if others == 0 {
		return ErrLastAdmin
	}
	return nil
}

// HasAdmin reports whether an enabled admin with a password exists. Without
// one, accounts mode asks for the one-time setup code.
func (d *DB) HasAdmin(ctx context.Context) (bool, error) {
	var n int
	err := d.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles WHERE role = 'admin' AND disabled = 0 AND password_hash <> ''`).
		Scan(&n)
	return n > 0, err
}

// SetupAdmin makes the named profile (created if it doesn't exist, so an
// existing "Me" keeps its history) an admin with this password.
func (d *DB) SetupAdmin(ctx context.Context, name, hash string) (Profile, error) {
	p, err := scanProfile(d.sql.QueryRowContext(ctx, `INSERT INTO profiles (name, password_hash, role, can_record)
		VALUES (?, ?, 'admin', 1)
		ON CONFLICT (name) DO UPDATE SET password_hash = excluded.password_hash, role = 'admin', can_record = 1,
		  disabled = 0, must_change_password = 0
		RETURNING `+profileCols, name, hash))
	return p, uniqueName(err)
}

// CreateAccount adds a profile, with a temporary password or (passwordless sign-in) none.
func (d *DB) CreateAccount(ctx context.Context, name, color, role string, canRecord bool, hash string) (Profile, error) {
	// A temporary password must be replaced at first sign-in; no password means passwordless.
	p, err := scanProfile(d.sql.QueryRowContext(ctx, `INSERT INTO profiles (name, color, role, can_record, password_hash,
		must_change_password) VALUES (?, ?, ?, ?, ?, ?) RETURNING `+profileCols, name, color, role, canRecord, hash, hash != ""))
	return p, uniqueName(err)
}

// --- sessions --------------------------------------------------------------------

// Session is one signed-in device.
type Session struct {
	ID         string `json:"id"`
	ProfileID  int64  `json:"profileId"`
	Client     string `json:"client"` // web | tv
	Device     string `json:"device"`
	UserAgent  string `json:"userAgent"`
	IP         string `json:"ip"`
	CreatedAt  int64  `json:"createdAt"`
	LastUsedAt int64  `json:"lastUsedAt"`
	ExpiresAt  int64  `json:"expiresAt"`
}

const sessionCols = `id, profile_id, client, device, user_agent, ip, created_at, last_used_at, expires_at`

func scanSession(r interface{ Scan(...any) error }) (Session, error) {
	var s Session
	err := r.Scan(&s.ID, &s.ProfileID, &s.Client, &s.Device, &s.UserAgent, &s.IP, &s.CreatedAt, &s.LastUsedAt, &s.ExpiresAt)
	return s, err
}

func (d *DB) CreateSession(ctx context.Context, s Session, refreshHash string) error {
	_, err := d.sql.ExecContext(ctx, `INSERT INTO sessions (id, profile_id, refresh_hash, client, device, user_agent, ip,
		created_at, last_used_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, unixepoch(), unixepoch(), ?)`,
		s.ID, s.ProfileID, refreshHash, s.Client, s.Device, s.UserAgent, s.IP, s.ExpiresAt)
	return err
}

// SessionUser is a live session with what its profile may do, checked on
// every authenticated request.
type SessionUser struct {
	Session   string
	ExpiresAt int64
	Profile   Profile
}

// SessionUser looks up a session that hasn't expired, for an enabled profile.
func (d *DB) SessionUser(ctx context.Context, id string, now int64) (SessionUser, error) {
	u := SessionUser{Session: id}
	p, err := scanProfile(scanExtra(d.sql.QueryRowContext(ctx, `SELECT `+prefixed("p.", profileCols)+`, s.expires_at
		FROM sessions s JOIN profiles p ON p.id = s.profile_id
		WHERE s.id = ? AND s.expires_at > ? AND p.disabled = 0`, id, now), &u.ExpiresAt))
	u.Profile = p
	return u, notFound(err)
}

// prefixed qualifies a column list with a table alias.
func prefixed(alias, cols string) string {
	parts := strings.Split(cols, ",")
	for i, c := range parts {
		parts[i] = alias + strings.TrimSpace(c)
	}
	return strings.Join(parts, ", ")
}

// RefreshResult says what happened to a refresh token.
type RefreshResult int

const (
	RefreshOK      RefreshResult = iota
	RefreshUnknown               // never issued, or its session is gone
	RefreshStale                 // replaced moments ago (another tab refreshed first); not a theft
	RefreshReused                // an old token came back: assume it was stolen, and end the session
)

// staleGrace is how long a just-replaced refresh token is treated as a race
// between tabs rather than a replay.
const staleGrace = 60

// RotateRefresh swaps a refresh token for a new one and extends the session
// by idle(client) seconds from now. Presenting a token that was already
// swapped ends the session, unless it was swapped in the last minute.
func (d *DB) RotateRefresh(ctx context.Context, oldHash, newHash string, now int64, idle func(client string) int64, ip string) (Session, RefreshResult, error) {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return Session{}, RefreshUnknown, err
	}
	defer tx.Rollback()
	s, err := scanSession(tx.QueryRowContext(ctx, `SELECT `+prefixed("s.", sessionCols)+`
		FROM sessions s JOIN profiles p ON p.id = s.profile_id
		WHERE s.refresh_hash = ? AND s.expires_at > ? AND p.disabled = 0`, oldHash, now))
	if errors.Is(err, sql.ErrNoRows) {
		var sid string
		var usedAt int64
		err := tx.QueryRowContext(ctx, `SELECT session_id, used_at FROM session_used_tokens WHERE hash = ?`, oldHash).
			Scan(&sid, &usedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return Session{}, RefreshUnknown, nil
		}
		if err != nil {
			return Session{}, RefreshUnknown, err
		}
		if now-usedAt <= staleGrace {
			return Session{}, RefreshStale, nil
		}
		s, _ := scanSession(tx.QueryRowContext(ctx, `SELECT `+sessionCols+` FROM sessions WHERE id = ?`, sid))
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, sid); err != nil {
			return Session{}, RefreshUnknown, err
		}
		return s, RefreshReused, tx.Commit()
	}
	if err != nil {
		return s, RefreshUnknown, err
	}
	expiresAt := now + idle(s.Client)
	if _, err := tx.ExecContext(ctx, `INSERT INTO session_used_tokens (hash, session_id, used_at) VALUES (?, ?, ?)`,
		oldHash, s.ID, now); err != nil {
		return s, RefreshUnknown, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET refresh_hash = ?, last_used_at = ?, expires_at = ?, ip = ? WHERE id = ?`,
		newHash, now, expiresAt, ip, s.ID); err != nil {
		return s, RefreshUnknown, err
	}
	s.LastUsedAt, s.ExpiresAt, s.IP = now, expiresAt, ip
	return s, RefreshOK, tx.Commit()
}

// Sessions lists a profile's live sessions, most recently used first.
func (d *DB) Sessions(ctx context.Context, profileID, now int64) ([]Session, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT `+sessionCols+` FROM sessions WHERE profile_id = ? AND expires_at > ?
		ORDER BY last_used_at DESC`, profileID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// PeekRefresh finds the live session a refresh token belongs to, without
// using it up: it tells the web which profiles this browser is signed in to.
func (d *DB) PeekRefresh(ctx context.Context, hash string, now int64) (Profile, error) {
	p, err := scanProfile(d.sql.QueryRowContext(ctx, `SELECT `+prefixed("p.", profileCols)+`
		FROM sessions s JOIN profiles p ON p.id = s.profile_id
		WHERE s.refresh_hash = ? AND s.expires_at > ? AND p.disabled = 0`, hash, now))
	return p, notFound(err)
}

func (d *DB) Session(ctx context.Context, id string) (Session, error) {
	s, err := scanSession(d.sql.QueryRowContext(ctx, `SELECT `+sessionCols+` FROM sessions WHERE id = ?`, id))
	return s, notFound(err)
}

func (d *DB) DeleteSession(ctx context.Context, id string) error {
	_, err := d.sql.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// DeleteSessions signs a profile out everywhere except keep ("" for everywhere).
func (d *DB) DeleteSessions(ctx context.Context, profileID int64, keep string) error {
	_, err := d.sql.ExecContext(ctx, `DELETE FROM sessions WHERE profile_id = ? AND id <> ?`, profileID, keep)
	return err
}

// DeleteSessionsWithoutPassword signs out every profile that has no
// password: once passwordless sign-in is off, they couldn't sign in again.
func (d *DB) DeleteSessionsWithoutPassword(ctx context.Context) (int64, error) {
	res, err := d.sql.ExecContext(ctx, `DELETE FROM sessions WHERE profile_id IN (SELECT id FROM profiles WHERE password_hash = '')`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// TrimSessions keeps a profile's newest keep sessions and deletes the rest.
func (d *DB) TrimSessions(ctx context.Context, profileID int64, keep int) error {
	_, err := d.sql.ExecContext(ctx, `DELETE FROM sessions WHERE profile_id = ? AND id NOT IN
		(SELECT id FROM sessions WHERE profile_id = ? ORDER BY last_used_at DESC, created_at DESC LIMIT ?)`, profileID, profileID, keep)
	return err
}

// PruneSessions drops expired sessions (and with them their used tokens).
func (d *DB) PruneSessions(ctx context.Context, now int64) error {
	_, err := d.sql.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now)
	return err
}

func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- ownership of recordings and rules -------------------------------------------

// SetRecordingOwner records who scheduled a recording.
func (d *DB) SetRecordingOwner(ctx context.Context, id, profileID int64) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE recordings SET profile_id = ? WHERE id = ? AND profile_id IS NULL`, profileID, id)
	return err
}

// SetRuleOwner records who made a series rule (the first one to, if it existed).
func (d *DB) SetRuleOwner(ctx context.Context, id, profileID int64) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE series_rules SET profile_id = ? WHERE id = ? AND profile_id IS NULL`, profileID, id)
	return err
}

// RuleOwnerForSeries returns whether a series already has a rule, and who owns it.
func (d *DB) RuleOwnerForSeries(ctx context.Context, seriesID string) (bool, int64, error) {
	var owner int64
	err := d.sql.QueryRowContext(ctx, `SELECT COALESCE(profile_id, 0) FROM series_rules WHERE series_id = ?`, seriesID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return false, 0, nil
	}
	return err == nil, owner, err
}

// AnyPassword reports whether any profile has a password set.
func (d *DB) AnyPassword(ctx context.Context) (bool, error) {
	var n int
	err := d.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles WHERE password_hash <> ''`).Scan(&n)
	return n > 0, err
}
