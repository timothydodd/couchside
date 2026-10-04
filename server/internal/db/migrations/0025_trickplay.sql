-- Seek-bar preview thumbnails are made for a library's files when this is on.
-- Making them reads each file from start to end once, so it's a choice.
ALTER TABLE libraries ADD COLUMN trickplay INTEGER NOT NULL DEFAULT 0;
