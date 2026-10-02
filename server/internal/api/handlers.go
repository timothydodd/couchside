package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/metadata"
	"github.com/timothydodd/couchside/internal/worker"
)

// --- status & home -----------------------------------------------------------

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	counts, err := s.db.Counts(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	jobs, err := s.db.JobCounts(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	tv := map[string]any{"configured": false}
	if s.tv != nil {
		tv = s.tv.Summary()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"livetv":    tv,
		"version":   s.version,
		"providers": s.providers.Names(),
		"tmdbKey":   s.cfg.TMDBKeySource(),
		"mediaRoot": s.cfg.MediaRoot,
		"counts":    counts,
		"jobs":      jobs,
		"scanEvery": s.cfg.ScanInterval.String(),
		"workers":   s.cfg.Workers,
		"comskip":   s.worker.CommercialsAvailable(),
		"transcode": map[string]any{
			"hwaccel":        s.tc.Encoder().HW,
			"requested":      s.cfg.HWAccel,
			"tonemap":        s.tc.Encoder().Tonemap,
			"maxSessions":    s.tc.Max(),
			"active":         len(s.tc.Sessions()),
			"encodeWorkers":  s.cfg.EncodeWorkers,
			"optimizeHeight": s.cfg.OptimizeHeight,
		},
	})
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cont, err := s.db.ContinueWatching(ctx, 12)
	if err != nil {
		writeErr(w, err)
		return
	}
	movies, err := s.db.RecentItems(ctx, "movie", 24)
	if err != nil {
		writeErr(w, err)
		return
	}
	series, err := s.db.RecentItems(ctx, "series", 24)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"continueWatching": cont, "recentMovies": movies, "recentSeries": series})
}

// --- libraries ---------------------------------------------------------------

func (s *Server) listLibraries(w http.ResponseWriter, r *http.Request) {
	libs, err := s.db.Libraries(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, libs)
}

func (s *Server) createLibrary(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name, Path, Kind string }
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeErr(w, badRequest("name is required"))
		return
	}
	if in.Kind != "movies" && in.Kind != "tv" {
		writeErr(w, badRequest("kind must be movies or tv"))
		return
	}
	path, err := s.checkPath(in.Path)
	if err != nil {
		writeErr(w, err)
		return
	}
	id, err := s.db.CreateLibrary(r.Context(), in.Name, path, in.Kind)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeErr(w, badRequest("a library already uses that folder"))
			return
		}
		writeErr(w, err)
		return
	}
	if err := s.worker.Enqueue(r.Context(), worker.KindScan, id, "Scan "+in.Name); err != nil {
		writeErr(w, err)
		return
	}
	lib, err := s.db.Library(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, lib)
}

// updateLibrary renames a library or moves it to another folder. The kind
// can't change: every item would have to be rebuilt, so that's remove and re-add.
func (s *Server) updateLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in struct{ Name, Path string }
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	ctx := r.Context()
	old, err := s.db.Library(ctx, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeErr(w, badRequest("name is required"))
		return
	}
	path := old.Path
	if strings.TrimSpace(in.Path) != "" && filepath.Clean(strings.TrimSpace(in.Path)) != old.Path {
		if path, err = s.checkPath(in.Path); err != nil {
			writeErr(w, err)
			return
		}
	}
	if err := s.db.UpdateLibrary(ctx, id, in.Name, path); err != nil {
		if errors.Is(err, db.ErrPathInUse) {
			err = badRequest(err.Error())
		}
		writeErr(w, err)
		return
	}
	if path != old.Path {
		s.repointRecordings(ctx, old.Path, path)
		if err := s.worker.Enqueue(ctx, worker.KindScan, id, "Scan "+in.Name); err != nil {
			writeErr(w, err)
			return
		}
	}
	lib, err := s.db.Library(ctx, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lib)
}

// repointRecordings follows DVR recordings to a library's new folder when
// the file is there, so they stay linked to their library entry.
func (s *Server) repointRecordings(ctx context.Context, oldDir, newDir string) {
	recs, err := s.db.RecordingsWithStatus(ctx, "completed")
	if err != nil {
		return
	}
	prefix := strings.TrimSuffix(oldDir, "/") + "/"
	for _, rec := range recs {
		if !strings.HasPrefix(rec.Path, prefix) {
			continue
		}
		moved := filepath.Join(newDir, strings.TrimPrefix(rec.Path, prefix))
		if _, err := os.Stat(moved); err == nil {
			_ = s.db.SetRecordingPath(ctx, rec.ID, moved)
		}
	}
}

// checkPath cleans a library path and confirms it's a readable folder inside
// the media root (when one is configured).
func (s *Server) checkPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" || !filepath.IsAbs(p) {
		return "", badRequest("path must be an absolute folder path")
	}
	p = filepath.Clean(p)
	if !s.underRoot(p) {
		return "", badRequest("path must be inside " + s.cfg.MediaRoot)
	}
	st, err := os.Stat(p)
	if err != nil || !st.IsDir() {
		return "", badRequest("folder not found: " + p)
	}
	return p, nil
}

func (s *Server) underRoot(p string) bool {
	root := s.cfg.MediaRoot
	if root == "" {
		return true
	}
	within := func(r string) bool { return p == r || strings.HasPrefix(p, strings.TrimSuffix(r, "/")+"/") }
	// The DVR's own folder is allowed too: it lives outside the read-only media mount.
	return within(root) || (s.tv != nil && within(s.tv.DefaultRecordingsDir()))
}

func (s *Server) deleteLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.db.DeleteLibrary(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) scanLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	lib, err := s.db.Library(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.worker.Enqueue(r.Context(), worker.KindScan, id, "Scan "+lib.Name); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) scanAll(w http.ResponseWriter, r *http.Request) {
	s.worker.ScanAll(r.Context())
	w.WriteHeader(http.StatusAccepted)
}

// browse lists subfolders so the Libraries page can offer a folder picker.
// Only available when a media root is configured, and never escapes it.
func (s *Server) browse(w http.ResponseWriter, r *http.Request) {
	if s.cfg.MediaRoot == "" {
		writeErr(w, httpError{http.StatusNotFound, "folder browsing needs COUCHSIDE_MEDIA_ROOT"})
		return
	}
	p := r.URL.Query().Get("path")
	if p == "" {
		p = s.cfg.MediaRoot
	}
	p = filepath.Clean(p)
	if !filepath.IsAbs(p) || !s.underRoot(p) {
		writeErr(w, badRequest("path must be inside "+s.cfg.MediaRoot))
		return
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		writeErr(w, badRequest("cannot read "+p))
		return
	}
	dirs := []string{}
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			dirs = append(dirs, e.Name())
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return strings.ToLower(dirs[i]) < strings.ToLower(dirs[j]) })
	var parent *string
	if p != s.cfg.MediaRoot {
		pp := filepath.Dir(p)
		parent = &pp
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": p, "parent": parent, "dirs": dirs})
}

// --- items -------------------------------------------------------------------

func (s *Server) listItems(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	if kind != "movie" && kind != "series" {
		writeErr(w, badRequest("kind must be movie or series"))
		return
	}
	items, err := s.db.Items(r.Context(), kind)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

type season struct {
	Season   int             `json:"season"`
	Episodes []db.EpisodeRow `json:"episodes"`
}

func (s *Server) getItem(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	ctx := r.Context()
	item, err := s.db.Item(ctx, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	files, err := s.db.ItemFiles(ctx, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	for i := range files {
		files[i].Path = filepath.Base(files[i].Path) // don't leak server paths to the UI
	}
	out := map[string]any{"item": item, "files": files}
	if item.Kind == "series" {
		eps, err := s.db.SeriesEpisodes(ctx, id)
		if err != nil {
			writeErr(w, err)
			return
		}
		seasons := []season{}
		for _, e := range eps {
			if len(seasons) == 0 || seasons[len(seasons)-1].Season != e.Season {
				seasons = append(seasons, season{Season: e.Season})
			}
			seasons[len(seasons)-1].Episodes = append(seasons[len(seasons)-1].Episodes, e)
		}
		out["seasons"] = seasons
		out["files"] = []db.File{}
	}
	writeJSON(w, http.StatusOK, out)
}

var reImdb = regexp.MustCompile(`tt\d{5,10}`)

// rematch re-runs metadata matching, optionally pinned to an IMDb id or URL.
func (s *Server) rematch(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in struct{ ImdbID string }
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	imdb := ""
	if strings.TrimSpace(in.ImdbID) != "" {
		// An IMDb id or URL, or a TMDB reference or URL for titles IMDb doesn't have.
		if imdb = reImdb.FindString(in.ImdbID); imdb == "" {
			imdb = metadata.TMDBID(in.ImdbID)
		}
		if imdb == "" {
			writeErr(w, badRequest("that doesn't look like an IMDb id (tt1234567), an IMDb URL or a TMDB URL"))
			return
		}
	}
	item, err := s.db.Item(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.db.SetImdbOverride(r.Context(), id, imdb); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.worker.Enqueue(r.Context(), worker.KindMatch, id, "Match "+item.ParsedTitle); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) itemWatched(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in struct{ Watched bool }
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	files, err := s.db.FeatureFiles(r.Context(), id) // extras keep their own state
	if err != nil {
		writeErr(w, err)
		return
	}
	ids := make([]int64, len(files))
	for i, f := range files {
		ids[i] = f.ID
	}
	if err := s.db.SetWatched(r.Context(), ids, in.Watched); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- files & playback --------------------------------------------------------

func (s *Server) playInfo(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	p, err := s.db.PlayInfo(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func (s *Server) saveProgress(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in struct {
		Position, Duration float64
		State              string // playing | paused | stopped (older clients send none)
		Mode               string // how it's playing, shown on the Settings page
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if in.Position < 0 || in.Duration < 0 {
		writeErr(w, badRequest("position and duration must be positive"))
		return
	}
	if in.State == "stopped" {
		s.presence.setPlaying(r, nil)
	} else {
		s.presence.setPlaying(r, &playing{Kind: "file", FileID: id, Position: in.Position, Duration: in.Duration,
			Mode: truncate(in.Mode, 60), Paused: in.State == "paused"})
	}
	if err := s.db.SaveProgress(r.Context(), id, in.Position, in.Duration); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) fileWatched(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in struct{ Watched bool }
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.db.SetWatched(r.Context(), []int64{id}, in.Watched); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var mimeTypes = map[string]string{
	".mp4": "video/mp4", ".m4v": "video/mp4", ".mov": "video/quicktime", ".webm": "video/webm",
	".mkv": "video/x-matroska", ".avi": "video/x-msvideo", ".ts": "video/mp2t", ".m2ts": "video/mp2t",
}

// stream serves the original file with byte-range support (direct play).
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	f, err := s.db.File(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if r.URL.Query().Get("version") == "optimized" {
		p, err := s.db.OptimizedPath(r.Context(), id)
		if err != nil || p == "" {
			writeErr(w, httpError{http.StatusNotFound, "no optimized version"})
			return
		}
		f.Path = p
	}
	fh, err := os.Open(f.Path)
	if err != nil {
		writeErr(w, httpError{http.StatusNotFound, "file is missing on disk; rescan the library"})
		return
	}
	defer fh.Close()
	st, err := fh.Stat()
	if err != nil {
		writeErr(w, err)
		return
	}
	if ct, ok := mimeTypes[strings.ToLower(filepath.Ext(f.Path))]; ok {
		w.Header().Set("Content-Type", ct)
	}
	http.ServeContent(w, r, filepath.Base(f.Path), st.ModTime(), fh)
}

// --- artwork -----------------------------------------------------------------

var artKinds = map[string]string{"poster": "poster.webp", "poster-thumb": "poster-thumb.webp", "backdrop": "backdrop.webp"}

func (s *Server) itemArtwork(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	name, ok := artKinds[chi.URLParam(r, "kind")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	serveImage(w, r, filepath.Join(worker.ItemArtDir(s.cfg.CacheDir, id), name))
}

func (s *Server) fileStill(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	serveImage(w, r, worker.FileStillPath(s.cfg.CacheDir, id))
}

// serveImage serves cached artwork. URLs carry a ?v= version from the item's
// updated_at, so the browser can cache them for a long time.
func serveImage(w http.ResponseWriter, r *http.Request, path string) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/webp")
	w.Header().Set("Cache-Control", "public, max-age=604800")
	http.ServeFile(w, r, path)
}

// --- jobs --------------------------------------------------------------------

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	jobs, err := s.db.RecentJobs(r.Context(), limit)
	if err != nil {
		writeErr(w, err)
		return
	}
	counts, err := s.db.JobCounts(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs, "counts": counts})
}

func (s *Server) retryJob(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.db.RetryJob(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	s.worker.Wake()
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) clearJobs(w http.ResponseWriter, r *http.Request) {
	if err := s.db.ClearFinishedJobs(r.Context()); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
