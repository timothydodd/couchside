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

// SetSegment records where a file's intro or credits are. A segment an admin
// marked by hand is only replaced by another manual one.
func (d *DB) SetSegment(ctx context.Context, fileID int64, s MarkedSegment) error {
	_, err := d.sql.ExecContext(ctx, `INSERT INTO file_segments (file_id, kind, start, "end", source) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (file_id, kind) DO UPDATE SET start = excluded.start, "end" = excluded."end", source = excluded.source
		WHERE file_segments.source <> 'manual' OR excluded.source = 'manual'`, fileID, s.Kind, s.Start, s.End, s.Source)
	return err
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
