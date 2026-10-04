package db

import "context"

// TOTP returns a profile's two-step secret and whether it's switched on.
func (d *DB) TOTP(ctx context.Context, id int64) (secret string, enabled bool, err error) {
	err = d.sql.QueryRowContext(ctx, `SELECT totp_secret, totp_enabled FROM profiles WHERE id = ?`, id).Scan(&secret, &enabled)
	return secret, enabled, notFound(err)
}

// StartTOTP stores a new secret for a profile that's enrolling. It isn't
// asked for at sign-in until EnableTOTP.
func (d *DB) StartTOTP(ctx context.Context, id int64, secret string) error {
	res, err := d.sql.ExecContext(ctx, `UPDATE profiles SET totp_secret = ?, totp_step = 0 WHERE id = ? AND totp_enabled = 0`, secret, id)
	return affected(res, err)
}

// EnableTOTP switches two-step sign-in on and stores the recovery codes'
// hashes, replacing any from before.
func (d *DB) EnableTOTP(ctx context.Context, id int64, recoveryHashes []string) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE profiles SET totp_enabled = 1 WHERE id = ? AND totp_secret <> ''`, id)
	if err := affected(res, err); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM totp_recovery WHERE profile_id = ?`, id); err != nil {
		return err
	}
	for _, h := range recoveryHashes {
		if _, err := tx.ExecContext(ctx, `INSERT INTO totp_recovery (profile_id, hash) VALUES (?, ?)`, id, h); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DisableTOTP switches two-step sign-in off and forgets the secret and
// recovery codes.
func (d *DB) DisableTOTP(ctx context.Context, id int64) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE profiles SET totp_secret = '', totp_enabled = 0, totp_step = 0 WHERE id = ?`, id)
	if err := affected(res, err); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM totp_recovery WHERE profile_id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// UseTOTPStep records that a code for step was accepted. It reports false
// when a code for that step (or a later one) already was: a replay.
func (d *DB) UseTOTPStep(ctx context.Context, id, step int64) (bool, error) {
	res, err := d.sql.ExecContext(ctx, `UPDATE profiles SET totp_step = ? WHERE id = ? AND totp_step < ?`, step, id, step)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// UseRecoveryCode spends a recovery code, reporting whether it was one.
func (d *DB) UseRecoveryCode(ctx context.Context, id int64, hash string) (bool, error) {
	res, err := d.sql.ExecContext(ctx, `DELETE FROM totp_recovery WHERE profile_id = ? AND hash = ?`, id, hash)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// RecoveryCodesLeft counts a profile's unused recovery codes.
func (d *DB) RecoveryCodesLeft(ctx context.Context, id int64) (int, error) {
	var n int
	err := d.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM totp_recovery WHERE profile_id = ?`, id).Scan(&n)
	return n, err
}
