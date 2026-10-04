package db

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Access is what the profile a request acts as may see. The zero value sees
// everything (admins, and background work).
type Access struct {
	Libraries []int64 // only these libraries; empty = all
	MaxRating string  // nothing rated above this; "" = any
}

type accessKey struct{}

// WithAccess limits the titles and files that queries made with ctx return.
func WithAccess(ctx context.Context, a Access) context.Context {
	return context.WithValue(ctx, accessKey{}, a)
}

// Ratings a profile can be limited to, mildest first.
var RatingLimits = []string{"G", "PG", "PG-13", "R"}

// ratingLevel puts a limit on the scale ratingLevelSQL uses; 0 = no limit.
func ratingLevel(limit string) int {
	for i, r := range RatingLimits {
		if r == limit {
			return i + 1
		}
	}
	return 0
}

// ratingLevelSQL maps a title's rating (film and TV systems) to a level: 1
// for everyone up to 4 for adults. Anything else (unrated, not matched) is 5,
// so a limited profile isn't shown what nobody has rated.
func ratingLevelSQL(col string) string {
	return `CASE WHEN ` + col + ` IN ('G', 'TV-Y', 'TV-Y7', 'TV-Y7-FV', 'TV-G') THEN 1
		WHEN ` + col + ` IN ('PG', 'TV-PG') THEN 2
		WHEN ` + col + ` IN ('PG-13', 'TV-14') THEN 3
		WHEN ` + col + ` IN ('R', 'TV-MA') THEN 4 ELSE 5 END`
}

// visible is a SQL condition (starting " AND ") that keeps only the titles
// ctx's profile may see, for a media_items row aliased m. "" when there's no
// limit. The values are an int list and a number, so formatting them into the
// SQL is safe and leaves callers' positional arguments alone.
func visible(ctx context.Context, m string) string {
	a, _ := ctx.Value(accessKey{}).(Access)
	cond := ""
	if len(a.Libraries) > 0 {
		ids := make([]string, len(a.Libraries))
		for i, id := range a.Libraries {
			ids[i] = strconv.FormatInt(id, 10)
		}
		cond += fmt.Sprintf(" AND %s.library_id IN (%s)", m, strings.Join(ids, ","))
	}
	if lvl := ratingLevel(a.MaxRating); lvl > 0 {
		cond += fmt.Sprintf(" AND (%s) <= %d", ratingLevelSQL(m+".rated"), lvl)
	}
	return cond
}

// visibleFileJoin limits files f to those of titles ctx's profile may see.
func visibleFileJoin(ctx context.Context) string {
	cond := visible(ctx, "mv")
	if cond == "" {
		return ""
	}
	return ` JOIN media_items mv ON mv.id = f.media_item_id` + cond + ` `
}

// ProfileAccess is a profile's limits as stored (not applied to admins).
func (d *DB) ProfileAccess(ctx context.Context, profileID int64) (Access, error) {
	var a Access
	if err := d.sql.QueryRowContext(ctx, `SELECT max_rating FROM profiles WHERE id = ?`, profileID).Scan(&a.MaxRating); err != nil {
		return a, notFound(err)
	}
	rows, err := d.sql.QueryContext(ctx, `SELECT library_id FROM profile_libraries WHERE profile_id = ? ORDER BY library_id`, profileID)
	if err != nil {
		return a, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return a, err
		}
		a.Libraries = append(a.Libraries, id)
	}
	return a, rows.Err()
}

// SetProfileAccess replaces a profile's limits.
func (d *DB) SetProfileAccess(ctx context.Context, profileID int64, a Access) error {
	if a.MaxRating != "" && ratingLevel(a.MaxRating) == 0 {
		return fmt.Errorf("unknown rating limit %q", a.MaxRating)
	}
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE profiles SET max_rating = ? WHERE id = ?`, a.MaxRating, profileID)
	if err := affected(res, err); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM profile_libraries WHERE profile_id = ?`, profileID); err != nil {
		return err
	}
	for _, lib := range a.Libraries {
		// A library that's gone is skipped rather than failing the save.
		if _, err := tx.ExecContext(ctx, `INSERT INTO profile_libraries (profile_id, library_id)
			SELECT ?, id FROM libraries WHERE id = ?`, profileID, lib); err != nil {
			return err
		}
	}
	return tx.Commit()
}
