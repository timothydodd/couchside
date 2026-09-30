package db

import "context"

func (d *DB) SetOptimized(ctx context.Context, fileID int64, path string, size int64, height int) error {
	_, err := d.sql.ExecContext(ctx, `INSERT INTO optimized (file_id, path, size, height) VALUES (?, ?, ?, ?)
		ON CONFLICT (file_id) DO UPDATE SET path = excluded.path, size = excluded.size, height = excluded.height,
		  created_at = unixepoch()`, fileID, path, size, height)
	return err
}

// OptimizedPath returns the optimized copy for a file, or "" when there is none.
func (d *DB) OptimizedPath(ctx context.Context, fileID int64) (string, error) {
	var p string
	err := d.sql.QueryRowContext(ctx, `SELECT path FROM optimized WHERE file_id = ?`, fileID).Scan(&p)
	if err != nil {
		return "", notFoundOK(err)
	}
	return p, nil
}

func (d *DB) DeleteOptimized(ctx context.Context, fileID int64) (string, error) {
	p, err := d.OptimizedPath(ctx, fileID)
	if err != nil || p == "" {
		return "", err
	}
	_, err = d.sql.ExecContext(ctx, `DELETE FROM optimized WHERE file_id = ?`, fileID)
	return p, err
}

// OptimizedPaths lists every optimized file on record, for orphan cleanup.
func (d *DB) OptimizedPaths(ctx context.Context) (map[string]bool, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT path FROM optimized`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out[p] = true
	}
	return out, rows.Err()
}

// OptimizeCandidates returns file ids (of an item or a library) that aren't
// already browser-friendly MP4/H.264/AAC and have no optimized copy yet.
func (d *DB) OptimizeCandidates(ctx context.Context, itemID, libraryID int64) ([]File, error) {
	return d.queryFiles(ctx, `WHERE (f.media_item_id = ? OR f.library_id = ?) AND f.problem = ''

		AND NOT EXISTS (SELECT 1 FROM optimized o WHERE o.file_id = f.id)
		AND NOT (f.container IN ('mp4', 'm4v', 'mov') AND f.video_codec = 'h264' AND f.audio_codec IN ('aac', 'mp3', ''))
		ORDER BY f.id`, itemID, libraryID)
}
