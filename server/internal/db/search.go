package db

import (
	"context"
	"strings"
)

// SearchRow is one searchable title: a movie or show, an episode (that has a
// file), a channel or an upcoming guide program.
type SearchRow struct {
	Kind    string // movie | series | episode | channel | program
	ID      int64
	Ref     string   // channel number
	Names   []string // what it can be found by, display name first
	StartAt int64    // programs
	EndAt   int64    // programs
}

// SearchRows lists everything search looks through. Channels and programs
// are included when withTV is set; programs only while they haven't ended.
func (d *DB) SearchRows(ctx context.Context, withTV bool, now int64) ([]SearchRow, error) {
	out := []SearchRow{}
	add := func(q string, args []any, scan func(func(...any) error) (SearchRow, error)) error {
		rows, err := d.sql.QueryContext(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			r, err := scan(rows.Scan)
			if err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	}
	if err := add(`SELECT id, kind, title FROM media_items`, nil, func(scan func(...any) error) (SearchRow, error) {
		var r SearchRow
		var title string
		err := scan(&r.ID, &r.Kind, &title)
		r.Names = []string{title}
		return r, err
	}); err != nil {
		return nil, err
	}
	if err := add(`SELECT e.id, e.title FROM episodes e
		WHERE e.title <> '' AND EXISTS (SELECT 1 FROM files f WHERE f.episode_id = e.id)`, nil,
		func(scan func(...any) error) (SearchRow, error) {
			r := SearchRow{Kind: "episode"}
			var title string
			err := scan(&r.ID, &title)
			r.Names = []string{title}
			return r, err
		}); err != nil {
		return nil, err
	}
	if !withTV {
		return out, nil
	}
	if err := add(`SELECT number, name, affiliate FROM channels`, nil, func(scan func(...any) error) (SearchRow, error) {
		r := SearchRow{Kind: "channel"}
		var name, affiliate string
		err := scan(&r.Ref, &name, &affiliate)
		r.Names = []string{name, r.Ref, affiliate}
		return r, err
	}); err != nil {
		return nil, err
	}
	if err := add(`SELECT id, title, episode_title, start_at, end_at FROM programs WHERE end_at > ?`, []any{now},
		func(scan func(...any) error) (SearchRow, error) {
			r := SearchRow{Kind: "program"}
			var title, episode string
			err := scan(&r.ID, &title, &episode, &r.StartAt, &r.EndAt)
			r.Names = []string{title, episode}
			return r, err
		}); err != nil {
		return nil, err
	}
	return out, nil
}

// inList is "(?,?,?)" for n ids, and the ids as args.
func inList(ids []int64) (string, []any) {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return "(?" + strings.Repeat(",?", max(0, len(ids)-1)) + ")", args
}

// ItemsByID returns item summaries in the order of ids, skipping missing ones.
func (d *DB) ItemsByID(ctx context.Context, ids []int64) ([]ItemSummary, error) {
	if len(ids) == 0 {
		return []ItemSummary{}, nil
	}
	in, args := inList(ids)
	items, err := d.querySummaries(ctx, `SELECT `+summaryCols(ctx)+` FROM media_items m WHERE m.id IN `+in+visible(ctx, "m"), args...)
	if err != nil {
		return nil, err
	}
	return inOrder(ids, items, func(it ItemSummary) int64 { return it.ID }), nil
}

// EpisodePlays returns the best file of each episode, in the order of ids,
// in the shape the "continue watching" row uses.
func (d *DB) EpisodePlays(ctx context.Context, episodeIDs []int64) ([]PlayInfo, error) {
	if len(episodeIDs) == 0 {
		return []PlayInfo{}, nil
	}
	in, args := inList(episodeIDs)
	rows, err := d.sql.QueryContext(ctx, `SELECT `+playCols+`, e.id`+playFrom(ctx)+`
		WHERE e.id IN `+in+` AND f.id = (SELECT f2.id FROM files f2 WHERE f2.episode_id = e.id ORDER BY f2.size DESC LIMIT 1)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type play struct {
		PlayInfo
		episode int64
	}
	var plays []play
	for rows.Next() {
		var p play
		pi, err := scanPlay(scanExtra(rows, &p.episode))
		if err != nil {
			return nil, err
		}
		p.PlayInfo = pi
		plays = append(plays, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []PlayInfo{}
	for _, p := range inOrder(episodeIDs, plays, func(p play) int64 { return p.episode }) {
		out = append(out, p.PlayInfo)
	}
	return out, nil
}

// scanExtra lets a scanner that knows its own columns read extra ones after them.
func scanExtra(r interface{ Scan(...any) error }, extra ...any) interface{ Scan(...any) error } {
	return scanFunc(func(dest ...any) error { return r.Scan(append(dest, extra...)...) })
}

type scanFunc func(...any) error

func (f scanFunc) Scan(dest ...any) error { return f(dest...) }

// ProgramsByID returns guide programs in the order of ids.
func (d *DB) ProgramsByID(ctx context.Context, ids []int64) ([]Program, error) {
	if len(ids) == 0 {
		return []Program{}, nil
	}
	in, args := inList(ids)
	rows, err := d.sql.QueryContext(ctx, `SELECT `+programCols+programFrom+`WHERE p.id IN `+in, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var progs []Program
	for rows.Next() {
		p, err := scanProgram(rows)
		if err != nil {
			return nil, err
		}
		progs = append(progs, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return inOrder(ids, progs, func(p Program) int64 { return p.ID }), nil
}

func inOrder[T any](ids []int64, rows []T, id func(T) int64) []T {
	by := make(map[int64]T, len(rows))
	for _, r := range rows {
		by[id(r)] = r
	}
	out := make([]T, 0, len(ids))
	for _, i := range ids {
		if r, ok := by[i]; ok {
			out = append(out, r)
		}
	}
	return out
}
