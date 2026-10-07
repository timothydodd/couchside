package db

import (
	"context"
	"database/sql"
)

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
	Plot            string
	RuntimeMin      *int
	StillURL        string
	TMDBID          int64
	Credits         []Credit // guest stars and the episode's crew; replaces what was there
}

// ApplyEpisodeMeta updates details for episodes we have files for; episodes
// the provider knows about but we don't have are ignored. An episode's
// credits are replaced only when the provider gave some, so a provider
// without them (OMDb) doesn't wipe TMDB's.
func (d *DB) ApplyEpisodeMeta(ctx context.Context, seriesID int64, eps []EpisodeMeta) error {
	for _, e := range eps {
		var id int64
		err := d.sql.QueryRowContext(ctx, `UPDATE episodes SET title = ?, released = ?, rating = ?, imdb_id = ?,
			plot = ?, runtime_min = ?, still_url = ?, tmdb_id = ?
			WHERE series_id = ? AND season = ? AND episode = ? RETURNING id`,
			e.Title, e.Released, e.Rating, e.ImdbID, e.Plot, e.RuntimeMin, e.StillURL, e.TMDBID, seriesID, e.Season, e.Episode).Scan(&id)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return err
		}
		if len(e.Credits) > 0 {
			if err := d.setCredits(ctx, "episode_credits", "episode_id", id, e.Credits); err != nil {
				return err
			}
		}
	}
	return nil
}

// EpisodeStill is an episode's provider still and the file it's shown on.
type EpisodeStill struct {
	FileID   int64
	StillURL string
}

// EpisodeStills lists the series' episodes that have a provider still, with
// each one's best file, for the artwork job.
func (d *DB) EpisodeStills(ctx context.Context, seriesID int64) ([]EpisodeStill, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT f.id, e.still_url FROM episodes e
		JOIN files f ON f.id = (SELECT f2.id FROM files f2 WHERE f2.episode_id = e.id ORDER BY f2.size DESC LIMIT 1)
		WHERE e.series_id = ? AND e.still_url <> ''`, seriesID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EpisodeStill{}
	for rows.Next() {
		var s EpisodeStill
		if err := rows.Scan(&s.FileID, &s.StillURL); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// EpisodeDetail is an episode's page: the row plus what only its page shows.
type EpisodeDetail struct {
	EpisodeRow
	SeriesID   int64  `json:"seriesId"`
	Plot       string `json:"plot"`
	RuntimeMin *int   `json:"runtimeMin"`
	ImdbID     string `json:"imdbId"`
	TMDBID     int64  `json:"tmdbId"`
}

// EpisodeRef names a neighbouring episode.
type EpisodeRef struct {
	ID      int64  `json:"id"`
	Season  int    `json:"season"`
	Episode int    `json:"episode"`
	Title   string `json:"title"`
}

// Episode is one episode with its best file, or ErrNotFound when it has none
// the profile may see.
func (d *DB) Episode(ctx context.Context, id int64) (EpisodeDetail, error) {
	var e EpisodeDetail
	err := d.sql.QueryRowContext(ctx, `SELECT e.id, e.season, e.episode, e.title, e.released, e.air_date, e.rating,
		f.id, f.problem, f.has_still, f.duration_sec, COALESCE(w.position_sec, 0), COALESCE(w.watched, 0),
		e.series_id, e.plot, e.runtime_min, e.imdb_id, e.tmdb_id
		FROM episodes e
		JOIN files f ON f.id = (SELECT f2.id FROM files f2 WHERE f2.episode_id = e.id ORDER BY f2.size DESC LIMIT 1)
		`+watchJoin(ctx)+`
		WHERE e.id = ?`, id).Scan(&e.ID, &e.Season, &e.Episode, &e.Title, &e.Released, &e.AirDate, &e.Rating, &e.FileID, &e.Problem, &e.HasStill,
		&e.DurationSec, &e.PositionSec, &e.Watched, &e.SeriesID, &e.Plot, &e.RuntimeMin, &e.ImdbID, &e.TMDBID)
	return e, notFound(err)
}

// EpisodeNeighbours returns the episodes before and after one in its series
// (by season and number, among those with a file); nil when there's none.
func (d *DB) EpisodeNeighbours(ctx context.Context, e EpisodeDetail) (prev, next *EpisodeRef, err error) {
	one := func(where, order string) (*EpisodeRef, error) {
		var r EpisodeRef
		err := d.sql.QueryRowContext(ctx, `SELECT e.id, e.season, e.episode, e.title FROM episodes e
			WHERE e.series_id = ? AND EXISTS (SELECT 1 FROM files f WHERE f.episode_id = e.id) AND `+where+`
			ORDER BY `+order+` LIMIT 1`, e.SeriesID, e.Season, e.Episode, e.Season).Scan(&r.ID, &r.Season, &r.Episode, &r.Title)
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return &r, err
	}
	if prev, err = one(`(e.season = ?2 AND e.episode < ?3) OR e.season < ?4`, `e.season DESC, e.episode DESC`); err != nil {
		return nil, nil, err
	}
	next, err = one(`(e.season = ?2 AND e.episode > ?3) OR e.season > ?4`, `e.season, e.episode`)
	return prev, next, err
}

// EpisodeCredits returns an episode's guest stars and crew.
func (d *DB) EpisodeCredits(ctx context.Context, episodeID int64) (cast, crew []CreditRow, err error) {
	return d.credits(ctx, "episode_credits", "episode_id", episodeID)
}

// PersonEpisode is one episode a person is in, for their page.
type PersonEpisode struct {
	EpisodeRow
	SeriesID    int64    `json:"seriesId"`
	SeriesTitle string   `json:"seriesTitle"`
	Roles       []string `json:"roles"`
}

// PersonEpisodes lists the episodes (with a file) a person is credited on,
// newest series first, then in order.
func (d *DB) PersonEpisodes(ctx context.Context, personID int64) ([]PersonEpisode, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT e.id, e.season, e.episode, e.title, e.released, e.air_date, e.rating,
		f.id, f.problem, f.has_still, f.duration_sec, COALESCE(w.position_sec, 0), COALESCE(w.watched, 0),
		m.id, m.title,
		(SELECT group_concat(CASE WHEN c.kind = 'cast' AND c.role = '' THEN 'Cast' ELSE c.role END, '|')
		 FROM episode_credits c WHERE c.episode_id = e.id AND c.person_id = ?1)
		FROM episodes e
		JOIN media_items m ON m.id = e.series_id
		JOIN files f ON f.id = (SELECT f2.id FROM files f2 WHERE f2.episode_id = e.id ORDER BY f2.size DESC LIMIT 1)
		`+watchJoin(ctx)+`
		WHERE e.id IN (SELECT episode_id FROM episode_credits WHERE person_id = ?1)`+visible(ctx, "m")+`
		ORDER BY COALESCE(m.year, 0) DESC, m.sort_title, e.season, e.episode`, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PersonEpisode{}
	for rows.Next() {
		var e PersonEpisode
		var roles *string
		if err := rows.Scan(&e.ID, &e.Season, &e.Episode, &e.Title, &e.Released, &e.AirDate, &e.Rating, &e.FileID, &e.Problem, &e.HasStill,
			&e.DurationSec, &e.PositionSec, &e.Watched, &e.SeriesID, &e.SeriesTitle, &roles); err != nil {
			return nil, err
		}
		e.Roles = splitRoles(roles)
		out = append(out, e)
	}
	return out, rows.Err()
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
