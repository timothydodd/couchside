-- An admin can stop an account changing its own password (e.g. a shared Guest
-- profile that anyone may pick, which a visitor could otherwise lock).
ALTER TABLE profiles ADD COLUMN password_locked INTEGER NOT NULL DEFAULT 0;
