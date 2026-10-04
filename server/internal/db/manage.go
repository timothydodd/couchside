package db

import (
	"context"
	"database/sql"
)

// ManageRow is one movie or show in a library's Manage view.
type ManageRow struct {
	ID             int64  `json:"id"`
	Kind           string `json:"kind"`
	Title          string `json:"title"`
	Year           *int   `json:"year"`
	ParsedTitle    string `json:"parsedTitle"`
	ParsedYear     int    `json:"parsedYear"`
	MatchStatus    string `json:"matchStatus"`
	ImdbID         string `json:"imdbId"`
	HasPoster      bool   `json:"hasPoster"`
	CustomPoster   bool   `json:"customPoster"`
	CustomBackdrop bool   `json:"customBackdrop"`
	UpdatedAt      int64  `json:"updatedAt"`
	AddedAt        int64  `json:"addedAt"`
	FileCount      int    `json:"fileCount"`
	EpisodeCount   int    `json:"episodeCount"` // distinct episodes with a file (series)
	Size           int64  `json:"size"`         // bytes, all files
	MaxHeight      int    `json:"maxHeight"`    // best file's resolution
	MinHeight      int    `json:"minHeight"`    // worst file's (series: the weakest episode)
	VideoCodec     string `json:"videoCodec"`   // best file's codec
	SameImdb       int    `json:"sameImdb"`     // other items in the library matched to the same title
	Parts          int    `json:"parts"`        // files that are parts of a split movie
	Extras         int    `json:"extras"`       // bonus-material files
	Editions       int    `json:"editions"`     // different cuts among a movie's copies (1 when they're all the same)
}

// ManageRows lists a library's items with the file facts the Manage view
// sorts and filters on.
func (d *DB) ManageRows(ctx context.Context, libraryID int64) ([]ManageRow, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT m.id, m.kind, m.title, m.year, m.parsed_title, m.parsed_year,
		m.match_status, m.imdb_id, m.has_poster, m.custom_poster, m.custom_backdrop, m.updated_at, m.added_at,
		(SELECT COUNT(*) FROM files f WHERE f.media_item_id = m.id),
		(SELECT COUNT(DISTINCT f.episode_id) FROM files f WHERE f.media_item_id = m.id),
		(SELECT COALESCE(SUM(f.size), 0) FROM files f WHERE f.media_item_id = m.id),
		COALESCE(best.height, 0),
		(SELECT COALESCE(MIN(COALESCE(f.height, 0)), 0) FROM files f WHERE f.media_item_id = m.id AND f.role <> 'extra'),
		COALESCE(best.video_codec, ''),
		(SELECT COUNT(*) FROM media_items m2 WHERE m2.library_id = m.library_id AND m2.id <> m.id
		   AND m.imdb_id <> '' AND m2.imdb_id = m.imdb_id),
		(SELECT COUNT(*) FROM files f WHERE f.media_item_id = m.id AND f.role = 'part'),
		(SELECT COUNT(*) FROM files f WHERE f.media_item_id = m.id AND f.role = 'extra'),
		(SELECT COUNT(DISTINCT f.edition) FROM files f WHERE f.media_item_id = m.id AND f.role = 'copy')
		FROM media_items m
		LEFT JOIN files best ON best.id = (SELECT f.id FROM files f WHERE f.media_item_id = m.id AND f.role <> 'extra'
		  ORDER BY COALESCE(f.height, 0) DESC, f.size DESC LIMIT 1)
		WHERE m.library_id = ? ORDER BY m.sort_title`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ManageRow{}
	for rows.Next() {
		var r ManageRow
		if err := rows.Scan(&r.ID, &r.Kind, &r.Title, &r.Year, &r.ParsedTitle, &r.ParsedYear, &r.MatchStatus, &r.ImdbID,
			&r.HasPoster, &r.CustomPoster, &r.CustomBackdrop, &r.UpdatedAt, &r.AddedAt, &r.FileCount, &r.EpisodeCount,
			&r.Size, &r.MaxHeight, &r.MinHeight, &r.VideoCodec, &r.SameImdb, &r.Parts, &r.Extras, &r.Editions); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ManageFile is one file of an item, with its episode when it has one.
type ManageFile struct {
	ID           int64    `json:"id"`
	Path         string   `json:"path"`
	Size         int64    `json:"size"`
	DurationSec  *float64 `json:"durationSec"`
	Container    string   `json:"container"`
	VideoCodec   string   `json:"videoCodec"`
	AudioCodec   string   `json:"audioCodec"`
	Width        *int     `json:"width"`
	Height       *int     `json:"height"`
	Problem      string   `json:"problem"`
	AddedAt      int64    `json:"addedAt"`
	Season       *int     `json:"season"`
	Episode      *int     `json:"episode"`
	EpisodeTitle string   `json:"episodeTitle"`
	Role         string   `json:"role"`
	PartNo       int      `json:"partNo"`
	ExtraTitle   string   `json:"extraTitle"`
}

// ManageFiles lists an item's files in episode order, best copy first.
func (d *DB) ManageFiles(ctx context.Context, itemID int64) ([]ManageFile, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT f.id, f.path, f.size, f.duration_sec, f.container, f.video_codec,
		f.audio_codec, f.width, f.height, f.problem, f.added_at, e.season, e.episode, COALESCE(e.title, ''),
		f.role, f.part_no, f.extra_title
		FROM files f LEFT JOIN episodes e ON e.id = f.episode_id
		WHERE f.media_item_id = ?
		ORDER BY COALESCE(e.season, 0), COALESCE(e.episode, 0), CASE f.role WHEN 'copy' THEN 0 WHEN 'part' THEN 1 ELSE 2 END,
		  f.part_no, COALESCE(f.height, 0) DESC, f.size DESC, f.extra_title`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ManageFile{}
	for rows.Next() {
		var f ManageFile
		if err := rows.Scan(&f.ID, &f.Path, &f.Size, &f.DurationSec, &f.Container, &f.VideoCodec, &f.AudioCodec,
			&f.Width, &f.Height, &f.Problem, &f.AddedAt, &f.Season, &f.Episode, &f.EpisodeTitle,
			&f.Role, &f.PartNo, &f.ExtraTitle); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// DeleteFileRows forgets files (already deleted on disk), then any of the
// library's items and episodes left without files. It returns the items that went.
func (d *DB) DeleteFileRows(ctx context.Context, libraryID int64, fileIDs []int64) ([]int64, error) {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for _, id := range fileIDs {
		if _, err := tx.ExecContext(ctx, `DELETE FROM files WHERE id = ? AND library_id = ?`, id, libraryID); err != nil {
			return nil, err
		}
	}
	gone, err := orphanItems(ctx, tx, libraryID)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM media_items WHERE library_id = ?
		AND NOT EXISTS (SELECT 1 FROM files f WHERE f.media_item_id = media_items.id)`, libraryID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM episodes WHERE series_id IN (SELECT id FROM media_items WHERE library_id = ?)
		AND NOT EXISTS (SELECT 1 FROM files f WHERE f.episode_id = episodes.id)`, libraryID); err != nil {
		return nil, err
	}
	return gone, tx.Commit()
}

func orphanItems(ctx context.Context, tx *sql.Tx, libraryID int64) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM media_items WHERE library_id = ?
		AND NOT EXISTS (SELECT 1 FROM files f WHERE f.media_item_id = media_items.id)`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// DeleteRecordingsAt drops DVR recording rows whose file was deleted.
func (d *DB) DeleteRecordingsAt(ctx context.Context, path string) error {
	_, err := d.sql.ExecContext(ctx, `DELETE FROM recordings WHERE path = ? AND status IN ('completed', 'failed')`, path)
	return err
}

// SetCustomArtwork records an uploaded poster or backdrop (or clears the flag).
func (d *DB) SetCustomArtwork(ctx context.Context, itemID int64, kind string, custom bool) error {
	q := `UPDATE media_items SET custom_poster = ?, has_poster = CASE WHEN ? THEN 1 ELSE has_poster END, updated_at = unixepoch() WHERE id = ?`
	if kind == "backdrop" {
		q = `UPDATE media_items SET custom_backdrop = ?, has_backdrop = CASE WHEN ? THEN 1 ELSE has_backdrop END, updated_at = unixepoch() WHERE id = ?`
	}
	res, err := d.sql.ExecContext(ctx, q, custom, custom, itemID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CustomArtwork reports which of an item's artwork was uploaded.
func (d *DB) CustomArtwork(ctx context.Context, itemID int64) (poster, backdrop bool, err error) {
	err = d.sql.QueryRowContext(ctx, `SELECT custom_poster, custom_backdrop FROM media_items WHERE id = ?`, itemID).Scan(&poster, &backdrop)
	return poster, backdrop, notFound(err)
}
