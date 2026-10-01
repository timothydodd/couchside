package api

import (
	"net/http"

	"github.com/timothydodd/couchside/internal/db"
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
