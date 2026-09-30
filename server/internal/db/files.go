package db

import (
	"context"
	"database/sql"
)

type File struct {
	ID             int64    `json:"id"`
	LibraryID      int64    `json:"libraryId"`
	MediaItemID    int64    `json:"mediaItemId"`
	EpisodeID      *int64   `json:"episodeId"`
	Path           string   `json:"path"`
	Size           int64    `json:"size"`
	Mtime          int64    `json:"mtime"`
	DurationSec    *float64 `json:"durationSec"`
	Container      string   `json:"container"`
	VideoCodec     string   `json:"videoCodec"`
	AudioCodec     string   `json:"audioCodec"`
	Width          *int     `json:"width"`
	Height         *int     `json:"height"`
	AudioTracks    int      `json:"audioTracks"`
	SubtitleTracks int      `json:"subtitleTracks"`
	HasStill       bool     `json:"hasStill"`
	AddedAt        int64    `json:"addedAt"`
	PositionSec    float64  `json:"positionSec"`
	Watched        bool     `json:"watched"`
	Optimized      bool     `json:"optimized"`
	Problem        string   `json:"problem"` // '' | unreadable | no-video
}

const fileCols = `f.id, f.library_id, f.media_item_id, f.episode_id, f.path, f.size, f.mtime, f.duration_sec,
	f.container, f.video_codec, f.audio_codec, f.width, f.height, f.audio_tracks, f.subtitle_tracks,
	f.has_still, f.added_at, COALESCE(w.position_sec, 0), COALESCE(w.watched, 0),
	EXISTS (SELECT 1 FROM optimized o WHERE o.file_id = f.id), f.problem`

const fileFrom = ` FROM files f LEFT JOIN watch_state w ON w.file_id = f.id `

func scanFile(r interface{ Scan(...any) error }) (File, error) {
	var f File
	err := r.Scan(&f.ID, &f.LibraryID, &f.MediaItemID, &f.EpisodeID, &f.Path, &f.Size, &f.Mtime, &f.DurationSec,
		&f.Container, &f.VideoCodec, &f.AudioCodec, &f.Width, &f.Height, &f.AudioTracks, &f.SubtitleTracks,
		&f.HasStill, &f.AddedAt, &f.PositionSec, &f.Watched, &f.Optimized, &f.Problem)
	return f, err
}

func (d *DB) queryFiles(ctx context.Context, where string, args ...any) ([]File, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT `+fileCols+fileFrom+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []File{}
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (d *DB) File(ctx context.Context, id int64) (File, error) {
	f, err := scanFile(d.sql.QueryRowContext(ctx, `SELECT `+fileCols+fileFrom+`WHERE f.id = ?`, id))
	return f, notFound(err)
}

// ItemFiles returns an item's files, largest (usually best quality) first.
func (d *DB) ItemFiles(ctx context.Context, itemID int64) ([]File, error) {
	return d.queryFiles(ctx, `WHERE f.media_item_id = ? ORDER BY f.size DESC`, itemID)
}

// FileStamp is what the scanner compares to skip unchanged files, plus how
// the file was parsed last time so parser improvements reach existing files.
type FileStamp struct {
	ID          int64
	Size        int64
	Mtime       int64
	ParsedTitle string
	ParsedYear  int
	Season      int
	Episode     int
}

func (d *DB) FileStamp(ctx context.Context, path string) (*FileStamp, error) {
	var s FileStamp
	err := d.sql.QueryRowContext(ctx, `SELECT f.id, f.size, f.mtime, m.parsed_title, m.parsed_year,
		COALESCE(e.season, 0), COALESCE(e.episode, 0)
		FROM files f JOIN media_items m ON m.id = f.media_item_id LEFT JOIN episodes e ON e.id = f.episode_id
		WHERE f.path = ?`, path).Scan(&s.ID, &s.Size, &s.Mtime, &s.ParsedTitle, &s.ParsedYear, &s.Season, &s.Episode)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &s, err
}

func (d *DB) TouchFile(ctx context.Context, id, seen int64) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE files SET last_seen = ? WHERE id = ?`, seen, id)
	return err
}

// UpsertFile inserts or refreshes a file row keyed by path and returns its id.
func (d *DB) UpsertFile(ctx context.Context, f File, seen int64) (int64, error) {
	var id int64
	err := d.sql.QueryRowContext(ctx, `INSERT INTO files (library_id, media_item_id, episode_id, path, size, mtime,
		duration_sec, container, video_codec, audio_codec, width, height, audio_tracks, subtitle_tracks, problem, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (path) DO UPDATE SET library_id = excluded.library_id, media_item_id = excluded.media_item_id,
		  episode_id = excluded.episode_id, size = excluded.size, mtime = excluded.mtime,
		  duration_sec = excluded.duration_sec, container = excluded.container, video_codec = excluded.video_codec,
		  audio_codec = excluded.audio_codec, width = excluded.width, height = excluded.height,
		  audio_tracks = excluded.audio_tracks, subtitle_tracks = excluded.subtitle_tracks,
		  problem = excluded.problem, has_still = 0, last_seen = excluded.last_seen
		RETURNING id`,
		f.LibraryID, f.MediaItemID, f.EpisodeID, f.Path, f.Size, f.Mtime, f.DurationSec, f.Container, f.VideoCodec,
		f.AudioCodec, f.Width, f.Height, f.AudioTracks, f.SubtitleTracks, f.Problem, seen).Scan(&id)
	return id, err
}

func (d *DB) SetFileStill(ctx context.Context, id int64, has bool) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE files SET has_still = ? WHERE id = ?`, has, id)
	return err
}

// PruneLibrary removes files not seen since scanStart, then any items and
// episodes left without files. Returns how many files were removed.
func (d *DB) PruneLibrary(ctx context.Context, libraryID, scanStart int64) (int64, error) {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `DELETE FROM files WHERE library_id = ? AND last_seen < ?`, libraryID, scanStart)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if _, err := tx.ExecContext(ctx, `DELETE FROM media_items WHERE library_id = ?
		AND NOT EXISTS (SELECT 1 FROM files f WHERE f.media_item_id = media_items.id)`, libraryID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM episodes WHERE series_id IN (SELECT id FROM media_items WHERE library_id = ?)
		AND NOT EXISTS (SELECT 1 FROM files f WHERE f.episode_id = episodes.id)`, libraryID); err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

// BackdropSource picks the file to grab a backdrop frame from: the largest
// file for a movie, the first episode for a series.
func (d *DB) BackdropSource(ctx context.Context, itemID int64) (*File, error) {
	files, err := d.queryFiles(ctx, `LEFT JOIN episodes e ON e.id = f.episode_id WHERE f.media_item_id = ? AND f.problem = ''
		
		ORDER BY COALESCE(e.season, 0), COALESCE(e.episode, 0), f.size DESC LIMIT 1`, itemID)
	if err != nil || len(files) == 0 {
		return nil, err
	}
	return &files[0], nil
}
