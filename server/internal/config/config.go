// Package config reads Couchside's settings from the environment.
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Config struct {
	Addr         string        // COUCHSIDE_ADDR, default :8080
	DataDir      string        // COUCHSIDE_DATA_DIR: SQLite database lives here
	CacheDir     string        // COUCHSIDE_CACHE_DIR: artwork, stills (and later HLS segments)
	WebDir       string        // COUCHSIDE_WEB_DIR: built frontend to serve; empty = API only
	MediaRoot    string        // COUCHSIDE_MEDIA_ROOT: libraries must live under it; enables folder browsing
	OMDbKey      string        // OMDB_API_KEY: empty disables metadata matching
	Workers      int           // COUCHSIDE_WORKERS: concurrent background jobs
	ScanInterval time.Duration // COUCHSIDE_SCAN_INTERVAL: periodic rescan, 0 disables
	FFmpeg       string        // COUCHSIDE_FFMPEG
	FFprobe      string        // COUCHSIDE_FFPROBE

	HWAccel        string // COUCHSIDE_HWACCEL: none | vaapi | qsv | nvenc (falls back to none if unusable)
	VAAPIDevice    string // COUCHSIDE_VAAPI_DEVICE, default /dev/dri/renderD128
	MaxTranscodes  int    // COUCHSIDE_MAX_TRANSCODES: concurrent live transcode sessions
	EncodeWorkers  int    // COUCHSIDE_ENCODE_WORKERS: concurrent background "optimize" encodes
	OptimizeHeight int    // COUCHSIDE_OPTIMIZE_HEIGHT: max height of optimized copies

	HDHomeRun     string        // COUCHSIDE_HDHOMERUN: tuner IP/host; empty disables Live TV and DVR
	RecordingsDir string        // COUCHSIDE_RECORDINGS_DIR: where the DVR writes (must be writable)
	PadBefore     time.Duration // COUCHSIDE_DVR_PAD_BEFORE, default 10s (Settings overrides)
	PadAfter      time.Duration // COUCHSIDE_DVR_PAD_AFTER, default 10s (Settings overrides)

	Comskip    string // COUCHSIDE_COMSKIP: comskip binary; commercial detection is off when it isn't found
	ComskipINI string // COUCHSIDE_COMSKIP_INI: your own comskip.ini; empty uses Couchside's defaults
}

func Load() Config {
	data := env("COUCHSIDE_DATA_DIR", "./data")
	c := Config{
		Addr:         env("COUCHSIDE_ADDR", ":8080"),
		DataDir:      data,
		CacheDir:     env("COUCHSIDE_CACHE_DIR", filepath.Join(data, "cache")),
		WebDir:       os.Getenv("COUCHSIDE_WEB_DIR"),
		MediaRoot:    os.Getenv("COUCHSIDE_MEDIA_ROOT"),
		OMDbKey:      os.Getenv("OMDB_API_KEY"),
		Workers:      2,
		ScanInterval: 6 * time.Hour,
		FFmpeg:       env("COUCHSIDE_FFMPEG", "ffmpeg"),
		FFprobe:      env("COUCHSIDE_FFPROBE", "ffprobe"),

		HWAccel:        env("COUCHSIDE_HWACCEL", "none"),
		VAAPIDevice:    env("COUCHSIDE_VAAPI_DEVICE", "/dev/dri/renderD128"),
		MaxTranscodes:  envInt("COUCHSIDE_MAX_TRANSCODES", 2),
		EncodeWorkers:  envInt("COUCHSIDE_ENCODE_WORKERS", 1),
		OptimizeHeight: envInt("COUCHSIDE_OPTIMIZE_HEIGHT", 1080),

		HDHomeRun:     os.Getenv("COUCHSIDE_HDHOMERUN"),
		RecordingsDir: filepath.Clean(env("COUCHSIDE_RECORDINGS_DIR", filepath.Join(data, "recordings"))),
		PadBefore:     envDur("COUCHSIDE_DVR_PAD_BEFORE", 10*time.Second),
		PadAfter:      envDur("COUCHSIDE_DVR_PAD_AFTER", 10*time.Second),

		Comskip:    env("COUCHSIDE_COMSKIP", "comskip"),
		ComskipINI: os.Getenv("COUCHSIDE_COMSKIP_INI"),
	}
	if n, err := strconv.Atoi(os.Getenv("COUCHSIDE_WORKERS")); err == nil && n > 0 {
		c.Workers = n
	}
	if v := os.Getenv("COUCHSIDE_SCAN_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.ScanInterval = d
		}
	}
	if c.MediaRoot != "" {
		c.MediaRoot = filepath.Clean(c.MediaRoot)
	}
	return c
}

func envDur(key string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(key)); err == nil && d >= 0 {
		return d
	}
	return def
}

func envInt(key string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil && n > 0 {
		return n
	}
	return def
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
