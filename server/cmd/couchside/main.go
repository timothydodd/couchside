// Couchside: a lightweight self-hosted media server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/timothydodd/couchside/internal/api"
	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/livetv"
	"github.com/timothydodd/couchside/internal/metadata"
	"github.com/timothydodd/couchside/internal/transcode"
	"github.com/timothydodd/couchside/internal/webui"
	"github.com/timothydodd/couchside/internal/worker"
)

// version is stamped at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	level := slog.LevelInfo
	if os.Getenv("COUCHSIDE_DEBUG") != "" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

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

	var tv *livetv.Service
	tvDone := make(chan struct{})
	if cfg.HDHomeRun != "" {
		tv, err = livetv.New(livetv.Config{Tuner: cfg.HDHomeRun, RecordingsDir: cfg.RecordingsDir, FFmpeg: cfg.FFmpeg,
			PadBefore: cfg.PadBefore, PadAfter: cfg.PadAfter, Metadata: providers}, database, enc, w, cfg.CacheDir)
		if err != nil {
			return err
		}
		slog.Info("live tv enabled", "tuner", cfg.HDHomeRun, "recordings", cfg.RecordingsDir)
		go func() {
			tv.Run(ctx)
			close(tvDone)
		}()
	} else {
		close(tvDone)
	}

	apiServer, err := api.New(database, cfg, w, providers, tc, tv, version)
	if err != nil {
		return err
	}
	go apiServer.Run(ctx)
	if cfg.Auth {
		slog.Info("accounts are on: every profile signs in with a password")
	}
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           apiServer.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
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
	<-workerDone
	<-tcDone
	<-tvDone
	return nil
}
