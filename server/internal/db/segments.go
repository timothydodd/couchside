package db

import "context"

// MarkedSegment is a file's intro or end credits.
type MarkedSegment struct {
	Kind   string  `json:"kind"`   // intro | credits
	Start  float64 `json:"start"`  // seconds into the file
	End    float64 `json:"end"`    //
	Source string  `json:"source"` // chapters | detected | manual
}

// FileSegments lists a file's intro and credits, in that order.
func (d *DB) FileSegments(ctx context.Context, fileID int64) ([]MarkedSegment, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT kind, start, "end", source FROM file_segments WHERE file_id = ? ORDER BY start`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MarkedSegment{}
	for rows.Next() {
		var s MarkedSegment
		if err := rows.Scan(&s.Kind, &s.Start, &s.End, &s.Source); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SetSegment records where a file's intro or credits are. A mark an admin
// made by hand is only replaced by another manual one, and one from the
// file's chapters isn't replaced by detection.
func (d *DB) SetSegment(ctx context.Context, fileID int64, s MarkedSegment) error {
	_, err := d.sql.ExecContext(ctx, `INSERT INTO file_segments (file_id, kind, start, "end", source) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (file_id, kind) DO UPDATE SET start = excluded.start, "end" = excluded."end", source = excluded.source
		WHERE excluded.source = 'manual' OR file_segments.source = 'detected'
		   OR (file_segments.source = 'chapters' AND excluded.source = 'chapters')`, fileID, s.Kind, s.Start, s.End, s.Source)
	return err
}

// EpisodeFile is one episode's file, as intro detection needs it.
type EpisodeFile struct {
	FileID          int64
	Path            string
	Season, Episode int
	DurationSec     float64
	Size, Mtime     int64
	Checked         bool // been through intro detection as it is now
	HasIntro        bool // already has an intro mark, from anywhere
}

// EpisodeFiles lists a series' episodes in order, one file each (the largest
// copy), leaving out files that can't be read.
func (d *DB) EpisodeFiles(ctx context.Context, itemID int64) ([]EpisodeFile, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT f.id, f.path, e.season, e.episode, COALESCE(f.duration_sec, 0), f.size, f.mtime,
		EXISTS (SELECT 1 FROM intro_checks c WHERE c.file_id = f.id AND c.size = f.size AND c.mtime = f.mtime),
		EXISTS (SELECT 1 FROM file_segments s WHERE s.file_id = f.id AND s.kind = 'intro')
		FROM files f JOIN episodes e ON e.id = f.episode_id
		WHERE f.media_item_id = ? AND f.problem = '' AND f.role <> 'extra'
		ORDER BY e.season, e.episode, f.size DESC`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EpisodeFile
	for rows.Next() {
		var f EpisodeFile
		if err := rows.Scan(&f.FileID, &f.Path, &f.Season, &f.Episode, &f.DurationSec, &f.Size, &f.Mtime, &f.Checked, &f.HasIntro); err != nil {
			return nil, err
		}
		if n := len(out); n > 0 && out[n-1].Season == f.Season && out[n-1].Episode == f.Episode {
			continue // a smaller copy of the same episode
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// MarkIntroChecked remembers that a file has been through intro detection.
func (d *DB) MarkIntroChecked(ctx context.Context, f EpisodeFile) error {
	_, err := d.sql.ExecContext(ctx, `INSERT INTO intro_checks (file_id, size, mtime) VALUES (?, ?, ?)
		ON CONFLICT (file_id) DO UPDATE SET size = excluded.size, mtime = excluded.mtime`, f.FileID, f.Size, f.Mtime)
	return err
}

// SeriesNeedingIntros lists a library's series that have an episode intro
// detection hasn't looked at yet.
func (d *DB) SeriesNeedingIntros(ctx context.Context, libraryID int64) ([]ItemRef, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT m.id, m.title FROM media_items m WHERE m.library_id = ? AND m.kind = 'series'
		AND EXISTS (SELECT 1 FROM files f WHERE f.media_item_id = m.id AND f.episode_id IS NOT NULL AND f.problem = '' AND f.role <> 'extra'
		  AND NOT EXISTS (SELECT 1 FROM intro_checks c WHERE c.file_id = f.id AND c.size = f.size AND c.mtime = f.mtime))`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ItemRef
	for rows.Next() {
		var r ItemRef
		if err := rows.Scan(&r.ID, &r.Title); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteSegment removes a file's intro or credits mark, whatever its source.
func (d *DB) DeleteSegment(ctx context.Context, fileID int64, kind string) error {
	_, err := d.sql.ExecContext(ctx, `DELETE FROM file_segments WHERE file_id = ? AND kind = ?`, fileID, kind)
	return err
}

// ChaptersChecked reports whether the file's chapters were read when it had
// this size and mtime.
func (d *DB) ChaptersChecked(ctx context.Context, fileID, size, mtime int64) (bool, error) {
	var n int
	err := d.sql.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM segment_checks WHERE file_id = ? AND size = ? AND mtime = ?)`,
		fileID, size, mtime).Scan(&n)
	return n > 0, err
}

// SetChapterSegments stores what a file's chapters say (replacing what an
// earlier read of them said, never a manual or detected mark that's better)
// and remembers the file was read.
func (d *DB) SetChapterSegments(ctx context.Context, fileID, size, mtime int64, segs []MarkedSegment) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM file_segments WHERE file_id = ? AND source = 'chapters'`, fileID); err != nil {
		return err
	}
	for _, s := range segs {
		// Chapters are the film-maker's own marks: they win over detection.
		if _, err := tx.ExecContext(ctx, `INSERT INTO file_segments (file_id, kind, start, "end", source) VALUES (?, ?, ?, ?, 'chapters')
			ON CONFLICT (file_id, kind) DO UPDATE SET start = excluded.start, "end" = excluded."end", source = 'chapters'
			WHERE file_segments.source = 'detected'`, fileID, s.Kind, s.Start, s.End); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO segment_checks (file_id, size, mtime) VALUES (?, ?, ?)
		ON CONFLICT (file_id) DO UPDATE SET size = excluded.size, mtime = excluded.mtime`, fileID, size, mtime); err != nil {
		return err
	}
	return tx.Commit()
}
