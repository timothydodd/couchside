package db

import (
	"context"
	"database/sql"
	"encoding/json"
)

// Segment is a commercial break, in seconds from the start of the file.
type Segment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

func (d *DB) SetCommercials(ctx context.Context, fileID, size, mtime int64, segs []Segment) error {
	if segs == nil {
		segs = []Segment{}
	}
	body, err := json.Marshal(segs)
	if err != nil {
		return err
	}
	_, err = d.sql.ExecContext(ctx, `INSERT INTO commercials (file_id, size, mtime, segments) VALUES (?, ?, ?, ?)
		ON CONFLICT (file_id) DO UPDATE SET size = excluded.size, mtime = excluded.mtime, segments = excluded.segments,
		  created_at = unixepoch()`, fileID, size, mtime, string(body))
	return err
}

// Commercials returns the breaks found in a file. ok is false when the file
// hasn't been analysed, or has changed since.
func (d *DB) Commercials(ctx context.Context, fileID int64) (segs []Segment, ok bool, err error) {
	var body string
	err = d.sql.QueryRowContext(ctx, `SELECT c.segments FROM commercials c
		JOIN files f ON f.id = c.file_id AND f.size = c.size AND f.mtime = c.mtime
		WHERE c.file_id = ?`, fileID).Scan(&body)
	if err == sql.ErrNoRows {
		return []Segment{}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if err := json.Unmarshal([]byte(body), &segs); err != nil {
		return nil, false, err
	}
	return segs, true, nil
}

// CommercialDismissals lists the breaks marked "Not a commercial" in a file.
func (d *DB) CommercialDismissals(ctx context.Context, fileID int64) ([]Segment, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT start, "end" FROM commercial_dismissals WHERE file_id = ? ORDER BY start`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Segment{}
	for rows.Next() {
		var s Segment
		if err := rows.Scan(&s.Start, &s.End); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// DismissCommercial marks a break (as the player showed it) as not a commercial.
func (d *DB) DismissCommercial(ctx context.Context, fileID int64, s Segment) error {
	_, err := d.sql.ExecContext(ctx, `INSERT INTO commercial_dismissals (file_id, start, "end") VALUES (?, ?, ?)
		ON CONFLICT (file_id, start) DO UPDATE SET "end" = excluded."end"`, fileID, s.Start, s.End)
	return err
}

// RestoreCommercial undoes DismissCommercial for the dismissal starting near s.Start.
func (d *DB) RestoreCommercial(ctx context.Context, fileID int64, s Segment) error {
	_, err := d.sql.ExecContext(ctx, `DELETE FROM commercial_dismissals WHERE file_id = ? AND abs(start - ?) < 0.5`, fileID, s.Start)
	return err
}

// IsRecording reports whether path is a finished DVR recording.
func (d *DB) IsRecording(ctx context.Context, path string) (bool, error) {
	var n int
	err := d.sql.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM recordings WHERE path = ? AND status = 'completed')`, path).Scan(&n)
	return n > 0, err
}

// LatestJob returns the newest job of a kind for a ref, or nil when there is none.
func (d *DB) LatestJob(ctx context.Context, kind string, refID int64) (*Job, error) {
	j, err := scanJob(d.sql.QueryRowContext(ctx, `SELECT `+jobCols+` FROM jobs WHERE kind = ? AND ref_id = ? ORDER BY id DESC LIMIT 1`, kind, refID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// CommercialCandidates lists an item's files to check for commercials, in
// episode order: playable, not extras, and (unless redo) not already checked
// since they last changed.
func (d *DB) CommercialCandidates(ctx context.Context, itemID int64, redo bool) ([]File, error) {
	return d.queryFiles(ctx, `LEFT JOIN episodes e ON e.id = f.episode_id
		WHERE f.media_item_id = ? AND f.problem = '' AND f.role <> 'extra'
		  AND (? OR NOT EXISTS (SELECT 1 FROM commercials c WHERE c.file_id = f.id AND c.size = f.size AND c.mtime = f.mtime))
		ORDER BY COALESCE(e.season, 0), COALESCE(e.episode, 0), f.part_no, f.size DESC`, itemID, redo)
}
