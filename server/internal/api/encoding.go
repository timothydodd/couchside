package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/transcode"
	"github.com/timothydodd/couchside/internal/worker"
)

// --- live HLS transcoding ------------------------------------------------------

// createHLS starts a transcode session for a file. The client says what it
// can decode natively; the server decides what to copy and what to encode.
func (s *Server) createHLS(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in struct {
		Height       int  `json:"height"`
		BitrateK     int  `json:"bitrateK"` // a quality preset's bitrate; 0 = the default for the height
		CopyVideo    bool `json:"copyVideo"`
		CopyAudio    bool `json:"copyAudio"`
		AudioIndex   int  `json:"audioIndex"`
		BurnSubtitle *int `json:"burnSubtitle"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	f, err := s.db.File(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if msg := ProblemMessage(f.Problem); msg != "" {
		writeErr(w, httpError{http.StatusUnprocessableEntity, msg})
		return
	}
	info, err := s.db.PlayInfo(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	dur := 0.0
	if f.DurationSec != nil {
		dur = *f.DurationSec
	}
	title := info.Title
	if info.Subtitle != "" {
		title += " · " + info.Subtitle
	}
	burn := -1
	if in.BurnSubtitle != nil && *in.BurnSubtitle >= 0 {
		burn = *in.BurnSubtitle
	}
	sess, err := s.tc.Create(r.Context(), transcode.Request{
		FileID: id, Title: title, Path: f.Path, Duration: dur,
		Height: in.Height, BitrateK: in.BitrateK, AllowCopyVideo: in.CopyVideo, AllowCopyAudio: in.CopyAudio,
		AudioIndex: max(0, in.AudioIndex), BurnSubtitle: burn,
	})
	if err != nil {
		if errors.Is(err, transcode.ErrBusy) {
			writeErr(w, httpError{http.StatusTooManyRequests, err.Error()})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"sessionId": sess.ID,
		"playlist":  fmt.Sprintf("/api/hls/%s/index.m3u8", sess.ID),
		"mode":      sess.Mode,
		"height":    sess.Height,
		"copyVideo": sess.CopyVideo,
		"copyAudio": sess.CopyAudio,
		"hdr":       sess.HDR,
		"hw":        sess.HW,
		"hwDecode":  sess.HWDecode,
		"audio":     sess.Audio,
		"burnSub":   sess.BurnSub,
		"bitrateK":  sess.BitrateK,
	})
}

func (s *Server) hlsPlaylist(w http.ResponseWriter, r *http.Request) {
	sess := s.tc.Get(chi.URLParam(r, "sid"))
	if sess == nil {
		writeErr(w, httpError{http.StatusNotFound, transcode.ErrNoSession.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	_, _ = w.Write(sess.Playlist())
}

var reSeg = regexp.MustCompile(`^seg(\d+)\.ts$`)

func (s *Server) hlsSegment(w http.ResponseWriter, r *http.Request) {
	m := reSeg.FindStringSubmatch(chi.URLParam(r, "seg"))
	if m == nil {
		http.NotFound(w, r)
		return
	}
	n, _ := strconv.Atoi(m[1])
	path, err := s.tc.Segment(r.Context(), chi.URLParam(r, "sid"), n)
	switch {
	case errors.Is(err, transcode.ErrNoSession), errors.Is(err, transcode.ErrBadSegment):
		writeErr(w, httpError{http.StatusNotFound, err.Error()})
		return
	case errors.Is(err, transcode.ErrPastEnd):
		// 416 tells the player the stream simply ended early (see ErrPastEnd).
		writeErr(w, httpError{http.StatusRequestedRangeNotSatisfiable, err.Error()})
		return
	case err != nil:
		if r.Context().Err() != nil {
			return // player moved on (seek/close); nothing to report
		}
		// err carries ffmpeg's last lines, which name files on the server.
		slog.Error("hls segment", "session", chi.URLParam(r, "sid"), "segment", n, "err", err)
		writeErr(w, httpError{http.StatusInternalServerError, "the server couldn't convert this video; the server log has the reason"})
		return
	}
	w.Header().Set("Content-Type", "video/mp2t")
	http.ServeFile(w, r, path)
}

func (s *Server) closeHLS(w http.ResponseWriter, r *http.Request) {
	s.tc.Close(chi.URLParam(r, "sid"))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) transcodeSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"hwaccel":     s.tc.Encoder().HW,
		"maxSessions": s.tc.Max(),
		"sessions":    s.tc.Sessions(),
	})
}

// --- background optimize encodes ---------------------------------------------

func (s *Server) optimizeItem(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	files, err := s.db.OptimizeCandidates(r.Context(), id, -1)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.enqueueOptimize(w, r, files)
}

func (s *Server) optimizeLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	files, err := s.db.OptimizeCandidates(r.Context(), -1, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.enqueueOptimize(w, r, files)
}

// optimizeFile queues an optimized copy of one file, whether or not it
// already plays in the browser: the admin asked for this one.
func (s *Server) optimizeFile(w http.ResponseWriter, r *http.Request) {
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
	if f.Problem != "" {
		writeErr(w, badRequest("this file can't be read"))
		return
	}
	s.enqueueOptimize(w, r, []db.File{f})
}

func (s *Server) enqueueOptimize(w http.ResponseWriter, r *http.Request, files []db.File) {
	for _, f := range files {
		if err := s.worker.Enqueue(r.Context(), worker.KindOptimize, f.ID, "Encode "+filepath.Base(f.Path)); err != nil {
			writeErr(w, err)
			return
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]int{"queued": len(files)})
}

func (s *Server) deleteOptimized(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	p, err := s.db.DeleteOptimized(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if p != "" {
		_ = os.Remove(p)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	ok, err := s.worker.Cancel(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !ok {
		writeErr(w, badRequest("job is not queued or running"))
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// ProblemMessage explains a file problem flagged by the scanner.
func ProblemMessage(problem string) string {
	switch problem {
	case "unreadable":
		return "This file is damaged or incomplete, so it can't be played. Replace it on the NAS and rescan."
	case "no-video":
		return "This file has no video Couchside can decode. It's most likely a DRM-protected iTunes purchase."
	}
	return ""
}
