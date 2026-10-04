package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

// ErrNumberTaken means a channel number is already in the lineup.
var ErrNumberTaken = errors.New("that channel number is already in use")

// VirtualChannel is one of Couchside's own channels (livetv/virtual.go).
// Config is owned by the livetv package; State is where its schedule
// builder left off.
type VirtualChannel struct {
	ID     int64           `json:"id"`
	Number string          `json:"number"`
	Name   string          `json:"name"`
	Config json.RawMessage `json:"config"`
	State  string          `json:"-"`
}

const virtualCols = `id, number, name, config, state`

func scanVirtual(r interface{ Scan(...any) error }) (VirtualChannel, error) {
	var v VirtualChannel
	var cfg string
	err := r.Scan(&v.ID, &v.Number, &v.Name, &cfg, &v.State)
	v.Config = json.RawMessage(cfg)
	return v, err
}

func (d *DB) VirtualChannels(ctx context.Context) ([]VirtualChannel, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT `+virtualCols+` FROM virtual_channels ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []VirtualChannel{}
	for rows.Next() {
		v, err := scanVirtual(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (d *DB) VirtualChannel(ctx context.Context, id int64) (VirtualChannel, error) {
	v, err := scanVirtual(d.sql.QueryRowContext(ctx, `SELECT `+virtualCols+` FROM virtual_channels WHERE id = ?`, id))
	return v, notFound(err)
}

// numberFree reports whether no other channel (tuner or virtual) has number.
func numberFree(ctx context.Context, tx *sql.Tx, number string, exceptVirtual int64) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM channels WHERE number = ? AND (virtual_id IS NULL OR virtual_id <> ?)`,
		number, exceptVirtual).Scan(&n)
	return n == 0, err
}

// CreateVirtualChannel adds a channel and its lineup row.
func (d *DB) CreateVirtualChannel(ctx context.Context, number, name string, config []byte, sortKey float64) (int64, error) {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if free, err := numberFree(ctx, tx, number, 0); err != nil {
		return 0, err
	} else if !free {
		return 0, ErrNumberTaken
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO virtual_channels (number, name, config) VALUES (?, ?, ?)`, number, name, string(config))
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if _, err := tx.ExecContext(ctx, `INSERT INTO channels (number, name, url, hd, sort_key, virtual_id) VALUES (?, ?, '', 1, ?, ?)`,
		number, name, sortKey, id); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// UpdateVirtualChannel changes a channel's number, name and config. Its
// schedule and guide are dropped, to be rebuilt from the new config.
func (d *DB) UpdateVirtualChannel(ctx context.Context, id int64, number, name string, config []byte, sortKey float64) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var old string
	if err := tx.QueryRowContext(ctx, `SELECT number FROM virtual_channels WHERE id = ?`, id).Scan(&old); err != nil {
		return notFound(err)
	}
	if free, err := numberFree(ctx, tx, number, id); err != nil {
		return err
	} else if !free {
		return ErrNumberTaken
	}
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`UPDATE virtual_channels SET number = ?, name = ?, config = ?, state = '{}', updated_at = unixepoch() WHERE id = ?`, []any{number, name, string(config), id}},
		{`UPDATE channels SET number = ?, name = ?, sort_key = ? WHERE virtual_id = ?`, []any{number, name, sortKey, id}},
		// A favourite left on the new number by a channel that has gone would
		// clash with this one's.
		{`DELETE FROM profile_channels WHERE number = ?1 AND ?1 <> ?2`, []any{number, old}},
		{`UPDATE profile_channels SET number = ? WHERE number = ?`, []any{number, old}},
		{`DELETE FROM virtual_playout WHERE channel_id = ?`, []any{id}},
		{`DELETE FROM programs WHERE channel IN (?, ?)`, []any{old, number}},
	} {
		if _, err := tx.ExecContext(ctx, q.sql, q.args...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteVirtualChannel removes a channel, its schedule, guide and favourites.
func (d *DB) DeleteVirtualChannel(ctx context.Context, id int64) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var number string
	if err := tx.QueryRowContext(ctx, `SELECT number FROM virtual_channels WHERE id = ?`, id).Scan(&number); err != nil {
		return notFound(err)
	}
	for _, q := range []string{
		`DELETE FROM programs WHERE channel = ?`,
		`DELETE FROM profile_channels WHERE number = ?`,
		`DELETE FROM channels WHERE number = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, number); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM virtual_channels WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// --- schedule ------------------------------------------------------------------

// PlayoutPiece is one stretch of a file in a virtual channel's schedule.
type PlayoutPiece struct {
	StartMs   int64
	EndMs     int64
	Path      string
	InMs      int64 // where in the file the piece starts
	Filler    bool
	HasAudio  bool
	ProgramAt int64 // the guide program (unix seconds) it belongs to
}

// PlayoutEnd is where a channel's schedule runs out (0 if it has none).
func (d *DB) PlayoutEnd(ctx context.Context, channelID int64) (int64, error) {
	var end sql.NullInt64
	err := d.sql.QueryRowContext(ctx, `SELECT MAX(end_ms) FROM virtual_playout WHERE channel_id = ?`, channelID).Scan(&end)
	return end.Int64, err
}

// PlayoutFrom returns up to limit pieces that are playing at ms or come after it.
func (d *DB) PlayoutFrom(ctx context.Context, channelID, ms int64, limit int) ([]PlayoutPiece, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT start_ms, end_ms, path, in_ms, filler, has_audio, program_at
		FROM virtual_playout WHERE channel_id = ? AND end_ms > ? ORDER BY start_ms LIMIT ?`, channelID, ms, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlayoutPiece
	for rows.Next() {
		var p PlayoutPiece
		if err := rows.Scan(&p.StartMs, &p.EndMs, &p.Path, &p.InMs, &p.Filler, &p.HasAudio, &p.ProgramAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ExtendPlayout appends pieces and their guide programs to a channel's
// schedule and saves the builder state, all at once.
func (d *DB) ExtendPlayout(ctx context.Context, channelID int64, pieces []PlayoutPiece, progs []Program, state string) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ins, err := tx.PrepareContext(ctx, `INSERT INTO virtual_playout (channel_id, start_ms, end_ms, path, in_ms, filler, has_audio, program_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	for _, p := range pieces {
		if _, err := ins.ExecContext(ctx, channelID, p.StartMs, p.EndMs, p.Path, p.InMs, p.Filler, p.HasAudio, p.ProgramAt); err != nil {
			return err
		}
	}
	prog, err := tx.PrepareContext(ctx, `INSERT INTO programs (channel, start_at, end_at, title, episode_title, episode_num,
		synopsis, image_url, series_id, original_airdate, is_new, categories) VALUES (?, ?, ?, ?, ?, ?, ?, ?, '', ?, 0, ?)
		ON CONFLICT (channel, start_at) DO UPDATE SET end_at = excluded.end_at, title = excluded.title,
		  episode_title = excluded.episode_title, episode_num = excluded.episode_num, synopsis = excluded.synopsis,
		  image_url = excluded.image_url, original_airdate = excluded.original_airdate, categories = excluded.categories`)
	if err != nil {
		return err
	}
	defer prog.Close()
	for _, p := range progs {
		if _, err := prog.ExecContext(ctx, p.Channel, p.StartAt, p.EndAt, p.Title, p.EpisodeTitle, p.EpisodeNum, p.Synopsis,
			p.ImageURL, p.OriginalAirdate, strings.Join(p.Categories, ",")); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE virtual_channels SET state = ? WHERE id = ?`, state, channelID); err != nil {
		return err
	}
	return tx.Commit()
}

// PrunePlayout drops schedule pieces that ended before ms.
func (d *DB) PrunePlayout(ctx context.Context, ms int64) error {
	_, err := d.sql.ExecContext(ctx, `DELETE FROM virtual_playout WHERE end_ms < ?`, ms)
	return err
}

// --- what a channel can play ---------------------------------------------------

// VirtualFile is one playable file a virtual channel could schedule.
type VirtualFile struct {
	FileID       int64
	ItemID       int64
	LibraryID    int64
	Kind         string // movie | series
	Title        string // the movie, or the show
	SortTitle    string
	Year         int
	Genres       []string
	Rating       float64
	Plot         string
	UpdatedAt    int64
	HasBackdrop  bool
	Path         string
	DurationSec  float64
	HasAudio     bool
	Height       int
	Role         string // copy | part
	PartNo       int
	Season       int // episodes only
	Episode      int
	EpisodeTitle string
	AirDate      string
	HasStill     bool
	AddedAt      int64
}

// VirtualFiles lists every playable file (no extras, no unreadable files,
// a known length) in the given libraries, or all of them.
func (d *DB) VirtualFiles(ctx context.Context, libraries []int64) ([]VirtualFile, error) {
	q := `SELECT f.id, m.id, m.library_id, m.kind, m.title, m.sort_title, COALESCE(m.year, 0), m.genres, COALESCE(m.rating, 0),
		m.plot, m.updated_at, m.has_backdrop, f.path, f.duration_sec, f.audio_codec <> '', COALESCE(f.height, 0), f.role, f.part_no,
		COALESCE(e.season, 0), COALESCE(e.episode, 0), COALESCE(e.title, ''), COALESCE(e.air_date, ''), f.has_still, f.added_at
		FROM files f JOIN media_items m ON m.id = f.media_item_id LEFT JOIN episodes e ON e.id = f.episode_id
		WHERE f.problem = '' AND f.role <> 'extra' AND f.duration_sec > 0 AND (m.kind = 'movie' OR f.episode_id IS NOT NULL)`
	var args []any
	if len(libraries) > 0 {
		q += ` AND m.library_id IN (?` + strings.Repeat(",?", len(libraries)-1) + `)`
		for _, l := range libraries {
			args = append(args, l)
		}
	}
	rows, err := d.sql.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VirtualFile
	for rows.Next() {
		var v VirtualFile
		var genres string
		if err := rows.Scan(&v.FileID, &v.ItemID, &v.LibraryID, &v.Kind, &v.Title, &v.SortTitle, &v.Year, &genres, &v.Rating,
			&v.Plot, &v.UpdatedAt, &v.HasBackdrop, &v.Path, &v.DurationSec, &v.HasAudio, &v.Height, &v.Role, &v.PartNo,
			&v.Season, &v.Episode, &v.EpisodeTitle, &v.AirDate, &v.HasStill, &v.AddedAt); err != nil {
			return nil, err
		}
		v.Genres = SplitGenres(genres)
		out = append(out, v)
	}
	return out, rows.Err()
}

// --- filler --------------------------------------------------------------------

// FillerClip is a commercial (or any short clip) used between and inside programs.
type FillerClip struct {
	Path       string
	Mtime      int64
	DurationMs int64
	HasAudio   bool
}

// FillerClips returns the cached lengths of clips, by path.
func (d *DB) FillerClips(ctx context.Context) (map[string]FillerClip, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT path, mtime, duration_ms, has_audio FROM filler_clips`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]FillerClip{}
	for rows.Next() {
		var c FillerClip
		if err := rows.Scan(&c.Path, &c.Mtime, &c.DurationMs, &c.HasAudio); err != nil {
			return nil, err
		}
		out[c.Path] = c
	}
	return out, rows.Err()
}

// DeleteFillerClips forgets clips that are no longer in their folder.
func (d *DB) DeleteFillerClips(ctx context.Context, paths []string) error {
	for _, p := range paths {
		if _, err := d.sql.ExecContext(ctx, `DELETE FROM filler_clips WHERE path = ?`, p); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) SaveFillerClip(ctx context.Context, c FillerClip) error {
	_, err := d.sql.ExecContext(ctx, `INSERT INTO filler_clips (path, mtime, duration_ms, has_audio) VALUES (?, ?, ?, ?)
		ON CONFLICT (path) DO UPDATE SET mtime = excluded.mtime, duration_ms = excluded.duration_ms, has_audio = excluded.has_audio`,
		c.Path, c.Mtime, c.DurationMs, c.HasAudio)
	return err
}
