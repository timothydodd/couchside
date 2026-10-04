-- Two-step sign-in. totp_secret is set when someone starts enrolling and
-- totp_enabled once they've proved their app works. totp_step is the last
-- 30-second step a code was accepted for, so a code can't be used twice.
ALTER TABLE profiles ADD COLUMN totp_secret TEXT NOT NULL DEFAULT '';
ALTER TABLE profiles ADD COLUMN totp_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE profiles ADD COLUMN totp_step INTEGER NOT NULL DEFAULT 0;

-- One-use codes for when the authenticator is lost. Only their hashes.
CREATE TABLE totp_recovery (
  profile_id INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  hash       TEXT NOT NULL,
  PRIMARY KEY (profile_id, hash)
);
