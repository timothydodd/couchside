// Couchside: a lightweight self-hosted media server.
package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/timothydodd/couchside/internal/api"
	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/discovery"
	"github.com/timothydodd/couchside/internal/livetv"
	"github.com/timothydodd/couchside/internal/logbuf"
	"github.com/timothydodd/couchside/internal/metadata"
	"github.com/timothydodd/couchside/internal/transcode"
	"github.com/timothydodd/couchside/internal/webui"
	"github.com/timothydodd/couchside/internal/worker"
)

// logs keeps recent log lines for System → Console.
var logs = logbuf.New(2000)

// version is stamped at build time with -ldflags "-X main.version=...".
var version = "dev"

// setLogger sends the log to w, and to System → Console.
func setLogger(w io.Writer) {
	level := slog.LevelInfo
	if os.Getenv("COUCHSIDE_DEBUG") != "" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(logs.Handler(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))))
}

func main() {
	// Started by the Windows service manager: it has no console, and stops
	// the server through the service's control requests (service_windows.go).
	if isService() {
		serviceMain()
		return
	}
	setLogger(os.Stdout)

	if len(os.Args) > 1 && os.Args[1] == "reset-password" {
		if err := resetPassword(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "reset-password:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "restore" {
		if err := restore(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "restore:", err)
			os.Exit(1)
		}
		return
	}
	if err := run(context.Background()); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// run serves until parent is cancelled, Ctrl-C or SIGTERM. Settings →
// Server's Restart stops everything and starts it again in this process, so
// saved settings apply the same way under the Windows service, Docker or a
// terminal.
//
// If it can't start with the saved settings (one the server can't use), it
// starts once more without them, so the web is still there to fix them.
func run(parent context.Context) error {
	ignoreSaved := ""
	for {
		again, err := serve(parent, ignoreSaved)
		if errors.Is(err, errSavedSettings) && ignoreSaved == "" && parent.Err() == nil {
			slog.Error("couldn't start with the saved settings; starting without them (fix them in Settings → Server)", "err", err)
			ignoreSaved = strings.TrimPrefix(err.Error(), errSavedSettings.Error()+": ")
			continue
		}
		if err != nil || !again || parent.Err() != nil {
			return err
		}
		slog.Info("restarting to apply settings")
		ignoreSaved = ""
	}
}

// errSavedSettings marks a start-up that failed while using saved settings.
var errSavedSettings = errors.New("with the saved settings")

// serve runs the server once. It returns true when it stopped for a restart.
// ignoreSaved, when set, is why the saved settings are left out this time.
func serve(parent context.Context, ignoreSaved string) (again bool, err error) {
	cfg := config.Load()
	if cfg.EnvFile != "" {
		slog.Info("settings file", "path", cfg.EnvFile)
	}
	// The data folder holds the session key, password hashes and API keys:
	// only the server's own user may read it. The cache holds nothing secret.
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return false, err
	}
	tightenDir(cfg.DataDir)
	if err := os.MkdirAll(cfg.CacheDir, 0o755); err != nil {
		return false, err
	}
	cfg.ServerID = serverID(cfg.DataDir)
	database, err := db.Open(filepath.Join(cfg.DataDir, "couchside.db"))
	if err != nil {
		return false, err
	}
	defer database.Close()
	// What admins saved in Settings → Server takes the place of those
	// environment variables.
	saved, err := database.EnvOverrides(parent)
	if err != nil {
		return false, err
	}
	// Settings an older version let admins save from the web that can only
	// come from the environment now (the ffmpeg and comskip programs).
	for k := range saved {
		if _, ok := config.EditableSetting(k); !ok {
			slog.Warn("ignoring a saved setting that can no longer be set from the web; set it in the environment or the settings file instead", "key", k)
			_ = database.SetEnvOverride(parent, k, nil)
			delete(saved, k)
		}
	}
	if ignoreSaved != "" {
		saved = nil
	}
	if len(saved) > 0 {
		defer func() {
			if err != nil && !again {
				err = fmt.Errorf("%w: %w", errSavedSettings, err)
			}
		}()
	}
	if len(saved) > 0 {
		id := cfg.ServerID
		cfg = config.LoadWith(saved)
		cfg.ServerID = id
		keys := make([]string, 0, len(saved))
		for k := range saved {
			keys = append(keys, k)
		}
		slog.Info("settings from Settings → Server", "keys", strings.Join(keys, ","))
	}
	// Media locations before anything reads media: libraries may be on
	// network shares to sign in to.
	shareStatus := api.PrepareLocations(parent, database, cfg)

	// TMDB first (backdrops, no sign-up), OMDb as a fallback when configured.
	providers := &metadata.Chain{}
	if cfg.TMDBKey != "" {
		providers.Providers = append(providers.Providers, metadata.NewTMDB(cfg.TMDBKey, database))
	}
	if cfg.OMDbKey != "" {
		providers.Providers = append(providers.Providers, metadata.NewOMDb(cfg.OMDbKey, database))
	}
	if providers.Empty() {
		slog.Warn("no metadata provider: set TMDB_API_KEY (or OMDB_API_KEY); items will show with filename titles and no posters")
	}

	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var restart atomic.Bool

	enc := transcode.Detect(ctx, cfg.FFmpeg, cfg.HWAccel, cfg.VAAPIDevice)
	tc, err := transcode.NewManager(enc, cfg.FFprobe, filepath.Join(cfg.CacheDir, "transcode"), cfg.MaxTranscodes)
	if err != nil {
		return false, err
	}
	slog.Info("transcoding", "hwaccel", enc.HW, "tonemap", enc.Tonemap, "maxSessions", cfg.MaxTranscodes)

	w := worker.New(database, cfg, providers, enc)
	workerDone := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(workerDone)
	}()
	tcDone := make(chan struct{})
	go func() {
		tc.Run(ctx)
		close(tcDone)
	}()

	// Live TV runs even without a tuner: it also serves Couchside's own
	// virtual channels, built from the library.
	tv, err := livetv.New(livetv.Config{Tuner: cfg.HDHomeRun, RecordingsDir: cfg.RecordingsDir, FFmpeg: cfg.FFmpeg,
		FFprobe: cfg.FFprobe, MaxEncodes: cfg.MaxTranscodes, PadBefore: cfg.PadBefore, PadAfter: cfg.PadAfter, Metadata: providers}, database, enc, w, cfg.CacheDir)
	if err != nil {
		return false, err
	}
	if cfg.HDHomeRun != "" {
		slog.Info("live tv enabled", "tuner", cfg.HDHomeRun, "recordings", cfg.RecordingsDir)
	}
	tvDone := make(chan struct{})
	go func() {
		tv.Run(ctx)
		close(tvDone)
	}()

	apiServer, err := api.New(database, cfg, w, providers, tc, tv, version)
	if err != nil {
		return false, err
	}
	apiServer.UseLogs(logs)
	apiServer.UseShares(shareStatus)
	apiServer.UseRestart(saved, ignoreSaved, func() {
		restart.Store(true)
		cancel()
	})
	go apiServer.Run(ctx)
	if cfg.Discovery {
		go runDiscovery(ctx, cfg, version, apiServer)
	}
	if cfg.Auth {
		slog.Info("COUCHSIDE_AUTH is set: every profile signs in with a password (no passwordless sign-in)")
	}
	// No ReadTimeout or WriteTimeout: streams and uploads run long.
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           apiServer.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		<-ctx.Done()
		// A second Ctrl-C or SIGTERM now exits at once.
		stop()
		// Kubernetes allows 60s (the chart's terminationGracePeriodSeconds):
		// 20 for requests to finish, then cut the rest (a long direct-play
		// download) so the worker and the database close before the kill.
		shutdown, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
		_ = srv.Close()
	}()

	web := cfg.WebDir
	if web == "" {
		web = "embedded"
		if webui.FS() == nil {
			web = "none (API only; set COUCHSIDE_WEB_DIR)"
		}
	}
	slog.Info("couchside listening", "addr", cfg.Addr, "version", version, "web", web, "providers", providers.Names())
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		cancel()
		return false, err
	}
	// ListenAndServe returns as soon as Shutdown starts; requests still
	// draining need the database, which closes when run returns.
	<-drained
	<-workerDone
	<-tcDone
	<-tvDone
	return restart.Load(), nil
}

// runDiscovery answers SSDP searches from TV apps on the LAN. A server that
// can't listen (port 1900 taken, no multicast on a container network) logs
// it and carries on: typing the address still works.
func runDiscovery(ctx context.Context, cfg config.Config, version string, apiServer *api.Server) {
	port := 8080
	if _, p, err := net.SplitHostPort(cfg.Addr); err == nil {
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
	}
	d := &discovery.Responder{ID: cfg.ServerID, Name: cfg.ServerName, Version: version, Port: port,
		URL: cfg.DiscoveryURL, Interface: cfg.DiscoveryInterface, SignIn: apiServer.SignInMode}
	if err := d.Run(ctx); err != nil {
		slog.Warn("lan discovery is off: TV apps need the server's address typed in", "err", err)
	}
}

// serverID is this server's stable id, made once and kept in the data folder.
func serverID(dataDir string) string {
	p := filepath.Join(dataDir, "server.id")
	if b, err := os.ReadFile(p); err == nil {
		if id := strings.TrimSpace(string(b)); id != "" {
			return id
		}
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	id := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	if err := os.WriteFile(p, []byte(id+"\n"), 0o600); err != nil {
		slog.Warn("couldn't save the server id", "err", err)
	}
	return id
}

// tightenDir takes group and other access off a folder that holds secrets,
// for data folders made by older versions (MkdirAll doesn't change an
// existing folder). Windows has no POSIX modes; the installer's ACLs cover
// it. A volume that refuses (some NFS mounts) only gets a warning.
func tightenDir(p string) {
	if runtime.GOOS == "windows" {
		return
	}
	if st, err := os.Stat(p); err == nil && st.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(p, 0o700); err != nil {
			slog.Warn("couldn't restrict the data folder to Couchside's user", "path", p, "err", err)
		}
	}
}
