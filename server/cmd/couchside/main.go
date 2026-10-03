// Couchside: a lightweight self-hosted media server.
package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
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

func main() {
	level := slog.LevelInfo
	if os.Getenv("COUCHSIDE_DEBUG") != "" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(logs.Handler(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))))

	if len(os.Args) > 1 && os.Args[1] == "reset-password" {
		if err := resetPassword(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "reset-password:", err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()
	for _, dir := range []string{cfg.DataDir, cfg.CacheDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	cfg.ServerID = serverID(cfg.DataDir)
	database, err := db.Open(filepath.Join(cfg.DataDir, "couchside.db"))
	if err != nil {
		return err
	}
	defer database.Close()

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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	enc := transcode.Detect(ctx, cfg.FFmpeg, cfg.HWAccel, cfg.VAAPIDevice)
	tc, err := transcode.NewManager(enc, cfg.FFprobe, filepath.Join(cfg.CacheDir, "transcode"), cfg.MaxTranscodes)
	if err != nil {
		return err
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
		return err
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
		return err
	}
	apiServer.UseLogs(logs)
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
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
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
		return err
	}
	// ListenAndServe returns as soon as Shutdown starts; requests still
	// draining need the database, which closes when run returns.
	<-drained
	<-workerDone
	<-tcDone
	<-tvDone
	return nil
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
	if err := os.WriteFile(p, []byte(id+"\n"), 0o644); err != nil {
		slog.Warn("couldn't save the server id", "err", err)
	}
	return id
}
