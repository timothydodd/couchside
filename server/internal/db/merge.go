package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// MergeItems puts the files of the from items under into and deletes them.
// Movies: as says what the other films' files become, "copy" (duplicates of
// the same film) or "extra" (bonus material, titled after the film they
// were). Series: episodes move over by season and number. Moved files are
// pinned to their new title, so a scan doesn't sort them back by name.
func (d *DB) MergeItems(ctx context.Context, into int64, from []int64, as string) error {
	if len(from) == 0 {
		return errors.New("nothing to merge")
	}
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var kind string
	if err := tx.QueryRowContext(ctx, `SELECT kind FROM media_items WHERE id = ?`, into).Scan(&kind); err != nil {
		return notFound(err)
	}
	if kind == "movie" && as != "copy" && as != "extra" {
		return errors.New("as must be copy or extra")
	}
	ids := make([]string, 0, len(from))
	for _, id := range from {
		if id == into {
			return errors.New("a title can't be merged into itself")
		}
		var k string
		if err := tx.QueryRowContext(ctx, `SELECT kind FROM media_items WHERE id = ?`, id).Scan(&k); err != nil {
			return notFound(err)
		}
		if k != kind {
			return errors.New("movies and shows can't be merged together")
		}
		ids = append(ids, fmt.Sprint(id))
	}
	in := "(" + strings.Join(ids, ",") + ")"
	if kind == "series" {
		// Episodes the target lacks come over; files then point at the target's row for their number.
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO episodes (series_id, season, episode, title, released, rating, imdb_id)
			SELECT ?, season, episode, title, released, rating, imdb_id FROM episodes WHERE series_id IN `+in, into); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE files SET episode_id = (
			SELECT e2.id FROM episodes e1 JOIN episodes e2 ON e2.series_id = ? AND e2.season = e1.season AND e2.episode = e1.episode
			WHERE e1.id = files.episode_id)
			WHERE media_item_id IN `+in+` AND episode_id IS NOT NULL`, into); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE files SET media_item_id = ?, item_pinned = 1 WHERE media_item_id IN `+in, into); err != nil {
			return err
		}
	} else {
		// Each other film's files become copies or extras of the one kept; an
		// extra is named after the film it was, unless it had its own name.
		if _, err := tx.ExecContext(ctx, `UPDATE files SET
			extra_title = CASE WHEN ? = 'extra' AND role <> 'extra' THEN (SELECT title FROM media_items m WHERE m.id = files.media_item_id) ELSE extra_title END,
			role = CASE WHEN ? = 'extra' THEN 'extra' WHEN role = 'extra' THEN 'extra' ELSE 'copy' END,
			part_no = CASE WHEN ? = 'extra' THEN 0 ELSE part_no END,
			role_pinned = 1, item_pinned = 1, media_item_id = ?
			WHERE media_item_id IN `+in, as, as, as, into); err != nil {
			return err
		}
	}
	// What profiles had on the other titles carries over.
	for _, q := range []string{
		`INSERT OR IGNORE INTO profile_items (profile_id, item_id, added_at) SELECT profile_id, ?, added_at FROM profile_items WHERE item_id IN ` + in,
		`INSERT OR IGNORE INTO home_hidden (profile_id, item_id, hidden_at) SELECT profile_id, ?, hidden_at FROM home_hidden WHERE item_id IN ` + in,
		`INSERT OR IGNORE INTO profile_versions (profile_id, item_id, file_id) SELECT profile_id, ?, file_id FROM profile_versions WHERE item_id IN ` + in,
		`DELETE FROM media_items WHERE id IN ` + in,
		`UPDATE media_items SET updated_at = unixepoch() WHERE id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, into); err != nil {
			return err
		}
	}
	return tx.Commit()
}
