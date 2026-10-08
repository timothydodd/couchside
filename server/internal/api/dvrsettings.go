package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/timothydodd/couchside/internal/livetv"
)

type folderOption struct {
	Path     string `json:"path"`
	Label    string `json:"label"`
	Detail   string `json:"detail"`
	Writable bool   `json:"writable"`
	Problem  string `json:"problem"`
}

// dvrSettings lists where recordings can go: Couchside's own storage and each
// TV library folder, with whether each is writable right now.
func (s *Server) dvrSettings(w http.ResponseWriter, r *http.Request) {
	if !s.tv.HasTuner() {
		writeErr(w, errNoTuner)
		return
	}
	ctx := r.Context()
	current := s.tv.RecordingsDir(ctx)
	opts := []folderOption{{Path: s.tv.DefaultRecordingsDir(), Label: "Couchside storage",
		Detail: "Couchside's own volume. Only Couchside sees these recordings."}}
	libs, err := s.db.Libraries(ctx)
	if err != nil {
		writeErr(w, err)
		return
	}
	for _, l := range libs {
		if l.Kind != "tv" || l.Path == s.tv.DefaultRecordingsDir() {
			continue
		}
		opts = append(opts, folderOption{Path: l.Path, Label: l.Name + " library",
			Detail: "Recordings land next to your shows, in existing show folders when they match."})
	}
	found := false
	for i := range opts {
		if err := livetv.CheckWritableNoCreate(opts[i].Path); err != nil {
			opts[i].Problem = err.Error()
		} else {
			opts[i].Writable = true
		}
		found = found || opts[i].Path == current
	}
	if !found {
		o := folderOption{Path: current, Label: "Custom folder", Detail: current}
		if err := livetv.CheckWritableNoCreate(current); err != nil {
			o.Problem = err.Error()
		} else {
			o.Writable = true
		}
		opts = append(opts, o)
	}
	recs, _ := s.db.RecordingsWithStatus(ctx, "completed")
	movable := 0
	for _, rec := range recs {
		if rec.Path != "" && strings.HasPrefix(rec.Path, current+"/") {
			movable++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"recordingsDir": current, "default": s.tv.DefaultRecordingsDir(),
		"options": opts, "movable": movable, "mediaRoot": first(s.mediaRoots(r.Context()))})
}

func (s *Server) dvrSaveSettings(w http.ResponseWriter, r *http.Request) {
	if !s.tv.HasTuner() {
		writeErr(w, errNoTuner)
		return
	}
	var in struct {
		RecordingsDir string `json:"recordingsDir"`
		MoveExisting  bool   `json:"moveExisting"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	dir := filepath.Clean(strings.TrimSpace(in.RecordingsDir))
	if !filepath.IsAbs(dir) {
		writeErr(w, badRequest("choose a folder"))
		return
	}
	if dir != s.tv.DefaultRecordingsDir() && !s.underRoot(r.Context(), dir) {
		writeErr(w, errOutsideLocations(dir))
		return
	}
	if st, err := os.Stat(filepath.Dir(dir)); err != nil || !st.IsDir() {
		writeErr(w, badRequest("the parent folder doesn't exist: "+filepath.Dir(dir)))
		return
	}
	res, err := s.tv.SetRecordingsDir(r.Context(), dir, in.MoveExisting)
	if err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, res)
}
