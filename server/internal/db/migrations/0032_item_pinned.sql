-- A file put under another title by hand (Merge): the scan keeps it there,
-- whatever its name parses to.
ALTER TABLE files ADD COLUMN item_pinned INTEGER NOT NULL DEFAULT 0;
