package db

import (
	"context"
	"database/sql"
	"strings"
)

// SeriesMatch is what a guide series was identified as (see livetv.showYear).
type SeriesMatch struct {
	SeriesID  string
	ImdbID    string
	Year      int // 0 = couldn't tell
	CheckedAt int64
}

// SeriesMatchFor returns the stored identification of a guide series, or nil.
func (d *DB) SeriesMatchFor(ctx context.Context, seriesID string) (*SeriesMatch, error) {
	var m SeriesMatch
	err := d.sql.QueryRowContext(ctx, `SELECT series_id, imdb_id, year, checked_at FROM series_matches WHERE series_id = ?`, seriesID).
		Scan(&m.SeriesID, &m.ImdbID, &m.Year, &m.CheckedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &m, err
}

func (d *DB) SetSeriesMatch(ctx context.Context, seriesID, imdbID string, year int) error {
	_, err := d.sql.ExecContext(ctx, `INSERT INTO series_matches (series_id, imdb_id, year, checked_at) VALUES (?, ?, ?, unixepoch())
		ON CONFLICT (series_id) DO UPDATE SET imdb_id = excluded.imdb_id, year = excluded.year, checked_at = excluded.checked_at`,
		seriesID, imdbID, year)
	return err
}

// FolderSeriesYear is the matched year of the show whose files live in dir,
// or 0 when that isn't known.
func (d *DB) FolderSeriesYear(ctx context.Context, dir string) (int, error) {
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimRight(dir, "/"))
	var y sql.NullInt64
	err := d.sql.QueryRowContext(ctx, `SELECT m.year FROM files f JOIN media_items m ON m.id = f.media_item_id
		WHERE f.path LIKE ? ESCAPE '\' AND m.kind = 'series' AND m.match_status = 'matched' AND m.year IS NOT NULL
		LIMIT 1`, esc+"/%").Scan(&y)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return int(y.Int64), err
}
