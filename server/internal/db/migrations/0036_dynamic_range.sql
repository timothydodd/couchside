-- A file's dynamic range, from its video stream: 'dv' (Dolby Vision, with
-- dv_profile), 'hdr10', 'hlg' or '' (SDR). NULL until the file has been read
-- for it: files indexed before this are filled in by the scan's catch-up.
ALTER TABLE files ADD COLUMN dynamic_range TEXT;
ALTER TABLE files ADD COLUMN dv_profile INTEGER NOT NULL DEFAULT 0;
