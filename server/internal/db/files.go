package db

import (
	"context"
	"database/sql"
	"strings"
)

type File struct {
	ID             int64    `json:"id"`
	LibraryID      int64    `json:"libraryId"`
	MediaItemID    int64    `json:"mediaItemId"`
	EpisodeID      *int64   `json:"episodeId"`
	Path           string   `json:"path"`
	Size           int64    `json:"size"`
	Mtime          int64    `json:"mtime"`
	DurationSec    *float64 `json:"durationSec"`
	Container      string   `json:"container"`
	VideoCodec     string   `json:"videoCodec"`
	AudioCodec     string   `json:"audioCodec"`
	Width          *int     `json:"width"`
	Height         *int     `json:"height"`
	AudioTracks    int      `json:"audioTracks"`
	SubtitleTracks int      `json:"subtitleTracks"`
	HasStill       bool     `json:"hasStill"`
	AddedAt        int64    `json:"addedAt"`
	PositionSec    float64  `json:"positionSec"`
	Watched        bool     `json:"watched"`
	Optimized      bool     `json:"optimized"`
	Problem        string   `json:"problem"` // '' | unreadable | no-video
	Role           string   `json:"role"`    // copy | part | extra (movies)
	PartNo         int      `json:"partNo"`
	ExtraTitle     string   `json:"extraTitle"`
	RolePinned     bool     `json:"rolePinned"`
	Edition        string   `json:"edition"` // which cut ("Extended"); "" = the ordinary one
	// DynamicRange is dv, hdr10, hlg or "" (SDR, or not read yet); DVProfile
	// is the Dolby Vision profile (5, 7, 8…), 0 when it isn't Dolby Vision.
	DynamicRange string `json:"dynamicRange"`
	DVProfile    int    `json:"dvProfile"`
}

const fileCols = `f.id, f.library_id, f.media_item_id, f.episode_id, f.path, f.size, f.mtime, f.duration_sec,
	f.container, f.video_codec, f.audio_codec, f.width, f.height, f.audio_tracks, f.subtitle_tracks,
	f.has_still, f.added_at, COALESCE(w.position_sec, 0), COALESCE(w.watched, 0),
	EXISTS (SELECT 1 FROM optimized o WHERE o.file_id = f.id), f.problem,
	f.role, f.part_no, f.extra_title, f.role_pinned, f.edition, COALESCE(f.dynamic_range, ''), f.dv_profile`

// fileFrom selects files the profile in ctx may see, with its watch state.
func fileFrom(ctx context.Context) string {
	return ` FROM files f ` + visibleFileJoin(ctx) + watchJoin(ctx)
}

func scanFile(r interface{ Scan(...any) error }) (File, error) {
	var f File
	err := r.Scan(&f.ID, &f.LibraryID, &f.MediaItemID, &f.EpisodeID, &f.Path, &f.Size, &f.Mtime, &f.DurationSec,
		&f.Container, &f.VideoCodec, &f.AudioCodec, &f.Width, &f.Height, &f.AudioTracks, &f.SubtitleTracks,
		&f.HasStill, &f.AddedAt, &f.PositionSec, &f.Watched, &f.Optimized, &f.Problem,
		&f.Role, &f.PartNo, &f.ExtraTitle, &f.RolePinned, &f.Edition, &f.DynamicRange, &f.DVProfile)
	return f, err
}

func (d *DB) queryFiles(ctx context.Context, where string, args ...any) ([]File, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT `+fileCols+fileFrom(ctx)+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []File{}
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (d *DB) File(ctx context.Context, id int64) (File, error) {
	f, err := scanFile(d.sql.QueryRowContext(ctx, `SELECT `+fileCols+fileFrom(ctx)+`WHERE f.id = ?`, id))
	return f, notFound(err)
}

// fileRoleOrder lists parts in order, then copies largest (usually best
// quality) first, then extras by title.
const fileRoleOrder = ` ORDER BY CASE f.role WHEN 'part' THEN 0 WHEN 'copy' THEN 1 ELSE 2 END, f.part_no, f.size DESC, f.extra_title`

// LibraryFiles returns every file in a library.
func (d *DB) LibraryFiles(ctx context.Context, libraryID int64) ([]File, error) {
	return d.queryFiles(ctx, `WHERE f.library_id = ? ORDER BY f.id`, libraryID)
}

// notFailed leaves out files whose job of a kind failed and is still in
// Activity, so a file that can't be done isn't queued again by every scan.
func notFailed(kind string) string {
	return ` AND NOT EXISTS (SELECT 1 FROM jobs j WHERE j.kind = '` + kind + `' AND j.ref_id = f.id AND j.status = 'failed')`
}

// FilesNeedingStills lists a library's playable episode and extra files
// that have no still.
func (d *DB) FilesNeedingStills(ctx context.Context, libraryID int64) ([]File, error) {
	return d.queryFiles(ctx, `WHERE f.library_id = ? AND f.problem = '' AND f.has_still = 0
		AND (f.episode_id IS NOT NULL OR f.role = 'extra')`+notFailed("still")+` ORDER BY f.id`, libraryID)
}

// RecordingsNeedingCommercials lists a library's finished DVR recordings
// that haven't been through commercial detection as they are now.
func (d *DB) RecordingsNeedingCommercials(ctx context.Context, libraryID int64) ([]File, error) {
	return d.queryFiles(ctx, `WHERE f.library_id = ? AND f.problem = ''
		AND EXISTS (SELECT 1 FROM recordings r WHERE r.path = f.path AND r.status = 'completed')
		AND NOT EXISTS (SELECT 1 FROM commercials c WHERE c.file_id = f.id AND c.size = f.size AND c.mtime = f.mtime)`+
		notFailed("commercials")+` ORDER BY f.id`, libraryID)
}

// ItemFiles returns all of an item's files, in fileRoleOrder.
// EpisodeCopies lists an episode's files, best (biggest) first.
func (d *DB) EpisodeCopies(ctx context.Context, episodeID int64) ([]File, error) {
	return d.queryFiles(ctx, `WHERE f.episode_id = ? ORDER BY f.size DESC`, episodeID)
}

func (d *DB) ItemFiles(ctx context.Context, itemID int64) ([]File, error) {
	return d.queryFiles(ctx, `WHERE f.media_item_id = ?`+fileRoleOrder, itemID)
}

// FeatureFiles is ItemFiles without extras: what "watched" means for an item.
func (d *DB) FeatureFiles(ctx context.Context, itemID int64) ([]File, error) {
	return d.queryFiles(ctx, `WHERE f.media_item_id = ? AND f.role <> 'extra'`+fileRoleOrder, itemID)
}

// SetDetectedRole stores the scanner's guess at a file's role, unless one
// was chosen by hand.
func (d *DB) SetDetectedRole(ctx context.Context, id int64, role string, partNo int, extraTitle string) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE files SET role = ?, part_no = ?, extra_title = ?
		WHERE id = ? AND role_pinned = 0`, role, partNo, extraTitle, id)
	return err
}

// SetFileRole stores a role chosen by hand, which later scans keep.
func (d *DB) SetFileRole(ctx context.Context, id int64, role string, partNo int, extraTitle string) error {
	res, err := d.sql.ExecContext(ctx, `UPDATE files SET role = ?, part_no = ?, extra_title = ?, role_pinned = 1
		WHERE id = ?`, role, partNo, extraTitle, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// FileStamp is what the scanner compares to skip unchanged files, plus how
// the file was parsed last time so parser improvements reach existing files.
type FileStamp struct {
	ID          int64
	Size        int64
	Mtime       int64
	ParsedTitle string
	ParsedYear  int
	Season      int
	Episode     int
	Role        string
	PartNo      int
	ExtraTitle  string
	RolePinned  bool
	Problem     string // unreadable | no-video | ""
	Edition     string
	ItemPinned  bool // put under its title by hand (Merge): the scan leaves it there
	MediaItemID int64
}

const stampSelect = `SELECT f.path, f.id, f.size, f.mtime, m.parsed_title, m.parsed_year,
	COALESCE(e.season, 0), COALESCE(e.episode, 0),
	f.role, f.part_no, f.extra_title, f.role_pinned, f.problem, f.edition, f.item_pinned, f.media_item_id
	FROM files f JOIN media_items m ON m.id = f.media_item_id LEFT JOIN episodes e ON e.id = f.episode_id `

func scanStamp(r interface{ Scan(...any) error }, path *string) (*FileStamp, error) {
	var s FileStamp
	err := r.Scan(path, &s.ID, &s.Size, &s.Mtime, &s.ParsedTitle, &s.ParsedYear, &s.Season, &s.Episode,
		&s.Role, &s.PartNo, &s.ExtraTitle, &s.RolePinned, &s.Problem, &s.Edition, &s.ItemPinned, &s.MediaItemID)
	return &s, err
}

func (d *DB) FileStamp(ctx context.Context, path string) (*FileStamp, error) {
	var p string
	s, err := scanStamp(d.sql.QueryRowContext(ctx, stampSelect+`WHERE f.path = ?`, path), &p)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// FileStamps is FileStamp for every file of a library, keyed by path, so a
// scan asks once instead of once per file.
func (d *DB) FileStamps(ctx context.Context, libraryID int64) (map[string]*FileStamp, error) {
	rows, err := d.sql.QueryContext(ctx, stampSelect+`WHERE f.library_id = ?`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*FileStamp{}
	for rows.Next() {
		var p string
		s, err := scanStamp(rows, &p)
		if err != nil {
			return nil, err
		}
		out[p] = s
	}
	return out, rows.Err()
}

func (d *DB) TouchFile(ctx context.Context, id, seen int64) error {
	return d.TouchFiles(ctx, []int64{id}, seen)
}

// TouchFiles marks files seen, many per statement: one write lock and one
// commit for hundreds of unchanged files instead of one each.
func (d *DB) TouchFiles(ctx context.Context, ids []int64, seen int64) error {
	for len(ids) > 0 {
		n := min(len(ids), 500)
		args := make([]any, 0, n+1)
		args = append(args, seen)
		for _, id := range ids[:n] {
			args = append(args, id)
		}
		q := `UPDATE files SET last_seen = ? WHERE id IN (?` + strings.Repeat(",?", n-1) + `)`
		if _, err := d.sql.ExecContext(ctx, q, args...); err != nil {
			return err
		}
		ids = ids[n:]
	}
	return nil
}

// UpsertFile inserts or refreshes a file row keyed by path and returns its id.
func (d *DB) UpsertFile(ctx context.Context, f File, seen int64) (int64, error) {
	var id int64
	// A file that couldn't be read has no known range (NULL), so it's read
	// again once it can be.
	var dr any = f.DynamicRange
	if f.Problem != "" {
		dr = nil
	}
	err := d.sql.QueryRowContext(ctx, `INSERT INTO files (library_id, media_item_id, episode_id, path, size, mtime,
		duration_sec, container, video_codec, audio_codec, width, height, audio_tracks, subtitle_tracks, problem, last_seen,
		dynamic_range, dv_profile)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (path) DO UPDATE SET library_id = excluded.library_id,
		  -- a file merged under another title by hand stays there, whatever its name says
		  media_item_id = CASE WHEN files.item_pinned THEN files.media_item_id ELSE excluded.media_item_id END,
		  episode_id = CASE WHEN files.item_pinned THEN files.episode_id ELSE excluded.episode_id END,
		  size = excluded.size, mtime = excluded.mtime,
		  duration_sec = excluded.duration_sec, container = excluded.container, video_codec = excluded.video_codec,
		  audio_codec = excluded.audio_codec, width = excluded.width, height = excluded.height,
		  audio_tracks = excluded.audio_tracks, subtitle_tracks = excluded.subtitle_tracks,
		  problem = excluded.problem, has_still = 0, last_seen = excluded.last_seen,
		  dynamic_range = excluded.dynamic_range, dv_profile = excluded.dv_profile
		RETURNING id`,
		f.LibraryID, f.MediaItemID, f.EpisodeID, f.Path, f.Size, f.Mtime, f.DurationSec, f.Container, f.VideoCodec,
		f.AudioCodec, f.Width, f.Height, f.AudioTracks, f.SubtitleTracks, f.Problem, seen, dr, f.DVProfile).Scan(&id)
	return id, err
}

// FilesNeedingDynamicRange lists a library's readable files whose dynamic
// range hasn't been read (indexed before it was recorded).
func (d *DB) FilesNeedingDynamicRange(ctx context.Context, libraryID int64) ([]File, error) {
	return d.queryFiles(ctx, `WHERE f.library_id = ? AND f.problem = '' AND f.dynamic_range IS NULL ORDER BY f.id`, libraryID)
}

// SetDynamicRange records a file's dynamic range (see File.DynamicRange).
func (d *DB) SetDynamicRange(ctx context.Context, id int64, dynamicRange string, dvProfile int) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE files SET dynamic_range = ?, dv_profile = ? WHERE id = ?`, dynamicRange, dvProfile, id)
	return err
}

// SetFileEdition records which cut of a film a file is.
func (d *DB) SetFileEdition(ctx context.Context, id int64, edition string) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE files SET edition = ? WHERE id = ?`, edition, id)
	return err
}

// PreferredVersion is the copy of a title the profile in ctx chose to watch
// (0 when it hasn't chosen, or that file has gone).
func (d *DB) PreferredVersion(ctx context.Context, itemID int64) (int64, error) {
	var id int64
	err := d.sql.QueryRowContext(ctx, `SELECT file_id FROM profile_versions WHERE profile_id = ? AND item_id = ?`, ProfileID(ctx), itemID).Scan(&id)
	return id, notFoundOK(err)
}

// SetPreferredVersion remembers which copy of a title the profile in ctx
// watches. The file must be one of the title's.
func (d *DB) SetPreferredVersion(ctx context.Context, itemID, fileID int64) error {
	res, err := d.sql.ExecContext(ctx, `INSERT INTO profile_versions (profile_id, item_id, file_id)
		SELECT ?, f.media_item_id, f.id FROM files f WHERE f.id = ? AND f.media_item_id = ?
		ON CONFLICT (profile_id, item_id) DO UPDATE SET file_id = excluded.file_id`, ProfileID(ctx), fileID, itemID)
	return affected(res, err)
}

func (d *DB) SetFileStill(ctx context.Context, id int64, has bool) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE files SET has_still = ? WHERE id = ?`, has, id)
	return err
}

// PruneLibrary removes files not seen since scanStart (after handing a
// renamed file's data to its new row, carryRenamed), then any items and
// episodes left without files.
func (d *DB) PruneLibrary(ctx context.Context, libraryID, scanStart int64) (Pruned, error) {
	var out Pruned
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if out.Renamed, err = carryRenamed(ctx, tx, libraryID, scanStart); err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM files WHERE library_id = ? AND last_seen < ?`, libraryID, scanStart)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return out, err
		}
		if _, moved := out.Renamed[id]; !moved {
			out.FileIDs = append(out.FileIDs, id)
		}
	}
	rows.Close()
	res, err := tx.ExecContext(ctx, `DELETE FROM files WHERE library_id = ? AND last_seen < ?`, libraryID, scanStart)
	if err != nil {
		return out, err
	}
	out.Files, _ = res.RowsAffected()
	if out.Items, err = tidyItems(ctx, tx); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

// Pruned is what a prune did: the rows removed, the ids of files that are
// gone and of items left without files (their cache folders are the
// caller's to delete), and files whose row moved to a renamed copy, old id
// to new (the caller moves cache/files/<old> to <new>).
type Pruned struct {
	Files   int64
	FileIDs []int64
	Items   []int64
	Renamed map[int64]int64
}

// carryRenamed keeps what belongs to a file across a rename. A file's path
// is its identity, so a renamed file is a new row and the old one is about
// to be pruned, taking every profile's progress, its commercials, intro
// marks and optimized copy with it. A stale row with exactly one twin added
// by this scan (same library, title, episode and size), which no other
// stale row also matches, hands all of that over first. Anything ambiguous
// (two copies of an episode that both moved) is left alone.
func carryRenamed(ctx context.Context, tx *sql.Tx, libraryID, scanStart int64) (map[int64]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT o.id, MIN(n.id) FROM files o
		JOIN files n ON n.library_id = o.library_id AND n.media_item_id = o.media_item_id
		            AND n.episode_id IS o.episode_id AND n.size = o.size AND n.id <> o.id
		            AND n.last_seen >= ?2 AND n.added_at >= ?2
		WHERE o.library_id = ?1 AND o.last_seen < ?2
		GROUP BY o.id HAVING COUNT(*) = 1`, libraryID, scanStart)
	if err != nil {
		return nil, err
	}
	pairs := map[int64]int64{}
	claims := map[int64]int{}
	for rows.Next() {
		var oldID, newID int64
		if err := rows.Scan(&oldID, &newID); err != nil {
			rows.Close()
			return nil, err
		}
		pairs[oldID] = newID
		claims[newID]++
	}
	rows.Close()
	for oldID, newID := range pairs {
		if claims[newID] > 1 {
			delete(pairs, oldID)
		}
	}
	for oldID, newID := range pairs {
		for _, q := range []string{
			`UPDATE OR IGNORE watch_state SET file_id = ?2 WHERE file_id = ?1`,
			`UPDATE OR IGNORE optimized SET file_id = ?2 WHERE file_id = ?1`,
			`UPDATE OR IGNORE commercials SET file_id = ?2 WHERE file_id = ?1`,
			`UPDATE OR IGNORE commercial_dismissals SET file_id = ?2 WHERE file_id = ?1`,
			`UPDATE OR IGNORE file_segments SET file_id = ?2 WHERE file_id = ?1`,
			`UPDATE OR IGNORE segment_checks SET file_id = ?2 WHERE file_id = ?1`,
			`UPDATE OR IGNORE intro_checks SET file_id = ?2 WHERE file_id = ?1`,
			`UPDATE OR IGNORE profile_versions SET file_id = ?2 WHERE file_id = ?1`,
			// Not "recently added", and the still comes along with its folder.
			`UPDATE files SET added_at = (SELECT added_at FROM files WHERE id = ?1),
			     has_still = (SELECT has_still FROM files WHERE id = ?1) WHERE id = ?2`,
			// A role picked by hand stays; a detected one was detected again.
			`UPDATE files SET role = o.role, part_no = o.part_no, extra_title = o.extra_title, role_pinned = 1
			   FROM (SELECT role, part_no, extra_title FROM files WHERE id = ?1 AND role_pinned = 1) AS o
			  WHERE files.id = ?2`,
		} {
			if _, err := tx.ExecContext(ctx, q, oldID, newID); err != nil {
				return nil, err
			}
		}
	}
	return pairs, nil
}

// tidyItems is the housekeeping after files go: items and episodes left
// without files are deleted, and an item whose home library has no files
// for it any more moves home to one that has. It returns the items deleted.
func tidyItems(ctx context.Context, tx *sql.Tx) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM media_items WHERE NOT EXISTS (SELECT 1 FROM files f WHERE f.media_item_id = media_items.id)`)
	if err != nil {
		return nil, err
	}
	gone := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		gone = append(gone, id)
	}
	rows.Close()
	for _, q := range []string{
		`DELETE FROM media_items WHERE NOT EXISTS (SELECT 1 FROM files f WHERE f.media_item_id = media_items.id)`,
		`DELETE FROM episodes WHERE NOT EXISTS (SELECT 1 FROM files f WHERE f.episode_id = episodes.id)`,
		`UPDATE media_items SET library_id = (SELECT f.library_id FROM files f WHERE f.media_item_id = media_items.id ORDER BY f.id LIMIT 1)
		   WHERE NOT EXISTS (SELECT 1 FROM files f WHERE f.media_item_id = media_items.id AND f.library_id = media_items.library_id)`,
	} {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return nil, err
		}
	}
	return gone, nil
}

// BackdropSource picks the file to grab a backdrop frame from: the first
// part or largest copy of a movie (never an extra), the first episode of a series.
func (d *DB) BackdropSource(ctx context.Context, itemID int64) (*File, error) {
	files, err := d.queryFiles(ctx, `LEFT JOIN episodes e ON e.id = f.episode_id WHERE f.media_item_id = ? AND f.problem = '' AND f.role <> 'extra'
		ORDER BY COALESCE(e.season, 0), COALESCE(e.episode, 0), f.part_no, f.size DESC LIMIT 1`, itemID)
	if err != nil || len(files) == 0 {
		return nil, err
	}
	return &files[0], nil
}
