package db

import (
	"context"
	"database/sql"
	"strings"
)

type SeriesRule struct {
	ID          int64   `json:"id"`
	SeriesID    string  `json:"seriesId"`
	Title       string  `json:"title"`
	ImageURL    ImageURL `json:"imageUrl"`
	Mode        string  `json:"mode"`
	Channel     string  `json:"channel"`
	MediaItemID *int64  `json:"mediaItemId"`
	KeepLast    int     `json:"keepLast"`
	Enabled     bool    `json:"enabled"`
	LastRunAt   *int64  `json:"lastRunAt"`
	LastSummary string  `json:"lastSummary"`
	CreatedAt   int64   `json:"createdAt"`
	LibraryName *string `json:"libraryTitle"` // title of the linked library show
	Scheduled   int     `json:"scheduled"`    // upcoming recordings from this rule
	Recorded    int     `json:"recorded"`     // completed recordings from this rule
	NextAt      *int64  `json:"nextAt"`       // next scheduled start
	OwnerID     int64   `json:"ownerId"`      // profile that made it; 0 = admins only
}

const ruleCols = `r.id, r.series_id, r.title, r.image_url, r.mode, r.channel, r.media_item_id, r.keep_last, r.enabled,
	r.last_run_at, r.last_summary, r.created_at,
	(SELECT m.title || COALESCE(' (' || m.year || ')', '') FROM media_items m WHERE m.id = r.media_item_id),
	(SELECT COUNT(*) FROM recordings x WHERE x.rule_id = r.id AND x.status IN ('scheduled', 'recording')),
	(SELECT COUNT(*) FROM recordings x WHERE x.rule_id = r.id AND x.status = 'completed'),
	(SELECT MIN(x.start_at) FROM recordings x WHERE x.rule_id = r.id AND x.status = 'scheduled'),
	COALESCE(r.profile_id, 0)`

func scanRule(row interface{ Scan(...any) error }) (SeriesRule, error) {
	var r SeriesRule
	err := row.Scan(&r.ID, &r.SeriesID, &r.Title, &r.ImageURL, &r.Mode, &r.Channel, &r.MediaItemID, &r.KeepLast, &r.Enabled,
		&r.LastRunAt, &r.LastSummary, &r.CreatedAt, &r.LibraryName, &r.Scheduled, &r.Recorded, &r.NextAt, &r.OwnerID)
	return r, err
}

func (d *DB) Rules(ctx context.Context) ([]SeriesRule, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT `+ruleCols+` FROM series_rules r ORDER BY lower(r.title)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SeriesRule{}
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) Rule(ctx context.Context, id int64) (SeriesRule, error) {
	r, err := scanRule(d.sql.QueryRowContext(ctx, `SELECT `+ruleCols+` FROM series_rules r WHERE r.id = ?`, id))
	return r, notFound(err)
}

// UpsertRule creates a rule or updates the one for the same series.
func (d *DB) UpsertRule(ctx context.Context, r SeriesRule) (int64, error) {
	var id int64
	err := d.sql.QueryRowContext(ctx, `INSERT INTO series_rules (series_id, title, image_url, mode, channel, media_item_id, keep_last, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1)
		ON CONFLICT (series_id) DO UPDATE SET title = excluded.title, image_url = excluded.image_url, mode = excluded.mode,
		  channel = excluded.channel, media_item_id = excluded.media_item_id, keep_last = excluded.keep_last, enabled = 1
		RETURNING id`, r.SeriesID, r.Title, r.ImageURL, r.Mode, r.Channel, r.MediaItemID, r.KeepLast).Scan(&id)
	return id, err
}

func (d *DB) UpdateRule(ctx context.Context, r SeriesRule) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE series_rules SET mode = ?, channel = ?, media_item_id = ?, keep_last = ?, enabled = ?
		WHERE id = ?`, r.Mode, r.Channel, r.MediaItemID, r.KeepLast, r.Enabled, r.ID)
	return err
}

func (d *DB) DeleteRule(ctx context.Context, id int64) error {
	_, err := d.sql.ExecContext(ctx, `DELETE FROM series_rules WHERE id = ?`, id)
	return err
}

func (d *DB) SetRuleSummary(ctx context.Context, id int64, summary string) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE series_rules SET last_run_at = unixepoch(), last_summary = ? WHERE id = ?`, summary, id)
	return err
}

// CancelRuleRecordings drops a rule's upcoming recordings (running ones continue).
func (d *DB) CancelRuleRecordings(ctx context.Context, ruleID int64) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE recordings SET status = 'cancelled' WHERE rule_id = ? AND status = 'scheduled'`, ruleID)
	return err
}

// UpcomingSeriesPrograms lists future airings of a series, soonest first.
func (d *DB) UpcomingSeriesPrograms(ctx context.Context, seriesID string, after int64) ([]Program, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT `+programCols+programFrom+`WHERE p.series_id = ? AND p.start_at >= ?
		ORDER BY p.start_at, p.channel`, seriesID, after)
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

// RecordedEpisodes returns episode numbers ("S11E04") and "t:title|episode title"
// keys already recorded or scheduled for a series, in any status except
// failed. Cancelled airings count too, so a rule never revives an airing the
// user dropped.
func (d *DB) RecordedEpisodes(ctx context.Context, seriesID string) (map[string]bool, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT episode_num, title, episode_title, status FROM recordings
		WHERE series_id = ? AND status <> 'failed'`, seriesID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var ep, title, epTitle, status string
		if err := rows.Scan(&ep, &title, &epTitle, &status); err != nil {
			return nil, err
		}
		if status == "cancelled" {
			continue // cancelled airings are handled per airing (channel+start), not per episode
		}
		if ep != "" {
			out[strings.ToUpper(ep)] = true
		}
		if epTitle != "" {
			out["t:"+title+"|"+epTitle] = true
		}
	}
	return out, rows.Err()
}

// ScheduleRuleRecording inserts a rule's recording but never revives an
// airing the user cancelled. Returns false when the airing already existed.
func (d *DB) ScheduleRuleRecording(ctx context.Context, r Recording, ruleID int64) (bool, error) {
	res, err := d.sql.ExecContext(ctx, `INSERT INTO recordings (channel, channel_name, title, episode_title, episode_num,
		synopsis, image_url, series_id, categories, start_at, end_at, pad_before, pad_after, rule_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (channel, start_at) DO NOTHING`,
		r.Channel, r.ChannelName, r.Title, r.EpisodeTitle, r.EpisodeNum, r.Synopsis, r.ImageURL, r.SeriesID,
		strings.Join(r.Categories, ","), r.StartAt, r.EndAt, r.PadBefore, r.PadAfter, ruleID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// UnscheduleStale cancels a rule's scheduled recordings that the rule no
// longer wants (mode changed, guide changed, episode now in the library).
func (d *DB) UnscheduleStale(ctx context.Context, ruleID int64, keepIDs []int64) error {
	// Leave anything about to start alone; the scheduler owns it now.
	q := `DELETE FROM recordings WHERE rule_id = ? AND status = 'scheduled' AND start_at - pad_before > unixepoch() + 30`
	args := []any{ruleID}
	if len(keepIDs) > 0 {
		q += ` AND id NOT IN (?` + strings.Repeat(",?", len(keepIDs)-1) + `)`
		for _, id := range keepIDs {
			args = append(args, id)
		}
	}
	_, err := d.sql.ExecContext(ctx, q, args...)
	return err
}

// RecordingAt finds the recording for an airing, if any.
func (d *DB) RecordingAt(ctx context.Context, channel string, start int64) (*Recording, error) {
	r, err := scanRecording(d.sql.QueryRowContext(ctx, `SELECT `+recCols+` FROM recordings r WHERE r.channel = ? AND r.start_at = ?`, channel, start))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &r, err
}

// RuleRecordingsNewestFirst lists a rule's completed recordings, for keep-last-N.
func (d *DB) RuleRecordingsNewestFirst(ctx context.Context, ruleID int64) ([]Recording, error) {
	return d.queryRecordings(ctx, `WHERE r.rule_id = ? AND r.status = 'completed' ORDER BY r.start_at DESC`, ruleID)
}

// LibraryEpisodeKeys returns "S11E04"-style keys and "t:show|episode title"
// keys for every episode of a library show that has a playable file.
func (d *DB) LibraryEpisodeKeys(ctx context.Context, itemID int64) (map[string]bool, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT e.season, e.episode, e.title, m.title FROM episodes e
		JOIN media_items m ON m.id = e.series_id
		WHERE e.series_id = ? AND EXISTS (SELECT 1 FROM files f WHERE f.episode_id = e.id AND f.problem = '')`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var season, ep int
		var title, show string
		if err := rows.Scan(&season, &ep, &title, &show); err != nil {
			return nil, err
		}
		out[episodeKey(season, ep)] = true
		if title != "" {
			out["t:"+show+"|"+title] = true
		}
	}
	return out, rows.Err()
}

// SeriesCandidates finds library shows whose title normalizes to the given one.
func (d *DB) SeriesByTitle(ctx context.Context) ([]ItemSummary, error) {
	return d.querySummaries(ctx, `SELECT `+summaryCols(ctx)+` FROM media_items m WHERE m.kind = 'series'`)
}

func episodeKey(season, ep int) string {
	return "S" + itoa(season) + "E" + itoa(ep)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
