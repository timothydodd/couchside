package api

import (
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/livetv"
)

// Virtual channels: Couchside's own channels built from the library
// (livetv/virtual.go). Admin only.

var reChannelNumber = regexp.MustCompile(`^\d{1,4}(\.\d{1,3})?$`)

type virtualOut struct {
	db.VirtualChannel
	Error string `json:"error,omitempty"` // why it has no schedule
}

func (s *Server) listVirtual(w http.ResponseWriter, r *http.Request) {
	chans, err := s.db.VirtualChannels(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	out := make([]virtualOut, len(chans))
	for i, c := range chans {
		out[i] = virtualOut{VirtualChannel: c, Error: s.tv.VirtualError(c.ID)}
	}
	writeJSON(w, http.StatusOK, out)
}

type virtualIn struct {
	Number string               `json:"number"`
	Name   string               `json:"name"`
	Config livetv.VirtualConfig `json:"config"`
}

// checkVirtual validates a channel from the wizard.
func (s *Server) checkVirtual(in *virtualIn) error {
	in.Number, in.Name = strings.TrimSpace(in.Number), strings.TrimSpace(in.Name)
	if !reChannelNumber.MatchString(in.Number) {
		return badRequest("the channel number is digits, like 900 or 900.1")
	}
	if in.Name == "" || len(in.Name) > 60 {
		return badRequest("give the channel a name (up to 60 characters)")
	}
	if f := strings.TrimSpace(in.Config.Filler.Folder); f != "" {
		p, err := s.checkPath(f)
		if err != nil {
			return err
		}
		in.Config.Filler.Folder = p
	}
	if err := in.Config.Validate(); err != nil {
		return badRequest(err.Error())
	}
	return nil
}

func (s *Server) createVirtual(w http.ResponseWriter, r *http.Request) { s.saveVirtual(w, r, 0) }

func (s *Server) updateVirtual(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.saveVirtual(w, r, id)
}

func (s *Server) saveVirtual(w http.ResponseWriter, r *http.Request, id int64) {
	var in virtualIn
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.checkVirtual(&in); err != nil {
		writeErr(w, err)
		return
	}
	id, err := s.tv.SaveVirtual(r.Context(), id, in.Number, in.Name, in.Config)
	switch {
	case errors.Is(err, db.ErrNumberTaken):
		writeErr(w, httpError{http.StatusConflict, "channel " + in.Number + " already exists; pick another number"})
		return
	case err != nil:
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "error": s.tv.VirtualError(id)})
}

func (s *Server) deleteVirtual(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.tv.DeleteVirtual(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// previewVirtual lays out the next hours of a config, for the wizard.
func (s *Server) previewVirtual(w http.ResponseWriter, r *http.Request) {
	var in virtualIn
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	in.Number, in.Name = "900", "Preview" // only the config matters here
	if err := s.checkVirtual(&in); err != nil {
		writeErr(w, err)
		return
	}
	hours, _ := strconv.Atoi(r.URL.Query().Get("hours"))
	progs, matches, err := s.tv.PreviewVirtual(r.Context(), in.Config, min(max(hours, 3), 48))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"programs": progs, "matches": matches})
}

// virtualOptions is what the wizard offers: genres, years and titles in the
// library, and a free channel number.
func (s *Server) virtualOptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	files, err := s.db.VirtualFiles(ctx, nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	type title struct {
		ID        int64  `json:"id"`
		Title     string `json:"title"`
		Year      int    `json:"year"`
		Kind      string `json:"kind"`
		LibraryID int64  `json:"libraryId"`
		sort      string
	}
	genres := map[string]bool{}
	titles := map[int64]title{}
	minYear, maxYear := 0, 0
	for _, f := range files {
		for _, g := range f.Genres {
			genres[g] = true
		}
		if f.Year > 0 {
			if minYear == 0 || f.Year < minYear {
				minYear = f.Year
			}
			maxYear = max(maxYear, f.Year)
		}
		titles[f.ItemID] = title{ID: f.ItemID, Title: f.Title, Year: f.Year, Kind: f.Kind, LibraryID: f.LibraryID, sort: f.SortTitle}
	}
	gs := make([]string, 0, len(genres))
	for g := range genres {
		gs = append(gs, g)
	}
	sort.Strings(gs)
	ts := make([]title, 0, len(titles))
	for _, t := range titles {
		ts = append(ts, t)
	}
	sort.Slice(ts, func(i, j int) bool { return ts[i].sort < ts[j].sort })

	// Suggest the first free number from 900 up.
	taken := map[string]bool{}
	if chans, err := s.db.Channels(ctx); err == nil {
		for _, c := range chans {
			taken[c.Number] = true
		}
	}
	next := 900
	for taken[strconv.Itoa(next)] {
		next++
	}
	writeJSON(w, http.StatusOK, map[string]any{"genres": gs, "titles": ts, "minYear": minYear, "maxYear": maxYear,
		"nextNumber": strconv.Itoa(next), "mediaRoot": s.cfg.MediaRoot})
}
