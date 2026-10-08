package api

import (
	"log/slog"
	"maps"
	"net/http"
	"time"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
)

// Settings → Server: the environment variables admins can set from the web
// instead (config.Editable). Saved values live in the settings table and
// apply when the server restarts, which POST /api/server/restart does in
// process.

// UseRestart hands over the Settings → Server values the server started
// with, why it started without them (when a restart with them failed), and
// how to restart it.
func (s *Server) UseRestart(started map[string]string, failed string, restart func()) {
	s.started, s.startFailed = maps.Clone(started), failed
	if s.started == nil {
		s.started = map[string]string{}
	}
	s.restart = restart
}

type serverSetting struct {
	config.Setting
	Value *string `json:"value"` // saved in Settings; nil when not
	Env   string  `json:"env"`   // the environment variable's value, "" when unset
	// Secrets never leave the server: only whether one is set.
	Saved   bool `json:"saved"`
	EnvSet  bool `json:"envSet"`
	Pending bool `json:"pending"` // saved since the server started: applies after a restart
}

func (s *Server) serverSettings(w http.ResponseWriter, r *http.Request) {
	saved, err := s.db.EnvOverrides(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	out := make([]serverSetting, 0, len(config.Editable))
	pending := false
	for _, st := range config.Editable {
		v, ok := saved[st.Key]
		was, wasOK := s.started[st.Key]
		row := serverSetting{Setting: st, Saved: ok, Env: config.Env(st.Key), Pending: ok != wasOK || v != was}
		row.EnvSet = row.Env != ""
		if ok {
			row.Value = &v
		}
		if st.Kind == "secret" {
			row.Value, row.Env = nil, ""
		}
		pending = pending || row.Pending
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"settings": out, "pending": pending, "envFile": s.cfg.EnvFile, "canRestart": s.restart != nil, "startFailed": s.startFailed,
	})
}

// setServerSettings saves the values sent ({"KEY": "value"}, or null to go
// back to the environment variable). All are checked before any is saved.
func (s *Server) setServerSettings(w http.ResponseWriter, r *http.Request) {
	var in map[string]*string
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	clean := map[string]*string{}
	for k, v := range in {
		st, ok := config.EditableSetting(k)
		if !ok {
			writeErr(w, badRequest(k+" can't be set here"))
			return
		}
		if v == nil {
			clean[k] = nil
			continue
		}
		c, err := st.Check(*v)
		if err != nil {
			writeErr(w, badRequest(err.Error()))
			return
		}
		clean[k] = &c
	}
	for k, v := range clean {
		if err := s.db.SetEnvOverride(r.Context(), k, v); err != nil {
			writeErr(w, err)
			return
		}
		if v == nil {
			slog.Info("setting reset to the environment", "key", k)
		} else if st, _ := config.EditableSetting(k); st.Kind == "secret" {
			slog.Info("setting saved", "key", k)
		} else {
			slog.Info("setting saved", "key", k, "value", *v)
		}
	}
	s.serverSettings(w, r)
}

// restartServer stops everything and starts it again with the saved
// settings. The response goes out first; the web app waits for /api/status.
func (s *Server) restartServer(w http.ResponseWriter, r *http.Request) {
	if s.restart == nil {
		writeErr(w, httpError{http.StatusNotImplemented, "this server can't restart itself"})
		return
	}
	slog.Info("restart requested", "by", db.ProfileID(r.Context()))
	w.WriteHeader(http.StatusAccepted)
	go func() {
		time.Sleep(300 * time.Millisecond)
		s.restart()
	}()
}
