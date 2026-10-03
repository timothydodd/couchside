package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/logbuf"
)

// system is the Settings page's live view: CPU and memory, who's connected
// and what they're watching, and the server's streams and encodes.
func (s *Server) system(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	profiles, err := s.db.Profiles(ctx)
	if err != nil {
		writeErr(w, err)
		return
	}
	type person struct {
		clientView
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	people := []person{}
	for _, c := range s.presence.connected(r) {
		p := person{clientView: c}
		for _, pr := range profiles {
			if pr.ID == c.ProfileID {
				p.Name, p.Color = pr.Name, pr.Color
			}
		}
		// Titles are looked up now rather than on every progress report.
		if pl := c.Playing; pl != nil && pl.Kind == "file" {
			if info, err := s.db.PlayInfo(db.WithProfile(ctx, c.ProfileID), pl.FileID); err == nil {
				pl.Title, pl.Subtitle = info.Title, info.Subtitle
			}
		}
		people = append(people, p)
	}
	live := map[string]any{"configured": false}
	if s.tv != nil {
		live = s.tv.Summary()
	}
	jobs, err := s.db.JobCounts(ctx)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"stats":      s.sys.Read(),
		"clients":    people,
		"transcodes": s.tc.Sessions(),
		"maxStreams": s.tc.Max(),
		"hwaccel":    s.tc.Encoder().HW,
		"livetv":     live,
		"jobs":       jobs,
	})
}

// UseLogs gives System → Console the buffer the server logs into.
func (s *Server) UseLogs(b *logbuf.Buffer) { s.logs = b }

// streamCount is what the server is producing right now: HLS sessions plus
// live TV channels being streamed.
func (s *Server) streamCount() int {
	n := len(s.tc.Sessions())
	if s.tv != nil {
		if live, ok := s.tv.Summary()["liveSessions"].(int); ok {
			n += live
		}
	}
	return n
}

// historyRanges are the windows the System page offers.
var historyRanges = map[string]time.Duration{"15m": 15 * time.Minute, "1h": time.Hour, "6h": 6 * time.Hour, "24h": 24 * time.Hour}

// systemHistory is CPU, memory and streams over the last ?range= (1h by
// default), kept in memory since the server started.
func (s *Server) systemHistory(w http.ResponseWriter, r *http.Request) {
	d, ok := historyRanges[r.URL.Query().Get("range")]
	if !ok {
		d = time.Hour
	}
	writeJSON(w, http.StatusOK, s.history.Points(d, time.Now()))
}

// systemLogs returns the log lines after ?after= (all that are kept when 0).
// gap means lines were dropped between after and the first one returned.
func (s *Server) systemLogs(w http.ResponseWriter, r *http.Request) {
	if s.logs == nil {
		writeJSON(w, http.StatusOK, map[string]any{"entries": []logbuf.Entry{}, "last": 0, "gap": false})
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	entries, last, gap := s.logs.Since(after)
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "last": last, "gap": gap})
}
