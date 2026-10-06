package db

import (
	"context"
	"encoding/json"
	"strings"
)

// Overrides are details an admin set by hand on a title. A nil field means
// "whatever the provider says". They're applied to the row when saved and
// again after every match, so a re-match never undoes them.
type Overrides struct {
	Title  *string   `json:"title,omitempty"`
	Year   *int      `json:"year,omitempty"`
	Plot   *string   `json:"plot,omitempty"`
	Genres *[]string `json:"genres,omitempty"`
	Rated  *string   `json:"rated,omitempty"`
}

func (o Overrides) empty() bool {
	return o.Title == nil && o.Year == nil && o.Plot == nil && o.Genres == nil && o.Rated == nil
}

// apply lays the overrides over provider metadata.
func (o Overrides) apply(m *Metadata) {
	if o.Title != nil {
		m.Title = *o.Title
	}
	if o.Year != nil {
		m.Year = *o.Year
	}
	if o.Plot != nil {
		m.Plot = *o.Plot
	}
	if o.Genres != nil {
		m.Genres = *o.Genres
	}
	if o.Rated != nil {
		m.Rated = *o.Rated
	}
}

func parseOverrides(raw string) Overrides {
	var o Overrides
	_ = json.Unmarshal([]byte(raw), &o)
	return o
}

// ItemOverrides returns what's set by hand on a title.
func (d *DB) ItemOverrides(ctx context.Context, id int64) (Overrides, error) {
	var raw string
	if err := d.sql.QueryRowContext(ctx, `SELECT overrides FROM media_items WHERE id = ?`, id).Scan(&raw); err != nil {
		return Overrides{}, notFound(err)
	}
	return parseOverrides(raw), nil
}

// SetItemDetails stores the overrides and puts them on the row. A field
// cleared here keeps its current value until the next match brings the
// provider's back (callers queue one). It reports whether any field was
// cleared.
func (d *DB) SetItemDetails(ctx context.Context, id int64, o Overrides) (cleared bool, err error) {
	old, err := d.ItemOverrides(ctx, id)
	if err != nil {
		return false, err
	}
	cleared = (old.Title != nil && o.Title == nil) || (old.Year != nil && o.Year == nil) || (old.Plot != nil && o.Plot == nil) ||
		(old.Genres != nil && o.Genres == nil) || (old.Rated != nil && o.Rated == nil)
	raw, err := json.Marshal(o)
	if err != nil {
		return false, err
	}
	if o.empty() {
		raw = []byte("{}")
	}
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE media_items SET overrides = ? WHERE id = ?`, string(raw), id); err != nil {
		return false, err
	}
	set := []string{"updated_at = unixepoch()"}
	var args []any
	if o.Title != nil {
		set = append(set, "title = ?", "sort_title = ?")
		args = append(args, *o.Title, SortTitle(*o.Title))
	}
	if o.Year != nil {
		set = append(set, "year = NULLIF(?, 0)")
		args = append(args, *o.Year)
	}
	if o.Plot != nil {
		set = append(set, "plot = ?")
		args = append(args, *o.Plot)
	}
	if o.Genres != nil {
		set = append(set, "genres = ?")
		args = append(args, strings.Join(*o.Genres, ", "))
	}
	if o.Rated != nil {
		set = append(set, "rated = ?")
		args = append(args, *o.Rated)
	}
	args = append(args, id)
	if _, err := tx.ExecContext(ctx, `UPDATE media_items SET `+strings.Join(set, ", ")+` WHERE id = ?`, args...); err != nil {
		return false, err
	}
	return cleared, tx.Commit()
}
