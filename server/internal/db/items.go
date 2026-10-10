package db

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"sync"
)

// ItemSummary is the lightweight shape used by grids and rows.
type ItemSummary struct {
	ID           int64    `json:"id"`
	Kind         string   `json:"kind"`
	Title        string   `json:"title"`
	SortTitle    string   `json:"sortTitle"`
	Year         *int     `json:"year"`
	Genres       []string `json:"genres"`
	Rating       *float64 `json:"rating"`
	RuntimeMin   *int     `json:"runtimeMin"`
	HasPoster    bool     `json:"hasPoster"`
	HasBackdrop  bool     `json:"hasBackdrop"`
	MatchStatus  string   `json:"matchStatus"`
	AddedAt      int64    `json:"addedAt"`
	UpdatedAt    int64    `json:"updatedAt"`
	FileCount    int      `json:"fileCount"` // extras aren't counted
	WatchedCount int      `json:"watchedCount"`
	InWatchlist  bool     `json:"inWatchlist"` // on the profile's "My list"
	LastAddedAt  int64    `json:"lastAddedAt"`
	// DynamicRange is the best among the title's files (extras aside): dv,
	// hdr10, hlg or "" (SDR or not known), for a badge.
	DynamicRange string `json:"dynamicRange"`
}

// Item is the full detail record.
type Item struct {
	ItemSummary
	LibraryID    int64     `json:"libraryId"`
	ParsedTitle  string    `json:"parsedTitle"`
	ParsedYear   int       `json:"parsedYear"`
	Plot         string    `json:"plot"`
	Rated        string    `json:"rated"`
	ImdbID       string    `json:"imdbId"`
	ImdbPinned   bool      `json:"imdbPinned"` // set by "Fix match"; automatic re-matches keep it
	TotalSeasons *int      `json:"totalSeasons"`
	PosterURL    string    `json:"-"`
	BackdropURL  string    `json:"-"`
	Provider     string    `json:"matchProvider"` // tmdb | omdb | "" (where the rating and details came from)
	Overrides    Overrides `json:"overrides"`     // details set by hand (Edit details)
}

// summaryCols counts watched files, and says whether the title is on "My
// list", for the profile in ctx.
func summaryCols(ctx context.Context) string {
	return `m.id, m.kind, m.title, m.sort_title, m.year, m.genres, m.rating, m.runtime_min,
	m.has_poster, m.has_backdrop, m.match_status, m.added_at, m.updated_at,
	(SELECT COUNT(*) FROM files f WHERE f.media_item_id = m.id AND f.role <> 'extra'),
	(SELECT COUNT(*) FROM files f ` + watchJoin(ctx) + ` WHERE f.media_item_id = m.id AND f.role <> 'extra' AND w.watched = 1),
	EXISTS (SELECT 1 FROM profile_items pi WHERE pi.item_id = m.id AND pi.profile_id = ` + strconv.FormatInt(ProfileID(ctx), 10) + `),
	COALESCE((SELECT MAX(f.added_at) FROM files f WHERE f.media_item_id = m.id), m.added_at) AS last_added,
	COALESCE((SELECT MAX(CASE f.dynamic_range WHEN 'dv' THEN 3 WHEN 'hdr10' THEN 2 WHEN 'hlg' THEN 1 ELSE 0 END)
		FROM files f WHERE f.media_item_id = m.id AND f.role <> 'extra'), 0)`
}

// summaryListCols and summaryListFrom are summaryCols for whole-kind lists:
// the per-title subqueries (five index probes per title) become one pass
// over files grouped by title. Same columns in the same order, so
// scanSummary reads both. Use them together; callers add WHERE and ORDER BY
// on m. Single titles and short IN lists stay on summaryCols, which SQLite
// answers faster there.
func summaryListCols() string {
	return `m.id, m.kind, m.title, m.sort_title, m.year, m.genres, m.rating, m.runtime_min,
	m.has_poster, m.has_backdrop, m.match_status, m.added_at, m.updated_at,
	COALESCE(fs.file_count, 0), COALESCE(fs.watched_count, 0),
	pi.item_id IS NOT NULL,
	COALESCE(fs.last_added, m.added_at) AS last_added,
	COALESCE(fs.range_rank, 0)`
}

func summaryListFrom(ctx context.Context) string {
	return ` FROM media_items m
	LEFT JOIN (SELECT f.media_item_id AS item_id,
		SUM(CASE WHEN f.role <> 'extra' THEN 1 ELSE 0 END) AS file_count,
		SUM(CASE WHEN f.role <> 'extra' AND w.watched = 1 THEN 1 ELSE 0 END) AS watched_count,
		MAX(f.added_at) AS last_added,
		MAX(CASE WHEN f.role = 'extra' THEN 0 WHEN f.dynamic_range = 'dv' THEN 3 WHEN f.dynamic_range = 'hdr10' THEN 2
			WHEN f.dynamic_range = 'hlg' THEN 1 ELSE 0 END) AS range_rank
		FROM files f ` + watchJoin(ctx) + ` GROUP BY f.media_item_id) fs ON fs.item_id = m.id
	LEFT JOIN profile_items pi ON pi.item_id = m.id AND pi.profile_id = ` + strconv.FormatInt(ProfileID(ctx), 10)
}

func scanSummary(dest *ItemSummary, extra ...any) []any {
	return append([]any{&dest.ID, &dest.Kind, &dest.Title, &dest.SortTitle, &dest.Year, &genreScanner{&dest.Genres}, &dest.Rating,
		&dest.RuntimeMin, &dest.HasPoster, &dest.HasBackdrop, &dest.MatchStatus, &dest.AddedAt, &dest.UpdatedAt,
		&dest.FileCount, &dest.WatchedCount, &dest.InWatchlist, &dest.LastAddedAt, rangeScanner{&dest.DynamicRange}}, extra...)
}

// rangeScanner reads summaryCols' range rank (3 dv, 2 hdr10, 1 hlg, 0 SDR) as its name.
type rangeScanner struct{ dest *string }

func (r rangeScanner) Scan(v any) error {
	n, _ := v.(int64)
	*r.dest = [...]string{"", "hlg", "hdr10", "dv"}[min(max(n, 0), 3)]
	return nil
}

// Watchlist is the profile's "My list", newest first.
func (d *DB) Watchlist(ctx context.Context, limit int) ([]ItemSummary, error) {
	return d.querySummaries(ctx, `SELECT `+summaryCols(ctx)+` FROM media_items m
		JOIN profile_items pl ON pl.item_id = m.id AND pl.profile_id = ?
		WHERE 1 = 1`+visible(ctx, "m")+`
		ORDER BY pl.added_at DESC, m.id DESC LIMIT ?`, ProfileID(ctx), limit)
}

// SetWatchlist adds a title to the profile's "My list", or takes it off.
func (d *DB) SetWatchlist(ctx context.Context, itemID int64, on bool) error {
	q := `DELETE FROM profile_items WHERE profile_id = ? AND item_id = ?`
	if on {
		q = `INSERT OR IGNORE INTO profile_items (profile_id, item_id) VALUES (?, ?)`
	}
	_, err := d.sql.ExecContext(ctx, q, ProfileID(ctx), itemID)
	return err
}

// genreScanner splits the comma-separated genres column into a slice.
type genreScanner struct{ out *[]string }

func (g *genreScanner) Scan(src any) error {
	var s string
	switch v := src.(type) {
	case string:
		s = v
	case []byte:
		s = string(v)
	}
	*g.out = SplitGenres(s)
	return nil
}

func SplitGenres(s string) []string {
	out := []string{}
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Items lists every item of a kind. The client filters and sorts: a home
// library is a few thousand rows, and doing it client-side keeps the grid instant.
func (d *DB) Items(ctx context.Context, kind string) ([]ItemSummary, error) {
	return d.querySummaries(ctx, `SELECT `+summaryListCols()+summaryListFrom(ctx)+` WHERE m.kind = ?`+visible(ctx, "m")+` ORDER BY m.sort_title`, kind)
}

// RecentItems returns items of a kind ordered by newest file.
func (d *DB) RecentItems(ctx context.Context, kind string, limit int) ([]ItemSummary, error) {
	return d.querySummaries(ctx, `SELECT `+summaryListCols()+summaryListFrom(ctx)+` WHERE m.kind = ?`+visible(ctx, "m")+`
		ORDER BY last_added DESC, m.id DESC LIMIT ?`, kind, limit)
}

func (d *DB) querySummaries(ctx context.Context, q string, args ...any) ([]ItemSummary, error) {
	rows, err := d.sql.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ItemSummary{}
	for rows.Next() {
		var s ItemSummary
		if err := rows.Scan(scanSummary(&s)...); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (d *DB) Item(ctx context.Context, id int64) (Item, error) {
	var it Item
	var raw string
	err := d.sql.QueryRowContext(ctx, `SELECT `+summaryCols(ctx)+`, m.library_id, m.parsed_title, m.parsed_year,
		m.plot, m.rated, m.imdb_id, m.imdb_pinned, m.total_seasons, m.poster_url, m.backdrop_url, m.match_provider, m.overrides
		FROM media_items m WHERE m.id = ?`+visible(ctx, "m"), id).
		Scan(scanSummary(&it.ItemSummary, &it.LibraryID, &it.ParsedTitle, &it.ParsedYear, &it.Plot, &it.Rated,
			&it.ImdbID, &it.ImdbPinned, &it.TotalSeasons, &it.PosterURL, &it.BackdropURL, &it.Provider, &raw)...)
	it.Overrides = parseOverrides(raw)
	return it, notFound(err)
}

// EnsureItem finds or creates the item for a parsed title, reporting whether it was created.
func (d *DB) EnsureItem(ctx context.Context, libraryID int64, kind, title string, year int) (int64, bool, error) {
	// One title is one item whichever libraries hold files for it, so look
	// across them first. The library a title was first seen in stays its
	// home (Manage view, counts); its files each know their own.
	ensureMu.Lock()
	defer ensureMu.Unlock()
	var id int64
	err := d.sql.QueryRowContext(ctx, `SELECT id FROM media_items WHERE kind = ? AND parsed_title = ? AND parsed_year = ?
		ORDER BY id LIMIT 1`, kind, title, year).Scan(&id)
	if err == nil {
		return id, false, nil
	}
	if err != sql.ErrNoRows {
		return 0, false, err
	}
	err = d.sql.QueryRowContext(ctx, `INSERT INTO media_items (library_id, kind, parsed_title, parsed_year, title, sort_title, year)
		VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, 0)) RETURNING id`,
		libraryID, kind, title, year, title, SortTitle(title), year).Scan(&id)
	return id, err == nil, err
}

// ensureMu keeps two scans from creating the same title at once: the table's
// unique key is per library, and titles are shared across them.
var ensureMu sync.Mutex

// itemsMu keeps the tidy-up after files go (tidyItems) from deleting an item
// or episode a scan has just made for a file it hasn't written yet, which
// failed that scan with a foreign key error when two libraries scanned at
// once. Indexers hold it shared (HoldItems) from EnsureItem to UpsertFile;
// the deletes that tidy hold it alone, taken before their transaction.
var itemsMu sync.RWMutex

// HoldItems keeps items and episodes from being tidied away until release is
// called; hold it from making a file's item and episode until its row is written.
func (d *DB) HoldItems() (release func()) {
	itemsMu.RLock()
	return itemsMu.RUnlock
}

// Metadata is what a provider contributes to an item.
type Metadata struct {
	Title        string
	Year         int
	Plot         string
	Genres       []string
	Rated        string
	Rating       *float64
	RuntimeMin   *int
	ImdbID       string
	TotalSeasons *int
	PosterURL    string
	BackdropURL  string
	Provider     string
	// Partial: a provider earlier in the chain couldn't be reached, so this
	// answer may be missing what only that one has. The item keeps its
	// backdrop link when this answer has none, and stays pending so the next
	// scan matches it again.
	Partial bool
}

func (d *DB) ApplyMetadata(ctx context.Context, id int64, m Metadata) error {
	// Details set by hand win over the provider's.
	if o, err := d.ItemOverrides(ctx, id); err == nil {
		o.apply(&m)
	}
	_, err := d.sql.ExecContext(ctx, `UPDATE media_items SET title = ?, sort_title = ?, year = NULLIF(?, 0), plot = ?,
		genres = ?, rated = ?, rating = ?, runtime_min = ?, imdb_id = ?, total_seasons = ?, poster_url = ?,
		backdrop_url = CASE WHEN ? AND ? = '' THEN backdrop_url ELSE ? END,
		match_status = CASE WHEN ? THEN 'pending' ELSE 'matched' END, match_provider = ?, updated_at = unixepoch() WHERE id = ?`,
		m.Title, SortTitle(m.Title), m.Year, m.Plot, strings.Join(m.Genres, ", "), m.Rated, m.Rating, m.RuntimeMin,
		m.ImdbID, m.TotalSeasons, m.PosterURL, m.Partial, m.BackdropURL, m.BackdropURL, m.Partial, m.Provider, id)
	return err
}

// ClearMatch marks an item unmatched and drops metadata from any earlier
// (wrong) match, so it shows its filename title and a placeholder again.
func (d *DB) ClearMatch(ctx context.Context, id int64) error {
	var parsed string
	if err := d.sql.QueryRowContext(ctx, `SELECT parsed_title FROM media_items WHERE id = ?`, id).Scan(&parsed); err != nil {
		return notFound(err)
	}
	if _, err := d.sql.ExecContext(ctx, `DELETE FROM item_credits WHERE item_id = ?`, id); err != nil {
		return err
	}
	if _, err := d.sql.ExecContext(ctx, `UPDATE media_items SET match_status = 'unmatched', title = parsed_title,
		sort_title = ?, year = NULLIF(parsed_year, 0), plot = '', genres = '', rated = '', rating = NULL,
		runtime_min = NULL, total_seasons = NULL, poster_url = '', backdrop_url = '', has_poster = custom_poster, match_provider = '',
		imdb_id = CASE WHEN imdb_pinned = 1 THEN imdb_id ELSE '' END, updated_at = unixepoch()
		WHERE id = ?`, SortTitle(parsed), id); err != nil {
		return err
	}
	// What was set by hand stays, matched or not.
	if o, err := d.ItemOverrides(ctx, id); err == nil && !o.empty() {
		_, err = d.SetItemDetails(ctx, id, o)
		return err
	}
	return nil
}

// SetImdbOverride pins (or clears, with "") the IMDb id used for the next match.
func (d *DB) SetImdbOverride(ctx context.Context, id int64, imdbID string) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE media_items SET imdb_id = ?, imdb_pinned = (? <> ''), match_status = 'pending',
		updated_at = unixepoch() WHERE id = ?`, imdbID, imdbID, id)
	return err
}

func (d *DB) SetArtwork(ctx context.Context, id int64, poster, backdrop bool) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE media_items SET has_poster = ?, has_backdrop = ?, updated_at = unixepoch() WHERE id = ?`, poster, backdrop, id)
	return err
}

// SortTitle drops a leading article so "The Matrix" files under M.
func SortTitle(t string) string {
	l := strings.ToLower(strings.TrimSpace(t))
	for _, a := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(l, a) && len(l) > len(a) {
			return l[len(a):]
		}
	}
	return l
}

type Counts struct {
	Movies    int `json:"movies"`
	Series    int `json:"series"`
	Episodes  int `json:"episodes"`
	Unmatched int `json:"unmatched"`
	Libraries int `json:"libraries"`
}

func (d *DB) Counts(ctx context.Context) (Counts, error) {
	var c Counts
	err := d.sql.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM media_items WHERE kind = 'movie'),
		(SELECT COUNT(*) FROM media_items WHERE kind = 'series'),
		(SELECT COUNT(*) FROM episodes e WHERE EXISTS (SELECT 1 FROM files f WHERE f.episode_id = e.id)),
		(SELECT COUNT(*) FROM media_items WHERE match_status = 'unmatched'),
		(SELECT COUNT(*) FROM libraries)`).Scan(&c.Movies, &c.Series, &c.Episodes, &c.Unmatched, &c.Libraries)
	return c, err
}
