package api

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/livetv"
)

var errNoTuner = httpError{http.StatusNotFound, "Live TV isn't set up: set COUCHSIDE_HDHOMERUN to your tuner's IP address"}

func (s *Server) tvStatus(w http.ResponseWriter, r *http.Request) {
	if s.tv == nil {
		writeJSON(w, http.StatusOK, map[string]any{"configured": false})
		return
	}
	st := s.tv.Status(r.Context())
	if currentUser(r.Context()).Admin {
		writeJSON(w, http.StatusOK, st)
		return
	}
	// Others see what the Live TV page needs, not the recordings folder,
	// other clients' addresses or the tuner's own address in errors.
	out := map[string]any{"configured": st.Configured, "tuner": st.Tuner, "virtualChannels": st.Virtual,
		"tunersInUse": st.TunersInUse, "recording": st.Recording}
	if st.Device != nil {
		out["device"] = map[string]any{"FriendlyName": st.Device.FriendlyName, "TunerCount": st.Device.TunerCount}
	}
	if st.Error != "" {
		out["error"] = "the tuner isn't answering"
	}
	if st.GuideError != "" {
		out["guideError"] = "the guide couldn't be updated"
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) tvRefresh(w http.ResponseWriter, r *http.Request) {
	if s.tv == nil {
		writeErr(w, errNoTuner)
		return
	}
	if err := s.tv.RefreshNow(r.Context()); err != nil {
		writeErr(w, httpError{http.StatusBadGateway, err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type channelNow struct {
	db.Channel
	Now  *db.Program `json:"now"`
	Next *db.Program `json:"next"`
}

// tvChannels lists channels with what's on now and next.
func (s *Server) tvChannels(w http.ResponseWriter, r *http.Request) {
	if s.tv == nil {
		writeErr(w, errNoTuner)
		return
	}
	ctx := r.Context()
	chans, err := s.db.Channels(ctx)
	if err != nil {
		writeErr(w, err)
		return
	}
	now := time.Now().Unix()
	progs, err := s.db.ProgramsBetween(ctx, now, now+6*3600)
	if err != nil {
		writeErr(w, err)
		return
	}
	byCh := map[string][]db.Program{}
	for _, p := range progs {
		byCh[p.Channel] = append(byCh[p.Channel], p)
	}
	out := make([]channelNow, 0, len(chans))
	for _, c := range chans {
		cn := channelNow{Channel: c}
		for i := range byCh[c.Number] {
			p := byCh[c.Number][i]
			if p.StartAt <= now && p.EndAt > now {
				cn.Now = &p
			} else if p.StartAt > now && cn.Next == nil {
				cn.Next = &p
			}
		}
		out = append(out, cn)
	}
	writeJSON(w, http.StatusOK, out)
}

// tvGuide returns every channel's programs overlapping [start, start+hours).
func (s *Server) tvGuide(w http.ResponseWriter, r *http.Request) {
	if s.tv == nil {
		writeErr(w, errNoTuner)
		return
	}
	start, _ := strconv.ParseInt(r.URL.Query().Get("start"), 10, 64)
	if start == 0 {
		start = time.Now().Unix()
	}
	hours, _ := strconv.Atoi(r.URL.Query().Get("hours"))
	if hours <= 0 || hours > 24 {
		hours = 4
	}
	ctx := r.Context()
	chans, err := s.db.Channels(ctx)
	if err != nil {
		writeErr(w, err)
		return
	}
	progs, err := s.db.ProgramsBetween(ctx, start, start+int64(hours)*3600)
	if err != nil {
		writeErr(w, err)
		return
	}
	byCh := map[string][]db.Program{}
	for _, p := range progs {
		byCh[p.Channel] = append(byCh[p.Channel], p)
	}
	type row struct {
		db.Channel
		Programs []db.Program `json:"programs"`
	}
	out := make([]row, 0, len(chans))
	for _, c := range chans {
		ps := byCh[c.Number]
		if ps == nil {
			ps = []db.Program{}
		}
		out = append(out, row{Channel: c, Programs: ps})
	}
	through, _ := s.db.GuideCoverage(ctx)
	writeJSON(w, http.StatusOK, map[string]any{"start": start, "hours": hours, "through": through, "channels": out})
}

func (s *Server) tvWatch(w http.ResponseWriter, r *http.Request) {
	if s.tv == nil {
		writeErr(w, errNoTuner)
		return
	}
	var in struct {
		Channel string `json:"channel"`
		Height  int    `json:"height"`
		// Codecs the client can decode itself (a Roku: mpeg2, h264, ac3…): a
		// channel in them is passed through instead of transcoded.
		VideoCodecs []string `json:"videoCodecs"`
		AudioCodecs []string `json:"audioCodecs"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	sess, err := s.tv.Watch(r.Context(), in.Channel, livetv.WatchOpts{Height: in.Height, VideoCodecs: in.VideoCodecs, AudioCodecs: in.AudioCodecs})
	switch {
	case errors.Is(err, livetv.ErrNoTuner):
		writeErr(w, httpError{http.StatusServiceUnavailable, "All tuners are busy (recordings, other viewers or Plex). Try again when one frees up."})
		return
	case errors.Is(err, livetv.ErrBusyEncoding):
		writeErr(w, httpError{http.StatusTooManyRequests, err.Error()})
		return
	case err != nil:
		writeErr(w, err)
		return
	}
	prog, _ := s.db.ProgramAt(r.Context(), sess.Channel, time.Now().Unix())
	pl := &playing{Kind: "live", Title: sess.Name, Mode: "Live TV"}
	if prog != nil {
		pl.Subtitle = prog.Title
	}
	s.presence.setPlaying(r, pl)
	writeJSON(w, http.StatusOK, map[string]any{
		"sessionId": sess.ID, "playlist": "/api/live/" + sess.ID + "/index.m3u8",
		"channel": sess.Channel, "name": sess.Name, "height": sess.Height, "hw": sess.HW, "now": prog,
		"copyVideo": sess.CopyVideo, "copyAudio": sess.CopyAudio, "hwDecode": sess.HWDecode, "virtual": sess.Virtual,
	})
}

func (s *Server) tvLiveFile(w http.ResponseWriter, r *http.Request) {
	if s.tv == nil {
		writeErr(w, errNoTuner)
		return
	}
	name := chi.URLParam(r, "file")
	s.presence.keepPlaying(r)
	p, err := s.tv.LiveFile(chi.URLParam(r, "sid"), name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		writeErr(w, err)
		return
	}
	if name == "index.m3u8" {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	} else {
		w.Header().Set("Content-Type", "video/mp2t")
	}
	http.ServeFile(w, r, p)
}

func (s *Server) tvLeave(w http.ResponseWriter, r *http.Request) {
	if s.tv != nil {
		s.tv.LeaveLive(chi.URLParam(r, "sid"))
	}
	s.presence.setPlaying(r, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) dvrList(w http.ResponseWriter, r *http.Request) {
	if s.tv == nil {
		writeErr(w, errNoTuner)
		return
	}
	recs, err := s.db.Recordings(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, recs)
}

func (s *Server) dvrRecord(w http.ResponseWriter, r *http.Request) {
	if s.tv == nil {
		writeErr(w, errNoTuner)
		return
	}
	var in struct {
		ProgramID int64 `json:"programId"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	id, overlap, existing, err := s.tv.Record(r.Context(), in.ProgramID, currentUser(r.Context()).ID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeErr(w, err)
			return
		}
		writeErr(w, badRequest(err.Error()))
		return
	}
	tuners := 0
	if st := s.tv.Status(r.Context()); st.Device != nil {
		tuners = st.Device.TunerCount
	}
	code := http.StatusCreated
	if existing {
		// Already set to record (perhaps by someone else's rule): it will be recorded anyway.
		code = http.StatusOK
	}
	writeJSON(w, code, map[string]any{"id": id, "overlapping": overlap, "tuners": tuners,
		"conflict": tuners > 0 && overlap >= tuners})
}

func (s *Server) dvrCancel(w http.ResponseWriter, r *http.Request) {
	if s.tv == nil {
		writeErr(w, errNoTuner)
		return
	}
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.ownRecording(r, id); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.tv.Cancel(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) dvrDelete(w http.ResponseWriter, r *http.Request) {
	if s.tv == nil {
		writeErr(w, errNoTuner)
		return
	}
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.ownRecording(r, id); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.tv.Delete(r.Context(), id); err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ownRecording refuses changes to a recording someone else scheduled, unless
// the user is an admin.
func (s *Server) ownRecording(r *http.Request, id int64) error {
	rec, err := s.db.Recording(r.Context(), id)
	if err != nil {
		return err
	}
	if !currentUser(r.Context()).mayManage(rec.OwnerID) {
		return forbidden("only whoever scheduled this recording, or an admin, can change it")
	}
	return nil
}

// dvrWatch starts playback of an in-progress recording from its beginning.
func (s *Server) dvrWatch(w http.ResponseWriter, r *http.Request) {
	if s.tv == nil {
		writeErr(w, errNoTuner)
		return
	}
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in struct {
		Height      int      `json:"height"`
		VideoCodecs []string `json:"videoCodecs"`
		AudioCodecs []string `json:"audioCodecs"`
	}
	_ = decode(r, &in)
	sess, err := s.tv.WatchRecording(r.Context(), id, livetv.WatchOpts{Height: in.Height, VideoCodecs: in.VideoCodecs, AudioCodecs: in.AudioCodecs})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeErr(w, err)
			return
		}
		if errors.Is(err, livetv.ErrBusyEncoding) {
			writeErr(w, httpError{http.StatusTooManyRequests, err.Error()})
			return
		}
		writeErr(w, badRequest(err.Error()))
		return
	}
	rec, _ := s.db.Recording(r.Context(), id)
	pl := &playing{Kind: "recording", Title: sess.Name, Mode: "Recording in progress"}
	if rec.Title != "" {
		pl.Title, pl.Subtitle = rec.Title, rec.EpisodeTitle
	}
	s.presence.setPlaying(r, pl)
	writeJSON(w, http.StatusOK, map[string]any{
		"sessionId": sess.ID, "playlist": "/api/live/" + sess.ID + "/index.m3u8",
		"channel": sess.Channel, "name": sess.Name, "height": sess.Height, "hw": sess.HW, "recording": rec,
		"copyVideo": sess.CopyVideo, "copyAudio": sess.CopyAudio, "hwDecode": sess.HWDecode,
	})
}

// tvPin pins or unpins a channel ({"pinned": true}).
func (s *Server) tvPin(w http.ResponseWriter, r *http.Request) {
	if s.tv == nil {
		writeErr(w, errNoTuner)
		return
	}
	var in struct {
		Pinned bool `json:"pinned"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.db.SetChannelPinned(r.Context(), chi.URLParam(r, "number"), in.Pinned); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
