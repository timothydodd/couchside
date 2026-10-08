// Package config reads Couchside's settings from the environment, with
// the ones admins change in Settings → Server (see Editable) on top.
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr     string // COUCHSIDE_ADDR, default :8080
	DataDir  string // COUCHSIDE_DATA_DIR: SQLite database lives here
	CacheDir string // COUCHSIDE_CACHE_DIR: artwork, stills, subtitles, optimized copies, HLS segments
	WebDir   string // COUCHSIDE_WEB_DIR: built frontend to serve; empty = API only
	// COUCHSIDE_MEDIA_ROOT: media locations from the environment (containers),
	// one or more separated like PATH (";" on Windows, ":" elsewhere). They're
	// added to the ones saved in Settings → Server. MediaRootsFromFile are
	// the ones the settings file gave (the Windows installer used to write
	// it), imported once as saved locations instead.
	MediaRoots         []string
	MediaRootsFromFile []string
	OMDbKey            string        // OMDB_API_KEY: optional second metadata source
	TMDBKey            string        // TMDB_API_KEY: overrides the built-in key; "off" disables TMDB
	Workers            int           // COUCHSIDE_WORKERS: concurrent background jobs
	ScanInterval       time.Duration // COUCHSIDE_SCAN_INTERVAL: periodic rescan, 0 disables
	FFmpeg             string        // COUCHSIDE_FFMPEG, default ffmpeg beside the executable, else on the PATH
	FFprobe            string        // COUCHSIDE_FFPROBE, likewise
	EnvFile            string        // the settings file that was read (see envFilePath), or ""

	HWAccel        string // COUCHSIDE_HWACCEL: auto (default) | none | vaapi | qsv | nvenc (falls back to none if unusable)
	VAAPIDevice    string // COUCHSIDE_VAAPI_DEVICE, default /dev/dri/renderD128
	MaxTranscodes  int    // COUCHSIDE_MAX_TRANSCODES: concurrent live transcode sessions
	EncodeWorkers  int    // COUCHSIDE_ENCODE_WORKERS: concurrent background "optimize" encodes
	OptimizeHeight int    // COUCHSIDE_OPTIMIZE_HEIGHT: max height of optimized copies

	HDHomeRun     string        // COUCHSIDE_HDHOMERUN: tuner IP/host; empty disables Live TV and DVR
	RecordingsDir string        // COUCHSIDE_RECORDINGS_DIR: where the DVR writes (must be writable)
	PadBefore     time.Duration // COUCHSIDE_DVR_PAD_BEFORE, default 10s (Settings overrides)
	PadAfter      time.Duration // COUCHSIDE_DVR_PAD_AFTER, default 10s (Settings overrides)

	// COUCHSIDE_AUTH: require passwords. Accounts are always on; without this,
	// admins can allow passwordless sign-in (pick a profile, no password) in
	// Settings, which is the default on a server where nobody has a password.
	// Set it for a server reachable from the internet.
	Auth bool
	// COUCHSIDE_TRUSTED_PROXIES: CIDRs or addresses of reverse proxies whose
	// X-Forwarded-For and X-Forwarded-Proto are believed. Empty trusts nobody.
	TrustedProxies []string

	// LAN discovery (SSDP), so TV apps find the server without typing its address.
	Discovery          bool   // COUCHSIDE_DISCOVERY: answer SSDP searches (default on; "false" turns it off)
	DiscoveryURL       string // COUCHSIDE_DISCOVERY_URL: base URL to advertise, when the port others reach differs (Docker -p 8095:8080)
	DiscoveryInterface string // COUCHSIDE_DISCOVERY_INTERFACE: network interface to listen on; empty = the default one
	ServerName         string // COUCHSIDE_SERVER_NAME: shown in TV apps' server lists; default the host name
	ServerID           string // stable id for this server, kept in $DATA/server.id (set at start-up, not from the environment)

	Comskip    string // COUCHSIDE_COMSKIP: comskip binary; commercial detection is off when it isn't found
	ComskipINI string // COUCHSIDE_COMSKIP_INI: your own comskip.ini; empty uses Couchside's defaults
}

// Load reads the settings from the environment (and the settings file).
func Load() Config { return LoadWith(nil) }

// overrides are the values set in Settings → Server, which win over the
// environment for the keys in Editable. Set by LoadWith at start-up.
var overrides map[string]string

// getenv is the override for key when Settings has one (even an empty one,
// which means "not set"), else the environment variable.
func getenv(key string) string {
	if v, ok := overrides[key]; ok {
		return v
	}
	return os.Getenv(key)
}

// LoadWith is Load with the values saved in Settings → Server, keyed by
// environment variable name, taking the place of those variables.
func LoadWith(o map[string]string) Config {
	overrides = map[string]string{}
	for k, v := range o {
		if _, ok := EditableSetting(k); ok { // nothing else can be overridden
			overrides[k] = v
		}
	}
	envFile := loadEnvFile()
	data := env("COUCHSIDE_DATA_DIR", "./data")
	c := Config{
		Addr:         env("COUCHSIDE_ADDR", ":8080"),
		DataDir:      data,
		CacheDir:     env("COUCHSIDE_CACHE_DIR", filepath.Join(data, "cache")),
		WebDir:       getenv("COUCHSIDE_WEB_DIR"),
		OMDbKey:      getenv("OMDB_API_KEY"),
		TMDBKey:      tmdbKey(),
		Workers:      2,
		ScanInterval: 6 * time.Hour,
		FFmpeg:       env("COUCHSIDE_FFMPEG", besideExe("ffmpeg")),
		FFprobe:      env("COUCHSIDE_FFPROBE", besideExe("ffprobe")),
		EnvFile:      envFile,

		HWAccel:        env("COUCHSIDE_HWACCEL", "auto"),
		VAAPIDevice:    env("COUCHSIDE_VAAPI_DEVICE", "/dev/dri/renderD128"),
		MaxTranscodes:  envInt("COUCHSIDE_MAX_TRANSCODES", 2),
		EncodeWorkers:  envInt("COUCHSIDE_ENCODE_WORKERS", 1),
		OptimizeHeight: envInt("COUCHSIDE_OPTIMIZE_HEIGHT", 1080),

		HDHomeRun:     getenv("COUCHSIDE_HDHOMERUN"),
		RecordingsDir: filepath.Clean(env("COUCHSIDE_RECORDINGS_DIR", filepath.Join(data, "recordings"))),
		PadBefore:     envDur("COUCHSIDE_DVR_PAD_BEFORE", 10*time.Second),
		PadAfter:      envDur("COUCHSIDE_DVR_PAD_AFTER", 10*time.Second),

		Auth:           envBool("COUCHSIDE_AUTH"),
		TrustedProxies: strings.FieldsFunc(getenv("COUCHSIDE_TRUSTED_PROXIES"), func(r rune) bool { return r == ',' || r == ' ' }),

		Discovery:          !envFalse("COUCHSIDE_DISCOVERY"),
		DiscoveryURL:       getenv("COUCHSIDE_DISCOVERY_URL"),
		DiscoveryInterface: getenv("COUCHSIDE_DISCOVERY_INTERFACE"),
		ServerName:         serverName(),

		Comskip:    env("COUCHSIDE_COMSKIP", "comskip"),
		ComskipINI: getenv("COUCHSIDE_COMSKIP_INI"),
	}
	if n, err := strconv.Atoi(getenv("COUCHSIDE_WORKERS")); err == nil && n > 0 {
		c.Workers = n
	}
	if v := getenv("COUCHSIDE_SCAN_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.ScanInterval = d
		}
	}
	roots := splitPaths(getenv("COUCHSIDE_MEDIA_ROOT"))
	if FromFile("COUCHSIDE_MEDIA_ROOT") {
		c.MediaRootsFromFile = roots
	} else {
		c.MediaRoots = roots
	}
	return c
}

func envDur(key string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(getenv(key)); err == nil && d >= 0 {
		return d
	}
	return def
}

func envInt(key string, def int) int {
	if n, err := strconv.Atoi(getenv(key)); err == nil && n > 0 {
		return n
	}
	return def
}

func env(key, def string) string {
	if v := getenv(key); v != "" {
		return v
	}
	return def
}

// builtinTMDBKey is Couchside's own TMDB key, stamped into release builds
// (-ldflags "-X .../config.builtinTMDBKey=...") from a CI secret so it isn't
// in the repo. TMDB lets apps ship their key; each server's requests count
// against its own address. Empty in dev builds.
var builtinTMDBKey string

// TMDBKeySource says where the TMDB key in use comes from: "builtin" (this
// build's own), "custom" (TMDB_API_KEY) or "" (none: TMDB is off).
func (c Config) TMDBKeySource() string {
	switch {
	case c.TMDBKey == "":
		return ""
	case c.TMDBKey == builtinTMDBKey:
		return "builtin"
	default:
		return "custom"
	}
}

func tmdbKey() string {
	switch v := strings.TrimSpace(getenv("TMDB_API_KEY")); strings.ToLower(v) {
	case "":
		return builtinTMDBKey
	case "off", "none", "false", "0":
		return ""
	default:
		return v
	}
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func envFalse(key string) bool {
	switch strings.ToLower(strings.TrimSpace(getenv(key))) {
	case "0", "false", "no", "off":
		return true
	}
	return false
}

// serverName is COUCHSIDE_SERVER_NAME, else the host name, unless that's a
// container's random id (12 hex characters), which says nothing to a person.
func serverName() string {
	if v := strings.TrimSpace(getenv("COUCHSIDE_SERVER_NAME")); v != "" {
		return v
	}
	h, _ := os.Hostname()
	if h == "" || (len(h) == 12 && strings.Trim(h, "0123456789abcdef") == "") {
		return "Couchside"
	}
	return h
}

// splitPaths splits a PATH-style list into clean, non-empty paths.
func splitPaths(v string) []string {
	var out []string
	for _, p := range filepath.SplitList(v) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, filepath.Clean(p))
		}
	}
	return out
}
