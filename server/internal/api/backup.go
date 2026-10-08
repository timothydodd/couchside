package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/timothydodd/couchside/internal/backup"
)

// Backups of the database (with auth.key and server.id) in $DATA/backups:
// one a day by default, the newest few kept, plus any made by hand. See
// internal/backup. Settings → System → Advanced lists, makes, downloads and deletes
// them; `couchside restore <file>` puts one back.
const (
	settingBackupDaily = "backup.daily" // "0" turns the daily backup off
	settingBackupKeep  = "backup.keep"  // how many daily backups to keep
	defaultBackupKeep  = 7
	maxBackupKeep      = 90
)

type backupSettings struct {
	Daily bool `json:"daily"`
	Keep  int  `json:"keep"`
}

func (s *Server) backupSettings(ctx context.Context) backupSettings {
	out := backupSettings{Daily: true, Keep: defaultBackupKeep}
	if v, err := s.db.Setting(ctx, settingBackupDaily); err == nil && v == "0" {
		out.Daily = false
	}
	if v, err := s.db.Setting(ctx, settingBackupKeep); err == nil && v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= maxBackupKeep {
			out.Keep = n
		}
	}
	return out
}

// runBackups makes the daily backup when the newest one is a day old, and
// prunes. It checks hourly, so a server that was off at the usual time
// catches up soon after it starts.
func (s *Server) runBackups(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		s.backupIfDue(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Server) backupIfDue(ctx context.Context) {
	set := s.backupSettings(ctx)
	all, err := backup.List(s.cfg.DataDir)
	if err != nil {
		slog.Warn("backups", "err", err)
		return
	}
	if set.Daily {
		due := true
		for _, b := range all {
			if b.Kind == backup.Scheduled && time.Since(time.Unix(b.At, 0)) < 24*time.Hour {
				due = false
			}
		}
		if due {
			b, err := backup.Create(ctx, s.db, s.cfg.DataDir, backup.Scheduled)
			if err != nil {
				if ctx.Err() == nil {
					slog.Error("daily backup failed", "err", err)
				}
				return
			}
			slog.Info("backed up the database", "file", b.Name, "bytes", b.Size)
		}
	}
	if err := backup.Prune(s.cfg.DataDir, set.Keep); err != nil {
		slog.Warn("prune backups", "err", err)
	}
}

func (s *Server) listBackups(w http.ResponseWriter, r *http.Request) {
	all, err := backup.List(s.cfg.DataDir)
	if err != nil {
		writeErr(w, err)
		return
	}
	set := s.backupSettings(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"backups": all, "daily": set.Daily, "keep": set.Keep, "dir": backup.Dir(s.cfg.DataDir)})
}

func (s *Server) createBackup(w http.ResponseWriter, r *http.Request) {
	b, err := backup.Create(r.Context(), s.db, s.cfg.DataDir, backup.Manual)
	if err != nil {
		writeErr(w, err)
		return
	}
	slog.Info("backed up the database", "file", b.Name, "bytes", b.Size, "by", currentUser(r.Context()).ID)
	writeJSON(w, http.StatusCreated, b)
}

func (s *Server) downloadBackup(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	p, err := backup.Path(s.cfg.DataDir, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// It holds password hashes and the key that signs sessions.
	slog.Info("backup downloaded", "file", name, "by", currentUser(r.Context()).ID, "ip", clientIP(r))
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, p)
}

func (s *Server) deleteBackup(w http.ResponseWriter, r *http.Request) {
	err := backup.Delete(s.cfg.DataDir, chi.URLParam(r, "name"))
	if errors.Is(err, os.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setBackupSettings(w http.ResponseWriter, r *http.Request) {
	var in backupSettings
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if in.Keep < 1 || in.Keep > maxBackupKeep {
		writeErr(w, badRequest("keep between 1 and "+strconv.Itoa(maxBackupKeep)+" daily backups"))
		return
	}
	daily := "1"
	if !in.Daily {
		daily = "0"
	}
	ctx := r.Context()
	if err := s.db.SetSetting(ctx, settingBackupDaily, daily); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.db.SetSetting(ctx, settingBackupKeep, strconv.Itoa(in.Keep)); err != nil {
		writeErr(w, err)
		return
	}
	if err := backup.Prune(s.cfg.DataDir, in.Keep); err != nil {
		slog.Warn("prune backups", "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
}
