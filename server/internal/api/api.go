// Package api serves the REST API, artwork, media streams and the web UI.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/timothydodd/couchside/internal/auth"
	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/livetv"
	"github.com/timothydodd/couchside/internal/metadata"
	"github.com/timothydodd/couchside/internal/sysstat"
	"github.com/timothydodd/couchside/internal/transcode"
	"github.com/timothydodd/couchside/internal/usererr"
	"github.com/timothydodd/couchside/internal/webui"
	"github.com/timothydodd/couchside/internal/worker"
)

type Server struct {
	db        *db.DB
	cfg       config.Config
	worker    *worker.Worker
	providers *metadata.Chain
	tc        *transcode.Manager
	tv        *livetv.Service // nil when no tuner is configured
	version   string
	presence  *presence
	sys       sysstat.Sampler
	index     searchIndex
	auth      *authState
}

func init() {
	// Go's mime table doesn't know the web app manifest.
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
}

func New(d *db.DB, cfg config.Config, w *worker.Worker, providers *metadata.Chain, tc *transcode.Manager, tv *livetv.Service, version string) (*Server, error) {
	s := &Server{db: d, cfg: cfg, worker: w, providers: providers, tc: tc, tv: tv, version: version, presence: newPresence()}
	key, err := auth.LoadKey(filepath.Join(cfg.DataDir, "auth.key"))
	if err != nil {
		return nil, fmt.Errorf("auth key: %w", err)
	}
	s.auth = newAuthState(key)
	return s, nil
}

// Run does the server's background upkeep until ctx ends.
func (s *Server) Run(ctx context.Context) {
	go func() {
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for {
			s.pruneRemote()
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	// Print the setup code at start-up if there's no admin yet.
	if _, err := s.setupNeeded(ctx); err != nil {
		slog.Error("accounts", "err", err)
	}
	s.pruneSessions(ctx)
	if s.cfg.Auth {
		// Passwords are required now; sessions from passwordless days end.
		if err := s.endPasswordlessSessions(ctx); err != nil {
			slog.Error("accounts", "err", err)
		}
	}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP, middleware.Recoverer)

	r.Get("/healthz", s.health)
	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.NoCache)
		// Open: how to sign in, and signing in.
		r.Get("/auth", s.authStatus)
		r.Post("/auth/login", s.login)
		r.Post("/auth/refresh", s.refresh)
		r.Post("/auth/setup", s.setup)
		r.Post("/auth/pick", s.pick)

		r.Group(func(r chi.Router) {
			r.Use(s.authenticate, s.presence.track)
			// Your own account; reachable while a temporary password still has to be changed.
			r.Post("/auth/logout", s.logout)
			r.Post("/auth/password", s.changePassword)
			r.Get("/auth/sessions", s.mySessions)
			r.Post("/auth/sessions/others/end", s.endOtherSessions)
			r.Delete("/auth/sessions/{sid}", s.endSession)

			r.Group(func(r chi.Router) {
				r.Use(s.passwordCurrent)
				s.userRoutes(r)
				r.Group(func(r chi.Router) {
					r.Use(recorders)
					s.recorderRoutes(r)
				})
				r.Group(func(r chi.Router) {
					r.Use(adminOnly)
					s.adminRoutes(r)
				})
			})
		})
	})
	// Artwork and streams sit outside the no-cache group. Artwork is open even
	// with accounts on, so TV apps' image nodes needn't send a token.
	r.Get("/api/artwork/items/{id}/{kind}", s.itemArtwork)
	r.Get("/api/artwork/files/{id}/still", s.fileStill)
	r.Get("/api/artwork/people/{id}", s.personPhoto)
	r.Get("/api/artwork/remote", s.remoteImage)
	r.Group(func(r chi.Router) {
		r.Use(s.authenticate, s.passwordCurrent)
		r.Get("/api/files/{id}/stream", s.stream)
		r.Get("/api/files/{id}/subtitles/{key}", s.subtitleVTT)
	})

	switch {
	case s.cfg.WebDir != "":
		r.NotFound(spa(os.DirFS(s.cfg.WebDir)))
	case webui.FS() != nil:
		r.NotFound(spa(webui.FS()))
	}
	return logRequests(r)
}

// userRoutes are for anyone signed in: browsing, watching, live TV.
func (s *Server) userRoutes(r chi.Router) {
	r.Get("/status", s.status)
	r.Get("/home", s.home)
	r.Get("/search", s.search)

	r.Get("/profiles", s.listProfiles)
	r.Put("/profiles/{id}", s.updateProfile)
	r.Patch("/profiles/{id}/prefs", s.profilePrefs)

	r.Get("/items", s.listItems)
	r.Get("/items/{id}", s.getItem)
	r.Get("/people/{id}", s.person)
	r.Post("/items/{id}/watched", s.itemWatched)

	r.Get("/files/{id}", s.playInfo)
	r.Put("/files/{id}/progress", s.saveProgress)
	r.Post("/files/{id}/watched", s.fileWatched)
	r.Post("/files/{id}/hls", s.createHLS)
	r.Get("/files/{id}/streams", s.fileStreams)
	r.Get("/files/{id}/commercials", s.commercials)
	r.Post("/files/{id}/commercials", s.findCommercials)
	r.Get("/hls/{sid}/index.m3u8", s.hlsPlaylist)
	r.Get("/hls/{sid}/{seg}", s.hlsSegment)
	r.Delete("/hls/{sid}", s.closeHLS)

	r.Get("/livetv/status", s.tvStatus)
	r.Get("/livetv/channels", s.tvChannels)
	r.Put("/livetv/channels/{number}/pin", s.tvPin)
	r.Get("/livetv/guide", s.tvGuide)
	r.Post("/livetv/watch", s.tvWatch)
	r.Get("/live/{sid}/{file}", s.tvLiveFile)
	r.Delete("/live/{sid}", s.tvLeave)
	r.Get("/dvr/recordings", s.dvrList)
	r.Post("/dvr/recordings/{id}/watch", s.dvrWatch)
	r.Get("/dvr/rules", s.rulesList)
	r.Get("/dvr/rules/options", s.ruleOptions)
	r.Get("/settings/timing", s.getTiming)
}

// recorderRoutes schedule recordings and series rules: admins, and users an
// admin has allowed to record (who can only change their own).
func (s *Server) recorderRoutes(r chi.Router) {
	r.Post("/dvr/recordings", s.dvrRecord)
	r.Post("/dvr/recordings/{id}/cancel", s.dvrCancel)
	r.Delete("/dvr/recordings/{id}", s.dvrDelete)
	r.Post("/dvr/rules", s.ruleCreate)
	r.Put("/dvr/rules/{id}", s.ruleUpdate)
	r.Delete("/dvr/rules/{id}", s.ruleDelete)
}

// adminRoutes are settings, libraries, file management, jobs and accounts.
func (s *Server) adminRoutes(r chi.Router) {
	r.Get("/system", s.system)

	r.Get("/accounts", s.listAccounts)
	r.Put("/settings/passwordless", s.setPasswordless)
	r.Post("/accounts", s.createAccount)
	r.Put("/accounts/{id}", s.updateAccount)
	r.Post("/accounts/{id}/password", s.resetPassword)
	r.Delete("/accounts/{id}", s.deleteAccount)
	r.Get("/accounts/{id}/sessions", s.accountSessions)

	r.Get("/livetv/virtual", s.listVirtual)
	r.Get("/livetv/virtual/options", s.virtualOptions)
	r.Post("/livetv/virtual/preview", s.previewVirtual)
	r.Post("/livetv/virtual", s.createVirtual)
	r.Put("/livetv/virtual/{id}", s.updateVirtual)
	r.Delete("/livetv/virtual/{id}", s.deleteVirtual)

	r.Get("/libraries", s.listLibraries)
	r.Post("/libraries", s.createLibrary)
	r.Put("/libraries/{id}", s.updateLibrary)
	r.Delete("/libraries/{id}", s.deleteLibrary)
	r.Post("/libraries/{id}/scan", s.scanLibrary)
	r.Post("/libraries/scan", s.scanAll)
	r.Post("/libraries/{id}/optimize", s.optimizeLibrary)
	r.Post("/libraries/{id}/rematch", s.rematchLibrary)
	r.Get("/libraries/{id}/manage", s.manageItems)
	r.Get("/fs", s.browse)

	r.Post("/items/{id}/match", s.rematch)
	r.Post("/items/{id}/optimize", s.optimizeItem)
	r.Post("/items/{id}/commercials", s.findItemCommercials)
	r.Get("/items/{id}/files", s.itemFiles)
	r.Get("/items/{id}/lookup", s.itemLookup)
	r.Delete("/items/{id}", s.deleteItem)
	r.Put("/items/{id}/artwork/{kind}", s.uploadArtwork)
	r.Delete("/items/{id}/artwork/{kind}", s.resetArtwork)

	r.Delete("/files/{id}/optimized", s.deleteOptimized)
	r.Delete("/files/{id}", s.deleteFile)
	r.Put("/files/{id}/role", s.setFileRole)
	r.Get("/transcode", s.transcodeSessions)

	r.Post("/livetv/refresh", s.tvRefresh)
	r.Put("/settings/timing", s.saveTiming)
	r.Get("/dvr/settings", s.dvrSettings)
	r.Put("/dvr/settings", s.dvrSaveSettings)

	r.Get("/jobs", s.listJobs)
	r.Post("/jobs/{id}/retry", s.retryJob)
	r.Post("/jobs/{id}/cancel", s.cancelJob)
	r.Post("/jobs/clear", s.clearJobs)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.db.Ping(r.Context()); err != nil {
		http.Error(w, "db: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.Write([]byte("ok"))
}

// --- helpers -----------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, db.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	default:
		var he httpError
		if errors.As(err, &he) {
			writeJSON(w, he.status, map[string]string{"error": he.msg})
			return
		}
		slog.Error("request failed", "err", err)
		msg := "internal error; see the server log"
		if usererr.Is(err) {
			msg = err.Error() // written for the user ("all tuners are busy")
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg})
	}
}

type httpError struct {
	status int
	msg    string
}

func (e httpError) Error() string { return e.msg }

func badRequest(msg string) error { return httpError{http.StatusBadRequest, msg} }

func idParam(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		return 0, badRequest("bad id")
	}
	return id, nil
}

func decode(r *http.Request, v any) error {
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(v); err != nil {
		return badRequest("bad JSON body: " + err.Error())
	}
	return nil
}

// spa serves the built frontend, falling back to index.html for client routes.
func spa(fsys fs.FS) http.HandlerFunc {
	fsrv := http.FileServerFS(fsys)
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if st, err := fs.Stat(fsys, name); err == nil && !st.IsDir() {
			if strings.HasPrefix(r.URL.Path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fsrv.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, fsys, "index.html")
	}
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		if strings.HasPrefix(r.URL.Path, "/api/") && ww.Status() >= 400 {
			slog.Warn("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "took", time.Since(start))
		}
	})
}
