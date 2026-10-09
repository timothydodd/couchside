package worker

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/fsx"
	"github.com/timothydodd/couchside/internal/parse"
	"github.com/timothydodd/couchside/internal/probe"
	"github.com/timothydodd/couchside/internal/usererr"
)

// Folders NAS boxes and download tools litter libraries with.
var skipDirs = map[string]bool{"@eadir": true, "#recycle": true, "$recycle.bin": true, ".trash": true, "lost+found": true}

// errProbeTimeout means ffprobe ran out of time on a file, which says more
// about the share than the file.
var errProbeTimeout = errors.New("ffprobe timed out")

// reSample is a sample clip's name: "sample", or a "-sample" style suffix.
// A word inside a title ("S02E05 - Free Sample") isn't one.
var reSample = regexp.MustCompile(`(?i)(^|[-._])sample$`)

// rePartFile matches the pieces a DVR recording writes before joining them.
var rePartFile = regexp.MustCompile(`\.(part\d+|joining)\.ts$`)

func (w *Worker) scan(ctx context.Context, libID int64) error {
	_, err := w.scanLibrary(ctx, libID)
	return err
}

// Why a scan passed over a file, as Activity says it.
const (
	skipNoEpisode = "no season and episode in the name"
	skipSample    = "sample clips"
	skipTimeout   = "ffprobe timed out (tried again next scan)"
	skipLink      = "links that lead outside the library to something that isn't a video"
)

// skipLog counts skipped files per reason, keeping a few names to show.
type skipLog struct {
	order []string
	count map[string]int
	names map[string][]string
}

func (s *skipLog) add(reason, name string) {
	if s.count == nil {
		s.count, s.names = map[string]int{}, map[string][]string{}
	}
	if s.count[reason] == 0 {
		s.order = append(s.order, reason)
	}
	s.count[reason]++
	if len(s.names[reason]) < 3 {
		s.names[reason] = append(s.names[reason], name)
	}
}

func (s *skipLog) total() int {
	n := 0
	for _, c := range s.count {
		n += c
	}
	return n
}

// scanSummary is a scan's result line in Activity: "Added 2, removed 1.
// Skipped 3: no season and episode in the name (a.mp4, b.mp4 and 1 more)."
func scanSummary(added, changed, removed, renamed int, skips *skipLog) string {
	var parts []string
	for _, c := range []struct {
		n    int
		verb string
	}{{added, "added"}, {changed, "changed"}, {renamed, "renamed"}, {removed, "removed"}} {
		if c.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", c.n, c.verb))
		}
	}
	out := "No changes."
	if len(parts) > 0 {
		j := strings.Join(parts, ", ")
		out = strings.ToUpper(j[:1]) + j[1:] + "."
	}
	for _, r := range skips.order {
		n, names := skips.count[r], skips.names[r]
		list := strings.Join(names, ", ")
		if more := n - len(names); more > 0 {
			list += fmt.Sprintf(" and %d more", more)
		}
		out += fmt.Sprintf(" Skipped %d: %s (%s).", n, r, list)
	}
	return out
}

// scanLibrary scans a library and returns its result line for Activity.
func (w *Worker) scanLibrary(ctx context.Context, libID int64) (string, error) {
	lib, err := w.db.Library(ctx, libID)
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(lib.Path); err != nil || !st.IsDir() {
		return "", fmt.Errorf("library path %s is not readable: %v", lib.Path, err)
	}
	// Every file visited gets last_seen = start; anything older is gone.
	start := time.Now().Unix()
	var added, changed, videos int
	var skips skipLog
	// unreadable is the first path the walk couldn't read. Files under it
	// weren't seen, so nothing may be pruned.
	var unreadable string
	missed := func(path string, err error) {
		slog.Warn("scan: skipping unreadable path", "path", path, "err", err)
		if unreadable == "" {
			unreadable = path
		}
	}

	walkErr := filepath.WalkDir(lib.Path, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			missed(path, err)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if path != lib.Path && (strings.HasPrefix(name, ".") || skipDirs[strings.ToLower(name)]) {
				return fs.SkipDir
			}
			// Movie extras are indexed as extras of their movie; TV has no use for
			// them. A show folder at the top ("Extras", "Shorts") is a show.
			if _, extras := parse.ExtrasDir(name); extras && lib.Kind == "tv" && path != lib.Path && filepath.Dir(path) != lib.Path {
				return fs.SkipDir
			}
			return nil
		}
		if !parse.IsVideo(name) || strings.HasPrefix(name, ".") {
			return nil
		}
		// A recording's parts aren't episodes: they're joined into the final
		// file, which the scan after it finishes picks up. That holds for a
		// recording whose join failed too (its parts wait for Recover).
		if m := rePartFile.FindStringIndex(path); m != nil {
			if owned, err := w.db.RecordingOwnsPath(ctx, path[:m[0]]+".ts"); err != nil || owned {
				return err
			}
		}
		info, err := d.Info()
		if err == nil && d.Type()&fs.ModeSymlink != 0 {
			// A link can point anywhere ("Film.mkv -> /data/auth.key"). Index
			// it only when it leads to a video, and describe the video, not
			// the link.
			if aerr := fsx.Allowed(path, lib.Path, parse.IsVideo); errors.Is(aerr, fsx.ErrRefused) {
				slog.Warn("scan: skipping a link", "path", path)
				skips.add(skipLink, relPath(lib, path))
				return nil
			} else if aerr != nil {
				err = aerr // dangling: treated like a file deleted mid-walk below
			} else {
				info, err = os.Stat(path)
			}
		}
		if errors.Is(err, fs.ErrNotExist) {
			return nil // deleted since the folder was listed (keep-last-N, Manage): it's gone, not unreadable
		}
		videos++
		if err != nil {
			missed(path, err)
			return nil
		}
		if isSample(path) && info.Size() < 300<<20 {
			slog.Debug("scan: skipping a sample clip", "path", path)
			skips.add(skipSample, relPath(lib, path))
			return nil
		}
		stamp, err := w.db.FileStamp(ctx, path)
		if err != nil {
			return err
		}
		// A file once unreadable is probed again: the failure may have been the share, not the file.
		if stamp != nil && stamp.Size == info.Size() && stamp.Mtime == info.ModTime().Unix() && stamp.Problem != "unreadable" &&
			(stamp.ItemPinned || !w.parseChanged(lib, path, stamp)) {
			if err := w.refreshRole(ctx, lib, path, stamp); err != nil {
				return err
			}
			return w.db.TouchFile(ctx, stamp.ID, start)
		}
		ok, err := w.indexFile(ctx, lib, path, info, start)
		if errors.Is(err, errProbeTimeout) {
			// Slow, not broken: leave it for the next scan. A file already
			// indexed keeps its row (and isn't pruned) until then.
			slog.Warn("scan: ffprobe timed out; will try again next scan", "path", path)
			skips.add(skipTimeout, relPath(lib, path))
			if stamp != nil {
				return w.db.TouchFile(ctx, stamp.ID, start)
			}
			return nil
		}
		if err != nil {
			return err
		}
		switch {
		case !ok:
			skips.add(skipNoEpisode, relPath(lib, path))
		case stamp == nil:
			added++
		default:
			changed++
		}
		return nil
	})
	if walkErr != nil {
		// Don't prune after a partial walk: missing files may just be unvisited.
		return "", walkErr
	}
	if unreadable != "" {
		slog.Warn("scan incomplete, nothing removed", "library", lib.Name, "unreadable", unreadable, "added", added, "changed", changed)
		return scanSummary(added, changed, 0, 0, &skips), fmt.Errorf("couldn't read %s, so nothing was removed", unreadable)
	}
	// An unmounted share is usually an empty folder. Someone who really
	// emptied a library removes it in Libraries.
	if videos == 0 && lib.FileCount > 0 {
		slog.Warn("scan found no files, nothing removed", "library", lib.Name, "path", lib.Path)
		return "", fmt.Errorf("library folder %s is empty; is the share mounted? Nothing was removed", lib.Path)
	}
	pr, err := w.db.PruneLibrary(ctx, lib.ID, start)
	if err != nil {
		return "", err
	}
	TidyCache(w.cfg.CacheDir, pr)
	// A renamed file was counted as added; it's neither added nor removed.
	renamed := len(pr.Renamed)
	added -= renamed
	removed := int(pr.Files) - renamed
	slog.Info("scan complete", "library", lib.Name, "added", added, "changed", changed, "removed", removed, "renamed", renamed, "skipped", skips.total())
	summary := scanSummary(added, changed, removed, renamed, &skips)
	if err := w.retryMatches(ctx, lib); err != nil {
		return summary, err
	}
	if err := w.catchUp(ctx, lib); err != nil {
		return summary, err
	}
	return summary, w.db.MarkLibraryScanned(ctx, lib.ID, time.Now().Unix())
}

// TidyCache brings the cache in line with a prune or a library delete. A renamed file's folder
// (its still and preview thumbnails, still valid since a rename keeps size
// and mtime) moves to its new id. A deleted file's folder and optimized copy
// and a deleted title's artwork go: ids are reused, so a folder left behind
// would show the old file's pictures for a new one.
func TidyCache(cacheDir string, pr db.Pruned) {
	if cacheDir == "" {
		return
	}
	for oldID, newID := range pr.Renamed {
		from := filepath.Dir(FileStillPath(cacheDir, oldID))
		to := filepath.Dir(FileStillPath(cacheDir, newID))
		if _, err := os.Stat(from); err != nil {
			continue
		}
		_ = os.RemoveAll(to) // anything the new row has made so far
		if err := os.Rename(from, to); err != nil {
			slog.Warn("couldn't move a renamed file's pictures", "from", from, "to", to, "err", err)
		}
	}
	for _, id := range pr.FileIDs {
		_ = os.RemoveAll(filepath.Dir(FileStillPath(cacheDir, id)))
		_ = os.Remove(OptimizedFile(cacheDir, id))
	}
	for _, id := range pr.Items {
		_ = os.RemoveAll(ItemArtDir(cacheDir, id))
	}
}

// retryMatches queues a match for items whose last one failed on the way to
// the provider (network, rate limit, server error), so a TMDB outage during
// the first scan doesn't leave them unmatched for good.
func (w *Worker) retryMatches(ctx context.Context, lib db.Library) error {
	if w.providers == nil || w.providers.Empty() {
		return nil
	}
	items, err := w.db.PendingMatches(ctx, lib.ID)
	if err != nil {
		return err
	}
	for _, it := range items {
		if err := w.Enqueue(ctx, KindMatch, it.ID, "Match "+it.Title); err != nil {
			return err
		}
	}
	if len(items) > 0 {
		slog.Info("retrying matches that didn't finish", "library", lib.Name, "items", len(items))
	}
	return nil
}

func isSample(path string) bool {
	return reSample.MatchString(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))) ||
		strings.EqualFold(filepath.Base(filepath.Dir(path)), "sample")
}

// videosIn lists the video files directly in a folder under the library, for
// parse.MovieRoleIn. It's only asked about extras folders and part markers.
func videosIn(lib db.Library) parse.Lister {
	return func(dir string) []string {
		entries, err := os.ReadDir(filepath.Join(lib.Path, dir))
		if err != nil {
			return nil
		}
		var out []string
		for _, e := range entries {
			if !e.IsDir() && parse.IsVideo(e.Name()) {
				out = append(out, filepath.Join(dir, e.Name()))
			}
		}
		return out
	}
}

// parseChanged reports whether today's parser reads an unchanged file
// differently than when it was indexed (e.g. "THE BURBS_t03" is now "THE
// BURBS"), so the file is re-indexed under the right title or episode.
func (w *Worker) parseChanged(lib db.Library, path string, st *db.FileStamp) bool {
	if lib.Kind == "tv" {
		rel, _ := filepath.Rel(lib.Path, path)
		r, ok := parse.Episode(rel)
		return ok && (r.Title != st.ParsedTitle || r.Year != st.ParsedYear || r.Season != st.Season || r.Episode != st.Episode)
	}
	r := parse.MovieIn(relPath(lib, path), videosIn(lib))
	return r.Title != st.ParsedTitle || r.Year != st.ParsedYear
}

func relPath(lib db.Library, path string) string {
	rel, err := filepath.Rel(lib.Path, path)
	if err != nil {
		return path
	}
	return rel
}

// refreshRole brings an unchanged movie file's guessed role (part, extra) up
// to date with the parser, so detection reaches files indexed before it.
func (w *Worker) refreshRole(ctx context.Context, lib db.Library, path string, st *db.FileStamp) error {
	if lib.Kind == "tv" {
		return nil
	}
	if e := parse.Edition(path); e != st.Edition {
		if err := w.db.SetFileEdition(ctx, st.ID, e); err != nil {
			return err
		}
	}
	if st.RolePinned || st.ItemPinned {
		return nil
	}
	r := parse.MovieRoleIn(relPath(lib, path), st.ParsedTitle, videosIn(lib))
	if r.Kind == st.Role && r.Part == st.PartNo && r.Extra == st.ExtraTitle {
		return nil
	}
	if err := w.db.SetDetectedRole(ctx, st.ID, r.Kind, r.Part, r.Extra); err != nil {
		return err
	}
	if r.Kind == "extra" {
		return w.Enqueue(ctx, KindStill, st.ID, "Still "+filepath.Base(path))
	}
	return nil
}

// indexFile parses, probes and records one file. It returns false for files
// that can't be placed (a TV file with no episode number).
func (w *Worker) indexFile(ctx context.Context, lib db.Library, path string, info fs.FileInfo, seen int64) (bool, error) {
	f := db.File{LibraryID: lib.ID, Path: path, Size: info.Size(), Mtime: info.ModTime().Unix()}
	prev, err := w.db.FileStamp(ctx, path)
	if err != nil {
		return false, err
	}
	var (
		itemID  int64
		created bool
		label   string
		role    = parse.Role{Kind: "copy"}
	)
	if lib.Kind == "tv" {
		rel, _ := filepath.Rel(lib.Path, path)
		r, ok := parse.Episode(rel)
		if !ok {
			slog.Info("scan: no episode number, skipping", "path", path)
			return false, nil
		}
		if prev != nil && prev.ItemPinned {
			// Merged under another show by hand: UpsertFile keeps its title and episode.
			itemID, label = prev.MediaItemID, r.Title
		} else if itemID, created, err = w.db.EnsureItem(ctx, lib.ID, "series", r.Title, r.Year); err != nil {
			return false, err
		}
		if prev == nil || !prev.ItemPinned {
			epID, err := w.db.EnsureEpisode(ctx, itemID, r.Season, r.Episode, r.EpisodeTitle, r.AirDate)
			if err != nil {
				return false, err
			}
			f.EpisodeID = &epID
		}
		label = r.Title
	} else {
		rel := relPath(lib, path)
		r := parse.MovieIn(rel, videosIn(lib))
		if prev != nil && prev.ItemPinned {
			itemID = prev.MediaItemID
		} else if itemID, created, err = w.db.EnsureItem(ctx, lib.ID, "movie", r.Title, r.Year); err != nil {
			return false, err
		}
		label = r.Title
		role = parse.MovieRoleIn(rel, r.Title, videosIn(lib))
	}
	f.MediaItemID = itemID

	if p, err := probe.Probe(ctx, w.cfg.FFprobe, path); err != nil {
		if errors.Is(err, context.Canceled) {
			return false, err
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return false, errProbeTimeout
		}
		slog.Warn("scan: file is unreadable (corrupt or incomplete)", "path", path, "err", err)
		f.Container = strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
		f.Problem = "unreadable"
	} else {
		f.DurationSec, f.Container, f.VideoCodec, f.AudioCodec = p.DurationSec, p.Container, p.VideoCodec, p.AudioCodec
		f.Width, f.Height, f.AudioTracks, f.SubtitleTracks = p.Width, p.Height, p.AudioTracks, p.SubtitleTracks
		f.DynamicRange, f.DVProfile = p.DynamicRange(), p.DVProfile
		if p.VideoCodec == "" {
			// Usually DRM-protected iTunes purchases: the video stream has no codec ffmpeg can decode.
			slog.Warn("scan: file has no decodable video (DRM-protected?)", "path", path)
			f.Problem = "no-video"
		}
	}

	fileID, err := w.db.UpsertFile(ctx, f, seen)
	if err != nil {
		return false, err
	}
	if lib.Kind != "tv" {
		if err := w.db.SetDetectedRole(ctx, fileID, role.Kind, role.Part, role.Extra); err != nil {
			return false, err
		}
		if err := w.db.SetFileEdition(ctx, fileID, parse.Edition(path)); err != nil {
			return false, err
		}
	}
	// A changed file makes its optimized copy stale. One re-indexed only
	// because its name parses differently, or re-scanned by hand, keeps it.
	if prev == nil || prev.Size != f.Size || prev.Mtime != f.Mtime {
		if old, err := w.db.DeleteOptimized(ctx, fileID); err == nil && old != "" {
			_ = os.Remove(old)
		}
	}
	if created {
		if err := w.Enqueue(ctx, KindMatch, itemID, "Match "+label); err != nil {
			return false, err
		}
	}
	// Episodes and extras are shown with a frame from the file.
	if (f.EpisodeID != nil || role.Kind == "extra") && f.Problem == "" {
		if err := w.Enqueue(ctx, KindStill, fileID, "Still "+filepath.Base(path)); err != nil {
			return false, err
		}
	}
	if w.comskip != "" && f.Problem == "" {
		if err := w.autoCommercials(ctx, fileID, path); err != nil {
			return false, err
		}
	}
	// A new or changed file in a library with preview thumbnails on.
	if lib.Trickplay && f.Problem == "" {
		if err := w.Enqueue(ctx, KindTrickplay, fileID, "Preview thumbnails "+filepath.Base(path)); err != nil {
			return false, err
		}
	}
	return true, nil
}

// RescanFile reads one file again the way a scan reads a new one (probe,
// name, role, still), even though it hasn't changed. Errors meant for the
// person who asked are usererr.
func (w *Worker) RescanFile(ctx context.Context, fileID int64) error {
	f, err := w.db.File(ctx, fileID)
	if err != nil {
		return err
	}
	lib, err := w.db.Library(ctx, f.LibraryID)
	if err != nil {
		return err
	}
	info, err := os.Stat(f.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return usererr.New("the file isn't there any more; scan the library to remove it")
	}
	if err != nil {
		return usererr.New("couldn't read the file: " + err.Error())
	}
	ok, err := w.indexFile(ctx, lib, f.Path, info, time.Now().Unix())
	if errors.Is(err, errProbeTimeout) {
		return usererr.New("reading the file timed out; try again")
	}
	if err != nil {
		return err
	}
	if !ok {
		return usererr.New("the name no longer has a season and episode number, so it isn't an episode; the next library scan removes it")
	}
	w.Wake()
	return nil
}

// autoCommercials queues commercial detection for a finished DVR recording
// that hasn't been analysed in its current form.
func (w *Worker) autoCommercials(ctx context.Context, fileID int64, path string) error {
	if rec, err := w.db.IsRecording(ctx, path); err != nil || !rec {
		return err
	}
	if _, done, err := w.db.Commercials(ctx, fileID); err != nil || done {
		return err
	}
	return w.EnqueueCommercials(ctx, fileID, path)
}
