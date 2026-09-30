-- Pinned (favorite) channels sort to the top of the guide.
ALTER TABLE channels ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0;
-- Signal readings from the tuner's last channel scan (0-100, NULL if unknown).
ALTER TABLE channels ADD COLUMN signal_strength INTEGER;
ALTER TABLE channels ADD COLUMN signal_quality INTEGER;
