-- Accounts are always on now; servers that ran without them become
-- passwordless (pick a profile to sign in). Someone has to be able to manage
-- the server: the oldest profile becomes admin if there's no admin yet. On a
-- server where nobody had a password, everyone could record before, so they
-- still can.
UPDATE profiles SET role = 'admin', can_record = 1
  WHERE id = (SELECT MIN(id) FROM profiles)
    AND NOT EXISTS (SELECT 1 FROM profiles WHERE role = 'admin' AND disabled = 0);
UPDATE profiles SET can_record = 1
  WHERE NOT EXISTS (SELECT 1 FROM profiles WHERE password_hash <> '');
