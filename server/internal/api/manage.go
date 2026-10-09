package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/fsx"
	"github.com/timothydodd/couchside/internal/imaging"
	"github.com/timothydodd/couchside/internal/metadata"
	"github.com/timothydodd/couchside/internal/worker"
)

// --- library Manage view ---------------------------------------------------------

func (s *Server) manageItems(w http.ResponseWriter, r *http.Request) {
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
	rows, err := s.db.ManageRows(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"library": lib, "items": rows})
}

// manageItem is one title's Manage row and its library, for the Edit panel on its page.
func (s *Server) manageItem(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	row, err := s.db.ManageRow(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	item, err := s.db.Item(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	lib, err := s.db.Library(r.Context(), item.LibraryID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": row, "library": lib})
}

func (s *Server) itemFiles(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	files, err := s.db.ManageFiles(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, files)
}

// itemLookup searches the metadata provider by any title, so a wrong or
// missing match can be fixed by picking the right one.
func (s *Server) itemLookup(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	item, err := s.db.Item(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		q = item.ParsedTitle
	}
	year, _ := strconv.Atoi(r.URL.Query().Get("year"))
	kind := metadata.Movie
	if item.Kind == "series" {
		kind = metadata.Series
	}
	res, err := s.providers.SearchTitles(r.Context(), kind, q, year)
	if err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	for i := range res {
		res[i].Poster = db.CachedImage(res[i].Poster) // through the server's image cache
	}
	writeJSON(w, http.StatusOK, res)
}

// --- deleting from disk ----------------------------------------------------------

// deleteFile removes one file from disk (with its subtitle sidecars) and
// from the library. Used for extra copies found as duplicates.
func (s *Server) deleteFile(w http.ResponseWriter, r *http.Request) {
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
	lib, err := s.db.Library(r.Context(), f.LibraryID)
	if err != nil {
		writeErr(w, err)
		return
	}
	res, err := s.deleteFiles(r.Context(), lib, []db.File{f})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// rescanFile reads one file again (length, codecs, name and still).
func (s *Server) rescanFile(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := s.db.File(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.worker.RescanFile(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteItem removes a whole movie or series: every file on disk, then the item.
func (s *Server) deleteItem(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	item, err := s.db.Item(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	lib, err := s.db.Library(r.Context(), item.LibraryID)
	if err != nil {
		writeErr(w, err)
		return
	}
	files, err := s.db.ItemFiles(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	res, err := s.deleteFiles(r.Context(), lib, files)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type deleteResult struct {
	Deleted      int      `json:"deleted"`
	Bytes        int64    `json:"bytes"`
	ItemsRemoved []int64  `json:"itemsRemoved"`
	KeptFolders  []string `json:"keptFolders"` // emptied of video but still holding other files
}

// deleteFiles deletes files on disk, then forgets them. It stops at the first
// file it can't delete (read-only mount, permissions) and reports it, keeping
// what was already done.
func (s *Server) deleteFiles(ctx context.Context, lib db.Library, files []db.File) (deleteResult, error) {
	res := deleteResult{ItemsRemoved: []int64{}, KeptFolders: []string{}}
	var done []int64
	var failure error
	dirs := map[string]string{} // folder → its library's root
	// A title's files can be in several libraries: each is checked against its own.
	libs := map[int64]db.Library{lib.ID: lib}
	for _, f := range files {
		l, ok := libs[f.LibraryID]
		if !ok {
			var err error
			if l, err = s.db.Library(ctx, f.LibraryID); err != nil {
				return res, err
			}
			libs[f.LibraryID] = l
		}
		if !insideDir(f.Path, l.Path) {
			failure = fmt.Errorf("%s is outside the library folder; not deleting it", f.Path)
			break
		}
		if err := os.Remove(f.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			failure = fmt.Errorf("couldn't delete %s: %v", filepath.Base(f.Path), err)
			break
		}
		for _, sc := range sidecars(f.Path) {
			_ = os.Remove(sc)
		}
		if old, _ := s.db.DeleteOptimized(ctx, f.ID); old != "" {
			_ = os.Remove(old)
		}
		_ = os.RemoveAll(filepath.Dir(worker.FileStillPath(s.cfg.CacheDir, f.ID)))
		_ = s.db.DeleteRecordingsAt(ctx, f.Path)
		slog.Info("deleted file", "path", f.Path, "size", f.Size)
		done = append(done, f.ID)
		res.Deleted++
		res.Bytes += f.Size
		dirs[filepath.Dir(f.Path)] = l.Path
	}
	if len(done) > 0 {
		gone, err := s.db.DeleteFileRows(ctx, done)
		if err != nil {
			return res, err
		}
		for _, id := range gone {
			_ = os.RemoveAll(worker.ItemArtDir(s.cfg.CacheDir, id))
		}
		res.ItemsRemoved = append(res.ItemsRemoved, gone...)
	}
	for d, root := range dirs {
		if kept := removeEmptyDirs(d, root); kept != "" {
			res.KeptFolders = append(res.KeptFolders, kept)
		}
	}
	if failure != nil {
		return res, badRequest(failure.Error())
	}
	return res, nil
}

// insideDir reports whether p is strictly inside dir.
func insideDir(p, dir string) bool { return fsx.Inside(p, dir) }

// removeEmptyDirs removes dir and its parents up to (not including) root
// while they're empty. It returns the first folder left because it still
// holds other files (artwork, .nfo), or "" when none of it held any.
func removeEmptyDirs(dir, root string) string {
	for d := filepath.Clean(dir); insideDir(d, root); d = filepath.Dir(d) {
		entries, err := os.ReadDir(d)
		if err != nil {
			return ""
		}
		if len(entries) > 0 {
			if hasVideoLeft(entries) {
				return "" // other episodes live here; nothing unusual
			}
			return d
		}
		if err := os.Remove(d); err != nil {
			return d
		}
	}
	return ""
}

func hasVideoLeft(entries []os.DirEntry) bool {
	for _, e := range entries {
		if e.IsDir() {
			return true
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".mkv", ".mp4", ".m4v", ".avi", ".ts", ".m2ts", ".mov", ".wmv", ".mpg", ".mpeg", ".webm", ".flv":
			return true
		}
	}
	return false
}

// --- custom artwork --------------------------------------------------------------

// uploadArtwork replaces an item's poster or backdrop with an uploaded image
// (the request body). The artwork job and re-matches leave it in place.
func (s *Server) uploadArtwork(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	kind := chi.URLParam(r, "kind")
	if kind != "poster" && kind != "backdrop" {
		writeErr(w, badRequest("artwork must be poster or backdrop"))
		return
	}
	if _, err := s.db.Item(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	dir := worker.ItemArtDir(s.cfg.CacheDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeErr(w, err)
		return
	}
	// Its own temp file: a poster and a backdrop can be uploaded together.
	out, err := os.CreateTemp(dir, "upload-*.orig")
	if err != nil {
		writeErr(w, err)
		return
	}
	orig := out.Name()
	defer os.Remove(orig)
	n, err := io.Copy(out, http.MaxBytesReader(w, r.Body, 25<<20))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		writeErr(w, badRequest("upload failed (images up to 25 MB): "+err.Error()))
		return
	}
	if n == 0 {
		writeErr(w, badRequest("the upload was empty"))
		return
	}
	ff := imaging.FFmpeg{Bin: s.cfg.FFmpeg}
	if kind == "poster" {
		err = ff.Resize(r.Context(), orig, filepath.Join(dir, "poster.webp"), 780)
		if err == nil {
			err = ff.Resize(r.Context(), orig, filepath.Join(dir, "poster-thumb.webp"), 360)
		}
	} else {
		err = ff.Resize(r.Context(), orig, filepath.Join(dir, "backdrop.webp"), 1280)
	}
	if err != nil {
		slog.Warn("artwork upload", "item", id, "err", err)
		writeErr(w, badRequest("that file isn't an image Couchside can read"))
		return
	}
	if err := s.db.SetCustomArtwork(r.Context(), id, kind, true); err != nil {
		writeErr(w, err)
		return
	}
	// The upload replaced whatever the provider link gave; forget that link
	// (only now: a failed upload leaves the automatic artwork as it was).
	_ = os.Remove(filepath.Join(dir, kind+".src"))
	w.WriteHeader(http.StatusNoContent)
}

// resetArtwork drops uploaded artwork and fetches the automatic one again.
func (s *Server) resetArtwork(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	kind := chi.URLParam(r, "kind")
	if kind != "poster" && kind != "backdrop" {
		writeErr(w, badRequest("artwork must be poster or backdrop"))
		return
	}
	item, err := s.db.Item(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.db.SetCustomArtwork(r.Context(), id, kind, false); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.worker.Enqueue(r.Context(), worker.KindArtwork, id, "Artwork "+item.Title); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// setFileRole marks a movie file as another copy, part N of the movie, or an
// extra with a title. The choice is pinned, so scans keep it. Marking part 2+
// while the movie has no part 1 makes its one other copy part 1: "this is the
// second part" is usually all anyone means.
func (s *Server) setFileRole(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in struct {
		Role       string
		PartNo     int
		ExtraTitle string
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	ctx := r.Context()
	f, err := s.db.File(ctx, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if f.EpisodeID != nil {
		writeErr(w, badRequest("only movie files can be parts or extras"))
		return
	}
	in.ExtraTitle = strings.TrimSpace(in.ExtraTitle)
	switch in.Role {
	case "copy":
		in.PartNo, in.ExtraTitle = 0, ""
	case "part":
		if in.PartNo < 1 || in.PartNo > 20 {
			writeErr(w, badRequest("part number must be between 1 and 20"))
			return
		}
		in.ExtraTitle = ""
	case "extra":
		in.PartNo = 0
		if in.ExtraTitle == "" {
			in.ExtraTitle = "Bonus"
		}
		if len(in.ExtraTitle) > 200 {
			writeErr(w, badRequest("extra title is too long"))
			return
		}
	default:
		writeErr(w, badRequest("role must be copy, part or extra"))
		return
	}
	if err := s.db.SetFileRole(ctx, id, in.Role, in.PartNo, in.ExtraTitle); err != nil {
		writeErr(w, err)
		return
	}
	if in.Role == "part" && in.PartNo > 1 {
		files, err := s.db.ItemFiles(ctx, f.MediaItemID)
		if err != nil {
			writeErr(w, err)
			return
		}
		var copies []db.File
		hasFirst := false
		for _, o := range files {
			if o.ID == id {
				continue
			}
			hasFirst = hasFirst || (o.Role == "part" && o.PartNo == 1)
			if o.Role == "copy" {
				copies = append(copies, o)
			}
		}
		if !hasFirst && len(copies) == 1 {
			if err := s.db.SetFileRole(ctx, copies[0].ID, "part", 1, ""); err != nil {
				writeErr(w, err)
				return
			}
		}
	}
	if in.Role == "extra" && !f.HasStill && f.Problem == "" {
		if err := s.worker.Enqueue(ctx, worker.KindStill, id, "Still "+filepath.Base(f.Path)); err != nil {
			writeErr(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
