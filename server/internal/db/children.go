package db

import (
	"context"
	"errors"
	"strconv"
)

// ChildItem is a title filed under another, with the file its tile plays.
type ChildItem struct {
	ItemSummary
	FileID      int64   `json:"fileId"` // its best copy (first part), playable ones first; 0 when it has none
	Problem     string  `json:"problem"` // that file's problem (unreadable, no-video), "" when it plays
	DurationSec float64 `json:"durationSec"`
	PositionSec float64 `json:"positionSec"` // the profile's progress in that file
	Watched     bool    `json:"watched"`
}

// ChildItems lists the titles filed under a parent, by group, track number
// and title, for the parent's page.
func (d *DB) ChildItems(ctx context.Context, parentID int64) ([]ChildItem, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT `+summaryCols(ctx)+`,
		COALESCE(bf.id, 0), COALESCE(bf.problem, ''), COALESCE(bf.duration_sec, 0), COALESCE(w.position_sec, 0), COALESCE(w.watched, 0)
		FROM media_items m
		LEFT JOIN files bf ON bf.id = (SELECT f.id FROM files f WHERE f.media_item_id = m.id AND f.role <> 'extra'
			ORDER BY f.problem <> '', f.part_no, f.size DESC LIMIT 1)
		LEFT JOIN watch_state w ON w.file_id = bf.id AND w.profile_id = `+strconv.FormatInt(ProfileID(ctx), 10)+`
		WHERE m.parent_id = ?`+visible(ctx, "m")+`
		ORDER BY m.group_label, m.sort_no, m.sort_title`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChildItem{}
	for rows.Next() {
		var c ChildItem
		if err := rows.Scan(scanSummary(&c.ItemSummary, &c.FileID, &c.Problem, &c.DurationSec, &c.PositionSec, &c.Watched)...); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SetDetectedParent files a title under the parent the scan read from its
// path (nil: it isn't nested), unless a parent was chosen by hand. A title
// with a file outside the parent's folder (under, the folder's path with a
// trailing separator; "" skips the check) is a movie in its own right that
// happens to be on the disc too, and is left alone.
func (d *DB) SetDetectedParent(ctx context.Context, id int64, parentID *int64, group string, sortNo int, under string) error {
	if parentID != nil && *parentID == id {
		return errors.New("a title can't be filed under itself")
	}
	_, err := d.sql.ExecContext(ctx, `UPDATE media_items SET parent_id = ?, group_label = ?, sort_no = ?, updated_at = unixepoch()
		WHERE id = ? AND kind = 'movie' AND parent_pinned = 0
		  AND (parent_id IS NOT ? OR group_label <> ? OR sort_no <> ?)
		  AND (? = '' OR NOT EXISTS (SELECT 1 FROM files f WHERE f.media_item_id = media_items.id AND substr(f.path, 1, length(?)) <> ?))`,
		parentID, group, sortNo, id, parentID, group, sortNo, under, under, under)
	return err
}

// SetParent files a title under another by hand (File under), or takes it
// out (nil: Unfile). Either sticks through scans. Only movies, one level
// deep: a title with children can't be filed, and a child can't be a parent.
func (d *DB) SetParent(ctx context.Context, id int64, parentID *int64) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var kind string
	var children int
	if err := tx.QueryRowContext(ctx, `SELECT kind, (SELECT COUNT(*) FROM media_items c WHERE c.parent_id = m.id) FROM media_items m WHERE id = ?`, id).Scan(&kind, &children); err != nil {
		return notFound(err)
	}
	if kind != "movie" {
		return errors.New("only movies can be filed under another title")
	}
	if parentID != nil {
		if *parentID == id {
			return errors.New("a title can't be filed under itself")
		}
		if children > 0 {
			return errors.New("this title has titles filed under it; unfile those first")
		}
		var pkind string
		var pparent *int64
		if err := tx.QueryRowContext(ctx, `SELECT kind, parent_id FROM media_items WHERE id = ?`, *parentID).Scan(&pkind, &pparent); err != nil {
			return notFound(err)
		}
		if pkind != "movie" {
			return errors.New("a movie can only be filed under another movie")
		}
		if pparent != nil {
			return errors.New("that title is itself filed under another; pick the one above it")
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE media_items SET parent_id = ?, parent_pinned = 1, group_label = '', updated_at = unixepoch() WHERE id = ?`, parentID, id); err != nil {
		return err
	}
	if parentID != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE media_items SET updated_at = unixepoch() WHERE id = ?`, *parentID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteEmptyItem removes a title that has no files of its own (a box set
// whose discs were unfiled, or deleted from the Movies page). Titles filed
// under it become movies in their own right.
func (d *DB) DeleteEmptyItem(ctx context.Context, id int64) error {
	res, err := d.sql.ExecContext(ctx, `DELETE FROM media_items WHERE id = ? AND NOT EXISTS (SELECT 1 FROM files f WHERE f.media_item_id = media_items.id)`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
