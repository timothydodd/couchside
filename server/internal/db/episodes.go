package db

import "context"

// EnsureEpisode finds or creates an episode. A title from the filename only
// fills an empty title, so metadata from a provider is never overwritten.
func (d *DB) EnsureEpisode(ctx context.Context, seriesID int64, season, episode int, title, airDate string) (int64, error) {
	if _, err := d.sql.ExecContext(ctx, `INSERT INTO episodes (series_id, season, episode, title, air_date) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (series_id, season, episode) DO UPDATE SET
		  title = CASE WHEN episodes.title = '' THEN excluded.title ELSE episodes.title END,
		  air_date = excluded.air_date`, seriesID, season, episode, title, airDate); err != nil {
		return 0, err
	}
	var id int64
	err := d.sql.QueryRowContext(ctx, `SELECT id FROM episodes WHERE series_id = ? AND season = ? AND episode = ?`,
		seriesID, season, episode).Scan(&id)
	return id, err
}

func (d *DB) SeriesSeasons(ctx context.Context, seriesID int64) ([]int, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT DISTINCT season FROM episodes WHERE series_id = ? ORDER BY season`, seriesID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int{}
	for rows.Next() {
		var s int
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

type EpisodeMeta struct {
	Season, Episode int
	Title, Released string
	Rating          *float64
	ImdbID          string
}

// ApplyEpisodeMeta updates titles for episodes we have files for; episodes
// the provider knows about but we don't have are ignored.
func (d *DB) ApplyEpisodeMeta(ctx context.Context, seriesID int64, eps []EpisodeMeta) error {
	for _, e := range eps {
		if _, err := d.sql.ExecContext(ctx, `UPDATE episodes SET title = ?, released = ?, rating = ?, imdb_id = ?
			WHERE series_id = ? AND season = ? AND episode = ?`,
			e.Title, e.Released, e.Rating, e.ImdbID, seriesID, e.Season, e.Episode); err != nil {
			return err
		}
	}
	return nil
}

// EpisodeRow is one playable episode: the episode joined to its best file.
type EpisodeRow struct {
	ID          int64    `json:"id"`
	Season      int      `json:"season"`
	Episode     int      `json:"episode"`
	Title       string   `json:"title"`
	Released    string   `json:"released"`
	AirDate     string   `json:"airDate"`
	Rating      *float64 `json:"rating"`
	FileID      int64    `json:"fileId"`
	Problem     string   `json:"problem"`
	HasStill    bool     `json:"hasStill"`
	DurationSec *float64 `json:"durationSec"`
	PositionSec float64  `json:"positionSec"`
	Watched     bool     `json:"watched"`
}

func (d *DB) SeriesEpisodes(ctx context.Context, seriesID int64) ([]EpisodeRow, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT e.id, e.season, e.episode, e.title, e.released, e.air_date, e.rating,
		f.id, f.problem, f.has_still, f.duration_sec, COALESCE(w.position_sec, 0), COALESCE(w.watched, 0)
		FROM episodes e
		JOIN files f ON f.id = (SELECT f2.id FROM files f2 WHERE f2.episode_id = e.id ORDER BY f2.size DESC LIMIT 1)
		`+watchJoin(ctx)+`
		WHERE e.series_id = ? ORDER BY e.season, e.episode`, seriesID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EpisodeRow{}
	for rows.Next() {
		var e EpisodeRow
		if err := rows.Scan(&e.ID, &e.Season, &e.Episode, &e.Title, &e.Released, &e.AirDate, &e.Rating, &e.FileID, &e.Problem, &e.HasStill,
			&e.DurationSec, &e.PositionSec, &e.Watched); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
