package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// --- channels ------------------------------------------------------------------

type Channel struct {
	Number     string   `json:"number"`
	Name       string   `json:"name"`
	Affiliate  string   `json:"affiliate"`
	LogoURL    ImageURL `json:"logoUrl"`
	URL        string   `json:"-"`
	HD         bool     `json:"hd"`
	DRM        bool     `json:"drm"`
	VideoCodec string   `json:"videoCodec"`
	AudioCodec string   `json:"audioCodec"`

	Pinned         bool `json:"pinned"`
	SignalStrength *int `json:"signalStrength"`
	SignalQuality  *int `json:"signalQuality"`
	// Virtual channels are Couchside's own (livetv/virtual.go), not the tuner's.
	Virtual   bool   `json:"virtual"`
	VirtualID *int64 `json:"-"`
}

// ReplaceChannels swaps in a fresh lineup. Guide logos/affiliates set by
// SetChannelGuideInfo survive because they're re-applied on every guide fetch.
func (d *DB) ReplaceChannels(ctx context.Context, chans []Channel, sortKey func(string) float64) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	keep := make([]any, 0, len(chans))
	for _, c := range chans {
		if _, err := tx.ExecContext(ctx, `INSERT INTO channels (number, name, url, hd, drm, video_codec, audio_codec, sort_key,
			signal_strength, signal_quality, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, unixepoch())
			ON CONFLICT (number) DO UPDATE SET name = excluded.name, url = excluded.url, hd = excluded.hd, drm = excluded.drm,
			  video_codec = excluded.video_codec, audio_codec = excluded.audio_codec, sort_key = excluded.sort_key,
			  signal_strength = excluded.signal_strength, signal_quality = excluded.signal_quality,
			  updated_at = excluded.updated_at
			WHERE channels.virtual_id IS NULL`, // a virtual channel keeps its number
			c.Number, c.Name, c.URL, c.HD, c.DRM, c.VideoCodec, c.AudioCodec, sortKey(c.Number),
			c.SignalStrength, c.SignalQuality); err != nil {
			return err
		}
		keep = append(keep, c.Number)
	}
	if len(keep) > 0 {
		q := `DELETE FROM channels WHERE virtual_id IS NULL AND number NOT IN (?` + strings.Repeat(",?", len(keep)-1) + `)`
		if _, err := tx.ExecContext(ctx, q, keep...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// channelCols reports pinned (a favourite) for the profile in ctx.
func channelCols(ctx context.Context) string {
	return fmt.Sprintf(`number, name, affiliate, logo_url, url, hd, drm, video_codec, audio_codec,
	EXISTS (SELECT 1 FROM profile_channels pc WHERE pc.profile_id = %d AND pc.number = channels.number) AS pinned,
	signal_strength, signal_quality, virtual_id`, ProfileID(ctx))
}

func channelDest(c *Channel) []any {
	return []any{&c.Number, &c.Name, &c.Affiliate, &c.LogoURL, &c.URL, &c.HD, &c.DRM, &c.VideoCodec, &c.AudioCodec,
		&c.Pinned, &c.SignalStrength, &c.SignalQuality, &c.VirtualID}
}

// SetChannelPinned makes a channel one of the ctx profile's favourites,
// pinned to the top of the guide (or unpins it).
func (d *DB) SetChannelPinned(ctx context.Context, number string, pinned bool) error {
	var n int
	if err := d.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM channels WHERE number = ?`, number).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	q := `DELETE FROM profile_channels WHERE profile_id = ? AND number = ?`
	if pinned {
		q = `INSERT OR IGNORE INTO profile_channels (profile_id, number) VALUES (?, ?)`
	}
	_, err := d.sql.ExecContext(ctx, q, ProfileID(ctx), number)
	return err
}

func (d *DB) SetChannelGuideInfo(ctx context.Context, number, affiliate, logo string) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE channels SET affiliate = ?, logo_url = ? WHERE number = ?`, affiliate, logo, number)
	return err
}

func (d *DB) Channels(ctx context.Context) ([]Channel, error) {
	// Pinned channels first, then by channel number.
	rows, err := d.sql.QueryContext(ctx, `SELECT `+channelCols(ctx)+` FROM channels ORDER BY pinned DESC, sort_key, number`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Channel{}
	for rows.Next() {
		var c Channel
		if err := rows.Scan(channelDest(&c)...); err != nil {
			return nil, err
		}
		c.Virtual = c.VirtualID != nil
		out = append(out, c)
	}
	return out, rows.Err()
}

func (d *DB) Channel(ctx context.Context, number string) (Channel, error) {
	var c Channel
	err := d.sql.QueryRowContext(ctx, `SELECT `+channelCols(ctx)+` FROM channels WHERE number = ?`, number).Scan(channelDest(&c)...)
	c.Virtual = c.VirtualID != nil
	return c, notFound(err)
}

// --- programs ------------------------------------------------------------------

type Program struct {
	ID              int64    `json:"id"`
	Channel         string   `json:"channel"`
	StartAt         int64    `json:"startAt"`
	EndAt           int64    `json:"endAt"`
	Title           string   `json:"title"`
	EpisodeTitle    string   `json:"episodeTitle"`
	EpisodeNum      string   `json:"episodeNum"`
	Synopsis        string   `json:"synopsis"`
	ImageURL        ImageURL `json:"imageUrl"`
	SeriesID        string   `json:"seriesId"`
	OriginalAirdate *int64   `json:"originalAirdate"`
	IsNew           bool     `json:"isNew"`
	Categories      []string `json:"categories"`
	RecordingID     *int64   `json:"recordingId"`     // set when a recording covers this program
	RecordingStatus string   `json:"recordingStatus"` // scheduled | recording | ...
	RuleID          *int64   `json:"ruleId"`          // series rule for this show, if any
}

func (d *DB) UpsertPrograms(ctx context.Context, ps []Program) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO programs (channel, start_at, end_at, title, episode_title, episode_num,
		synopsis, image_url, series_id, original_airdate, is_new, categories) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (channel, start_at) DO UPDATE SET end_at = excluded.end_at, title = excluded.title,
		  episode_title = excluded.episode_title, episode_num = excluded.episode_num, synopsis = excluded.synopsis,
		  image_url = excluded.image_url, series_id = excluded.series_id, original_airdate = excluded.original_airdate,
		  is_new = excluded.is_new, categories = excluded.categories`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	type span struct {
		from, to int64
		starts   []any
	}
	spans := map[string]*span{}
	for _, p := range ps {
		if _, err := stmt.ExecContext(ctx, p.Channel, p.StartAt, p.EndAt, p.Title, p.EpisodeTitle, p.EpisodeNum, p.Synopsis,
			p.ImageURL, p.SeriesID, p.OriginalAirdate, p.IsNew, strings.Join(p.Categories, ",")); err != nil {
			return err
		}
		sp := spans[p.Channel]
		if sp == nil {
			sp = &span{from: p.StartAt, to: p.EndAt}
			spans[p.Channel] = sp
		}
		sp.from, sp.to = min(sp.from, p.StartAt), max(sp.to, p.EndAt)
		sp.starts = append(sp.starts, p.StartAt)
	}
	// The page is the whole truth for the time it covers: a program it no
	// longer lists there moved (8:00 became 8:15) or was dropped, and its old
	// row would overlap the new one.
	for ch, sp := range spans {
		args := append([]any{ch, sp.from, sp.to}, sp.starts...)
		if _, err := tx.ExecContext(ctx, `DELETE FROM programs WHERE channel = ? AND start_at >= ? AND start_at < ?
			AND start_at NOT IN (?`+strings.Repeat(",?", len(sp.starts)-1)+`)`, args...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// PrunePrograms drops listings that ended before cutoff.
func (d *DB) PrunePrograms(ctx context.Context, cutoff int64) error {
	_, err := d.sql.ExecContext(ctx, `DELETE FROM programs WHERE end_at < ?`, cutoff)
	return err
}

func (d *DB) GuideCoverage(ctx context.Context) (int64, error) {
	var t sql.NullInt64
	err := d.sql.QueryRowContext(ctx, `SELECT MAX(end_at) FROM programs`).Scan(&t)
	return t.Int64, err
}

const programCols = `p.id, p.channel, p.start_at, p.end_at, p.title, p.episode_title, p.episode_num, p.synopsis,
	p.image_url, p.series_id, p.original_airdate, p.is_new, p.categories, r.id, COALESCE(r.status, ''),
	(SELECT sr.id FROM series_rules sr WHERE sr.series_id = p.series_id AND p.series_id <> '' AND sr.enabled = 1)`

// A recording "covers" a program when it's on the same channel and starts at
// the same time (that's how programs are recorded from the guide).
const programFrom = ` FROM programs p LEFT JOIN recordings r ON r.channel = p.channel AND r.start_at = p.start_at
	AND r.status IN ('scheduled', 'recording', 'completed') `

func scanProgram(r interface{ Scan(...any) error }) (Program, error) {
	var p Program
	var cats string
	err := r.Scan(&p.ID, &p.Channel, &p.StartAt, &p.EndAt, &p.Title, &p.EpisodeTitle, &p.EpisodeNum, &p.Synopsis,
		&p.ImageURL, &p.SeriesID, &p.OriginalAirdate, &p.IsNew, &cats, &p.RecordingID, &p.RecordingStatus, &p.RuleID)
	p.Categories = SplitGenres(cats)
	return p, err
}

// ProgramsBetween returns programs overlapping [from, to), by channel then time.
func (d *DB) ProgramsBetween(ctx context.Context, from, to int64) ([]Program, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT `+programCols+programFrom+`WHERE p.end_at > ? AND p.start_at < ?
		ORDER BY p.channel, p.start_at`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Program{}
	for rows.Next() {
		p, err := scanProgram(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (d *DB) Program(ctx context.Context, id int64) (Program, error) {
	p, err := scanProgram(d.sql.QueryRowContext(ctx, `SELECT `+programCols+programFrom+`WHERE p.id = ?`, id))
	return p, notFound(err)
}

// ProgramAt returns what's on a channel at time t.
func (d *DB) ProgramAt(ctx context.Context, channel string, t int64) (*Program, error) {
	p, err := scanProgram(d.sql.QueryRowContext(ctx, `SELECT `+programCols+programFrom+`WHERE p.channel = ? AND p.start_at <= ? AND p.end_at > ?
		ORDER BY p.start_at DESC LIMIT 1`, channel, t, t))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &p, err
}

// --- recordings ------------------------------------------------------------------

type Recording struct {
	ID           int64    `json:"id"`
	Channel      string   `json:"channel"`
	ChannelName  string   `json:"channelName"`
	Title        string   `json:"title"`
	EpisodeTitle string   `json:"episodeTitle"`
	EpisodeNum   string   `json:"episodeNum"`
	Synopsis     string   `json:"synopsis"`
	ImageURL     ImageURL `json:"imageUrl"`
	SeriesID     string   `json:"seriesId"`
	Categories   []string `json:"categories"`
	StartAt      int64    `json:"startAt"`
	EndAt        int64    `json:"endAt"`
	PadBefore    int64    `json:"padBefore"`
	PadAfter     int64    `json:"padAfter"`
	Status       string   `json:"status"`
	Path         string   `json:"-"`
	Size         int64    `json:"size"`
	Error        string   `json:"error"`
	StartedAt    *int64   `json:"startedAt"`
	FinishedAt   *int64   `json:"finishedAt"`
	CreatedAt    int64    `json:"createdAt"`
	FileID       *int64   `json:"fileId"`  // library file once the recording has been scanned
	RuleID       *int64   `json:"ruleId"`  // series rule that scheduled it
	OwnerID      int64    `json:"ownerId"` // profile that scheduled it (or owns its rule); 0 = admins only
}

const recCols = `r.id, r.channel, r.channel_name, r.title, r.episode_title, r.episode_num, r.synopsis, r.image_url,
	r.series_id, r.categories, r.start_at, r.end_at, r.pad_before, r.pad_after, r.status, r.path, r.size, r.error,
	r.started_at, r.finished_at, r.created_at, (SELECT f.id FROM files f WHERE f.path = r.path AND r.path <> ''), r.rule_id,
	COALESCE(r.profile_id, (SELECT sr.profile_id FROM series_rules sr WHERE sr.id = r.rule_id), 0)`

func scanRecording(row interface{ Scan(...any) error }) (Recording, error) {
	var r Recording
	var cats string
	err := row.Scan(&r.ID, &r.Channel, &r.ChannelName, &r.Title, &r.EpisodeTitle, &r.EpisodeNum, &r.Synopsis, &r.ImageURL,
		&r.SeriesID, &cats, &r.StartAt, &r.EndAt, &r.PadBefore, &r.PadAfter, &r.Status, &r.Path, &r.Size, &r.Error,
		&r.StartedAt, &r.FinishedAt, &r.CreatedAt, &r.FileID, &r.RuleID, &r.OwnerID)
	r.Categories = SplitGenres(cats)
	return r, err
}

func (d *DB) queryRecordings(ctx context.Context, where string, args ...any) ([]Recording, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT `+recCols+` FROM recordings r `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Recording{}
	for rows.Next() {
		r, err := scanRecording(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) Recordings(ctx context.Context) ([]Recording, error) {
	return d.queryRecordings(ctx, `WHERE r.status <> 'cancelled' ORDER BY
		CASE r.status WHEN 'recording' THEN 0 WHEN 'scheduled' THEN 1 ELSE 2 END,
		CASE WHEN r.status = 'scheduled' THEN r.start_at ELSE -r.start_at END LIMIT 500`)
}

func (d *DB) Recording(ctx context.Context, id int64) (Recording, error) {
	r, err := scanRecording(d.sql.QueryRowContext(ctx, `SELECT `+recCols+` FROM recordings r WHERE r.id = ?`, id))
	return r, notFound(err)
}

// DueRecordings are scheduled recordings whose padded window has started.
func (d *DB) DueRecordings(ctx context.Context, now int64) ([]Recording, error) {
	return d.queryRecordings(ctx, `WHERE r.status = 'scheduled' AND r.start_at - r.pad_before <= ? ORDER BY r.start_at`, now)
}

func (d *DB) RecordingsWithStatus(ctx context.Context, status string) ([]Recording, error) {
	return d.queryRecordings(ctx, `WHERE r.status = ?`, status)
}

// RecordingPathActive reports whether a scheduled or in-progress recording
// writes to path.
func (d *DB) RecordingPathActive(ctx context.Context, path string) (bool, error) {
	var n int
	err := d.sql.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM recordings WHERE path = ? AND status IN ('scheduled', 'recording'))`, path).Scan(&n)
	return n > 0, err
}

// ActiveWindows lists the padded [start, end) windows of scheduled and
// in-progress recordings that overlap [from, to).
func (d *DB) ActiveWindows(ctx context.Context, from, to int64) ([][2]int64, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT start_at - pad_before, end_at + pad_after FROM recordings
		WHERE status IN ('scheduled', 'recording') AND start_at - pad_before < ? AND end_at + pad_after > ?`, to, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][2]int64
	for rows.Next() {
		var w [2]int64
		if err := rows.Scan(&w[0], &w[1]); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// Overlapping counts active recordings whose padded windows overlap [from, to).
func (d *DB) Overlapping(ctx context.Context, from, to int64, excludeID int64) (int, error) {
	var n int
	err := d.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM recordings WHERE status IN ('scheduled', 'recording')
		AND id <> ? AND start_at - pad_before < ? AND end_at + pad_after > ?`, excludeID, to, from).Scan(&n)
	return n, err
}

// ScheduleRecording creates (or revives a cancelled/failed) recording for a program.
func (d *DB) ScheduleRecording(ctx context.Context, r Recording) (int64, error) {
	var id int64
	err := d.sql.QueryRowContext(ctx, `INSERT INTO recordings (channel, channel_name, title, episode_title, episode_num,
		synopsis, image_url, series_id, categories, start_at, end_at, pad_before, pad_after)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (channel, start_at) DO UPDATE SET status = CASE WHEN recordings.status IN ('cancelled', 'failed')
		    THEN 'scheduled' ELSE recordings.status END,
		  title = excluded.title, episode_title = excluded.episode_title, episode_num = excluded.episode_num,
		  synopsis = excluded.synopsis, image_url = excluded.image_url, end_at = excluded.end_at,
		  pad_before = excluded.pad_before, pad_after = excluded.pad_after, error = ''
		RETURNING id`,
		r.Channel, r.ChannelName, r.Title, r.EpisodeTitle, r.EpisodeNum, r.Synopsis, r.ImageURL, r.SeriesID,
		strings.Join(r.Categories, ","), r.StartAt, r.EndAt, r.PadBefore, r.PadAfter).Scan(&id)
	return id, err
}

// MarkRecording moves a scheduled recording to recording. It reports false
// when the row is no longer scheduled (cancelled while it was starting).
func (d *DB) MarkRecording(ctx context.Context, id int64, path string) (bool, error) {
	res, err := d.sql.ExecContext(ctx, `UPDATE recordings SET status = 'recording', path = ?, started_at = unixepoch(), error = ''
		WHERE id = ? AND status = 'scheduled'`, path, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (d *DB) FinishRecording(ctx context.Context, id int64, status, path string, size int64, errMsg string) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE recordings SET status = ?, path = ?, size = ?, error = ?, finished_at = unixepoch()
		WHERE id = ?`, status, path, size, errMsg, id)
	return err
}

func (d *DB) SetRecordingError(ctx context.Context, id int64, msg string) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE recordings SET error = ? WHERE id = ?`, msg, id)
	return err
}

func (d *DB) CancelRecording(ctx context.Context, id int64) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE recordings SET status = 'cancelled' WHERE id = ? AND status = 'scheduled'`, id)
	return err
}

func (d *DB) DeleteRecording(ctx context.Context, id int64) error {
	_, err := d.sql.ExecContext(ctx, `DELETE FROM recordings WHERE id = ?`, id)
	return err
}

// LibraryByPath finds a library by its root folder.
func (d *DB) LibraryByPath(ctx context.Context, path string) (*Library, error) {
	l, err := scanLibrary(d.sql.QueryRowContext(ctx, `SELECT `+libraryCols+` FROM libraries l WHERE l.path = ?`, path))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &l, err
}
