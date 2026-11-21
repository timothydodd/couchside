-- A failed job an admin has seen and set aside. It stays in Activity (so the
-- scan's catch-up still knows not to queue the same work again) but no longer
-- counts as a failure that needs someone. Running it again clears the mark.
ALTER TABLE jobs ADD COLUMN acknowledged INTEGER NOT NULL DEFAULT 0;
