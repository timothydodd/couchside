package db

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"
)

// watchedAt is the fraction of a file after which it counts as watched.
const watchedAt = 0.92

// SaveProgress and SetWatched write the watch state of the profile in ctx.
func (d *DB) SaveProgress(ctx context.Context, fileID int64, position, duration float64) error {
	watched := duration > 0 && position/duration >= watchedAt
	_, err := d.sql.ExecContext(ctx, `INSERT INTO watch_state (profile_id, file_id, position_sec, duration_sec, watched, updated_at)
		VALUES (?, ?, ?, ?, ?, unixepoch())
		ON CONFLICT (profile_id, file_id) DO UPDATE SET position_sec = excluded.position_sec, duration_sec = excluded.duration_sec,
		  watched = MAX(watch_state.watched, excluded.watched), updated_at = excluded.updated_at`,
		ProfileID(ctx), fileID, position, duration, watched)
	return err
}

// SetWatched marks files watched (position cleared) or unwatched (state removed).
func (d *DB) SetWatched(ctx context.Context, fileIDs []int64, watched bool) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range fileIDs {
		if watched {
			_, err = tx.ExecContext(ctx, `INSERT INTO watch_state (profile_id, file_id, position_sec, watched, updated_at)
				VALUES (?, ?, 0, 1, unixepoch())
				ON CONFLICT (profile_id, file_id) DO UPDATE SET position_sec = 0, watched = 1, updated_at = unixepoch()`, ProfileID(ctx), id)
		} else {
			_, err = tx.ExecContext(ctx, `DELETE FROM watch_state WHERE profile_id = ? AND file_id = ?`, ProfileID(ctx), id)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// PlayInfo is everything the player needs about one file.
type PlayInfo struct {
	FileID      int64    `json:"fileId"`
	ItemID      int64    `json:"itemId"`
	Kind        string   `json:"kind"`
	Title       string   `json:"title"`
	Subtitle    string   `json:"subtitle"`
	PositionSec float64  `json:"positionSec"`
	DurationSec *float64 `json:"durationSec"`
	Watched     bool     `json:"watched"`
	HasStill    bool     `json:"hasStill"`
	HasBackdrop bool     `json:"hasBackdrop"`
	UpdatedAt   int64    `json:"updatedAt"`
	NextFileID  *int64   `json:"nextFileId"`
	// NextUp: on Home's Continue Watching row because the episode before it
	// was finished, not because this one was started.
	NextUp     bool    `json:"nextUp,omitempty"`
	Progress   float64 `json:"progress"`
	Container  string  `json:"container"`
	VideoCodec string  `json:"videoCodec"`
	AudioCodec string  `json:"audioCodec"`
	Width      *int    `json:"width"`
	Height     *int    `json:"height"`
	Optimized  bool    `json:"optimized"`
	Problem    string  `json:"problem"`
	Role       string  `json:"role"` // copy | part | extra
	PartNo     int     `json:"partNo"`
	ExtraTitle string  `json:"extraTitle"`
	// Parts is the whole movie, in order, when this file is one part of it,
	// so the player can draw one timeline across them.
	Parts []PartRef `json:"parts,omitempty"`
}

// PartRef is one part of a movie split across files.
type PartRef struct {
	FileID      int64    `json:"fileId"`
	PartNo      int      `json:"partNo"`
	DurationSec *float64 `json:"durationSec"`
}

const playCols = `f.id, m.id, m.kind, m.title, e.season, e.episode, COALESCE(e.title, ''),
	COALESCE(w.position_sec, 0), f.duration_sec, COALESCE(w.watched, 0), f.has_still, m.has_backdrop, m.updated_at,
	f.container, f.video_codec, f.audio_codec, f.width, f.height, EXISTS (SELECT 1 FROM optimized o WHERE o.file_id = f.id),
	f.problem, COALESCE(e.air_date, ''), f.role, f.part_no, f.extra_title`

func playFrom(ctx context.Context) string {
	return ` FROM files f JOIN media_items m ON m.id = f.media_item_id` + visible(ctx, "m") + `
	LEFT JOIN episodes e ON e.id = f.episode_id ` + watchJoin(ctx)
}

// scanPlay reads playCols, then any further columns the query selected into extra.
func scanPlay(r interface{ Scan(...any) error }, extra ...any) (PlayInfo, error) {
	var p PlayInfo
	var season, episode sql.NullInt64
	var epTitle, airDate string
	err := r.Scan(append([]any{&p.FileID, &p.ItemID, &p.Kind, &p.Title, &season, &episode, &epTitle, &p.PositionSec,
		&p.DurationSec, &p.Watched, &p.HasStill, &p.HasBackdrop, &p.UpdatedAt,
		&p.Container, &p.VideoCodec, &p.AudioCodec, &p.Width, &p.Height, &p.Optimized, &p.Problem, &airDate,
		&p.Role, &p.PartNo, &p.ExtraTitle}, extra...)...)
	if err != nil {
		return p, err
	}
	switch p.Role {
	case "extra":
		p.Title += " - " + p.ExtraTitle
	case "part":
		p.Subtitle = fmt.Sprintf("Part %d", p.PartNo)
	}
	if airDate != "" {
		p.Subtitle = FormatAirDate(airDate)
		if epTitle != "" {
			p.Subtitle += " · " + epTitle
		}
	} else if season.Valid {
		p.Subtitle = fmt.Sprintf("S%d · E%d", season.Int64, episode.Int64)
		if epTitle != "" {
			p.Subtitle += " · " + epTitle
		}
	}
	if p.DurationSec != nil && *p.DurationSec > 0 {
		p.Progress = min(1, p.PositionSec / *p.DurationSec)
	}
	return p, nil
}

func (d *DB) PlayInfo(ctx context.Context, fileID int64) (PlayInfo, error) {
	p, err := scanPlay(d.sql.QueryRowContext(ctx, `SELECT `+playCols+playFrom(ctx)+`WHERE f.id = ?`, fileID))
	if err != nil {
		return p, notFound(err)
	}
	if p.Role == "part" {
		return p, d.addParts(ctx, &p)
	}
	// Next episode in the same series, for autoplay.
	var next int64
	err = d.sql.QueryRowContext(ctx, `SELECT f2.id FROM files f
		JOIN episodes e ON e.id = f.episode_id
		JOIN episodes e2 ON e2.series_id = e.series_id AND (e2.season, e2.episode) > (e.season, e.episode)
		JOIN files f2 ON f2.episode_id = e2.id
		WHERE f.id = ? ORDER BY e2.season, e2.episode, f2.size DESC LIMIT 1`, fileID).Scan(&next)
	if err == nil {
		p.NextFileID = &next
	} else if err != sql.ErrNoRows {
		return p, err
	}
	return p, nil
}

// addParts fills in the movie's parts (the best copy of each part number)
// and makes the next part what plays after this one.
func (d *DB) addParts(ctx context.Context, p *PlayInfo) error {
	rows, err := d.sql.QueryContext(ctx, `SELECT f.id, f.part_no, f.duration_sec FROM files f
		WHERE f.media_item_id = ? AND f.role = 'part' AND f.problem = ''
		ORDER BY f.part_no, f.id = ? DESC, f.size DESC`, p.ItemID, p.FileID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var r PartRef
		if err := rows.Scan(&r.FileID, &r.PartNo, &r.DurationSec); err != nil {
			return err
		}
		if n := len(p.Parts); n > 0 && p.Parts[n-1].PartNo == r.PartNo {
			continue // another copy of the same part
		}
		p.Parts = append(p.Parts, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i, r := range p.Parts {
		if r.FileID != p.FileID {
			continue
		}
		p.Subtitle = fmt.Sprintf("Part %d of %d", i+1, len(p.Parts))
		if i+1 < len(p.Parts) {
			next := p.Parts[i+1].FileID
			p.NextFileID = &next
		}
	}
	return nil
}

// nextUpSeries is how many recently watched series are checked for a next episode.
const nextUpSeries = 40

// ContinueWatching is Home's row: files the profile is part way through, and
// for each series whose last finished episode has an unwatched one after it,
// that next episode (NextUp). Most recent activity first. Titles the profile
// removed from the row (HideFromHome) stay out until they're watched again.
func (d *DB) ContinueWatching(ctx context.Context, limit int) ([]PlayInfo, error) {
	type entry struct {
		p  PlayInfo
		at int64 // when the profile last watched this title
	}
	var all []entry
	started := map[int64]bool{} // titles with a file in progress

	rows, err := d.sql.QueryContext(ctx, `SELECT `+playCols+`, w.updated_at`+playFrom(ctx)+`WHERE w.watched = 0 AND w.position_sec > 30
		ORDER BY w.updated_at DESC LIMIT ?`, limit*2)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var e entry
		if e.p, err = scanPlay(rows, &e.at); err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, e)
		started[e.p.ItemID] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// The episode each series was last finished at, most recent first.
	type last struct {
		series          int64
		season, episode int64
		at              int64
	}
	var lasts []last
	seen := map[int64]bool{}
	rows, err = d.sql.QueryContext(ctx, `SELECT e.series_id, e.season, e.episode, w.updated_at
		FROM watch_state w JOIN files f ON f.id = w.file_id JOIN episodes e ON e.id = f.episode_id
		WHERE w.profile_id = ? AND w.watched = 1 ORDER BY w.updated_at DESC, e.season DESC, e.episode DESC LIMIT 2000`, ProfileID(ctx))
	if err != nil {
		return nil, err
	}
	for rows.Next() && len(lasts) < nextUpSeries {
		var l last
		if err := rows.Scan(&l.series, &l.season, &l.episode, &l.at); err != nil {
			rows.Close()
			return nil, err
		}
		if !seen[l.series] {
			seen[l.series] = true
			lasts = append(lasts, l)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, l := range lasts {
		if started[l.series] {
			continue // already in the row with the episode being watched
		}
		p, err := scanPlay(d.sql.QueryRowContext(ctx, `SELECT `+playCols+playFrom(ctx)+`
			WHERE m.id = ? AND (e.season, e.episode) > (?, ?) AND f.problem = '' AND f.role <> 'extra' AND COALESCE(w.watched, 0) = 0
			ORDER BY e.season, e.episode, f.size DESC LIMIT 1`, l.series, l.season, l.episode))
		if err == sql.ErrNoRows {
			continue // caught up
		}
		if err != nil {
			return nil, err
		}
		p.NextUp = true
		all = append(all, entry{p, l.at})
	}

	hidden := map[int64]int64{}
	rows, err = d.sql.QueryContext(ctx, `SELECT item_id, hidden_at FROM home_hidden WHERE profile_id = ?`, ProfileID(ctx))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item, at int64
		if err := rows.Scan(&item, &at); err != nil {
			rows.Close()
			return nil, err
		}
		hidden[item] = at
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.SliceStable(all, func(i, j int) bool { return all[i].at > all[j].at })
	out := []PlayInfo{}
	for _, e := range all {
		if at, ok := hidden[e.p.ItemID]; ok && e.at <= at {
			continue
		}
		if len(out) == limit {
			break
		}
		out = append(out, e.p)
	}
	return out, nil
}

// HideFromHome takes a title off the profile's Continue Watching row until
// the profile watches it again. The resume point is kept.
func (d *DB) HideFromHome(ctx context.Context, itemID int64) error {
	_, err := d.sql.ExecContext(ctx, `INSERT INTO home_hidden (profile_id, item_id, hidden_at) VALUES (?, ?, unixepoch())
		ON CONFLICT (profile_id, item_id) DO UPDATE SET hidden_at = excluded.hidden_at`, ProfileID(ctx), itemID)
	return err
}

// FormatAirDate renders "2024-11-10 03:30" as "Nov 10, 2024 3:30 AM".
func FormatAirDate(s string) string {
	if t, err := time.Parse("2006-01-02 15:04", s); err == nil {
		return t.Format("Jan 2, 2006 3:04 PM")
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Format("Jan 2, 2006")
	}
	return s
}
