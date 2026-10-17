-- A shared profile (a demo server's guest, a household's "Everyone") can hide
-- its signed-in devices from itself: whoever is using it shouldn't see other
-- people's devices and addresses, or be able to sign them out. Admins still
-- see them in Accounts.
ALTER TABLE profiles ADD COLUMN hide_devices INTEGER NOT NULL DEFAULT 0;
