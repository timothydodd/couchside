package config

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Setting is an environment variable admins can also set in Settings →
// Server. A saved value wins over the variable and takes effect when the
// server restarts (everything here is read at start-up).
type Setting struct {
	Key         string   `json:"key"` // the environment variable
	Group       string   `json:"group"`
	Label       string   `json:"label"`
	Help        string   `json:"help"`
	Kind        string   `json:"kind"` // text | dir | file | int | duration | choice | bool | secret
	Options     []string `json:"options,omitempty"`
	Default     string   `json:"default"` // what an unset value means, in words or as a value
	Placeholder string   `json:"placeholder,omitempty"`
}

// Editable lists the settings in Settings → Server, in display order. Left
// out on purpose: the port and data/cache folders (needed before the
// database opens), COUCHSIDE_AUTH (a lock admins mustn't lift from the web),
// the DVR folder and padding (already in Settings → Live TV / Advanced), and
// COUCHSIDE_MEDIA_ROOT (media locations have their own list).
var Editable = func() []Setting {
	s := []Setting{
		{Key: "COUCHSIDE_SCAN_INTERVAL", Group: "Libraries", Label: "Rescan libraries every", Kind: "duration",
			Help: "How often every library is checked for new and removed files. 0 turns it off; you can still scan by hand.", Default: "6h", Placeholder: "6h"},
		{Key: "COUCHSIDE_WORKERS", Group: "Libraries", Label: "Background jobs at once", Kind: "int",
			Help: "Scans, matching, artwork and thumbnails running side by side.", Default: "2"},

		{Key: "COUCHSIDE_HWACCEL", Group: "Encoding", Label: "Hardware encoder", Kind: "choice",
			Options: []string{"auto", "none", "nvenc", "qsv", "vaapi"},
			Help:    "auto tries the GPUs this machine might have and uses the first that works. nvenc is NVIDIA, qsv Intel Quick Sync, vaapi Intel/AMD on Linux, none the CPU. A GPU that fails its test falls back to the CPU.",
			Default: "auto"},
		{Key: "COUCHSIDE_MAX_TRANSCODES", Group: "Encoding", Label: "Streams converted at once", Kind: "int",
			Help: "Playback and live TV sessions that need converting. Direct play and remuxes don't count. More than your CPU or GPU can keep up with makes every stream stutter.", Default: "2"},
		{Key: "COUCHSIDE_ENCODE_WORKERS", Group: "Encoding", Label: "Optimized copies at once", Kind: "int",
			Help: "Background \"optimize\" and commercial-detection jobs.", Default: "1"},
		{Key: "COUCHSIDE_OPTIMIZE_HEIGHT", Group: "Encoding", Label: "Optimized copy size", Kind: "choice",
			Options: []string{"480", "720", "1080"}, Help: "The tallest picture an optimized copy keeps.", Default: "1080"},
		{Key: "COUCHSIDE_FFMPEG", Group: "Encoding", Label: "ffmpeg", Kind: "file",
			Help: "Leave empty for the ffmpeg beside Couchside, else the one on the PATH.", Default: "Bundled or on the PATH"},
		{Key: "COUCHSIDE_FFPROBE", Group: "Encoding", Label: "ffprobe", Kind: "file",
			Help: "Leave empty for the ffprobe beside Couchside, else the one on the PATH.", Default: "Bundled or on the PATH"},

		{Key: "COUCHSIDE_HDHOMERUN", Group: "Live TV", Label: "HDHomeRun tuner", Kind: "text",
			Help: "The tuner's IP address or host name. Empty turns off broadcast TV and recording (your own channels still work).", Default: "None", Placeholder: "192.168.1.50"},
		{Key: "COUCHSIDE_COMSKIP", Group: "Live TV", Label: "comskip", Kind: "file",
			Help: "Commercial detection for recordings. Off when it can't be found.", Default: "comskip on the PATH"},
		{Key: "COUCHSIDE_COMSKIP_INI", Group: "Live TV", Label: "comskip.ini", Kind: "file",
			Help: "Your own comskip settings. Empty uses Couchside's, which find the same breaks as Plex.", Default: "Couchside's own"},

		{Key: "TMDB_API_KEY", Group: "Metadata", Label: "TMDB API key", Kind: "secret",
			Help: "Your own key from themoviedb.org. Empty uses this build's key; \"off\" turns TMDB off.", Default: "This build's key"},
		{Key: "OMDB_API_KEY", Group: "Metadata", Label: "OMDb API key", Kind: "secret",
			Help: "Optional fallback for titles TMDB doesn't have (omdbapi.com).", Default: "None"},

		{Key: "COUCHSIDE_SERVER_NAME", Group: "Network", Label: "Server name", Kind: "text",
			Help: "Shown in TV apps' server lists.", Default: "This computer's name"},
		{Key: "COUCHSIDE_DISCOVERY", Group: "Network", Label: "Let TV apps find this server", Kind: "bool",
			Help: "Answers TV apps looking for servers on your network (SSDP).", Default: "true"},
		{Key: "COUCHSIDE_DISCOVERY_URL", Group: "Network", Label: "Address to advertise", Kind: "text",
			Help: "Only when TV apps reach the server on another address or port than it listens on (Docker -p 8095:8080).", Default: "Worked out automatically", Placeholder: "http://192.168.1.20:8095"},
		{Key: "COUCHSIDE_DISCOVERY_INTERFACE", Group: "Network", Label: "Discovery network interface", Kind: "text",
			Help: "The network interface to listen on for TV apps.", Default: "The default one", Placeholder: "eth0"},
		{Key: "COUCHSIDE_TRUSTED_PROXIES", Group: "Network", Label: "Trusted reverse proxies", Kind: "text",
			Help: "Addresses or CIDRs of proxies whose X-Forwarded-For and X-Forwarded-Proto are believed, separated by commas.", Default: "None", Placeholder: "10.0.0.0/8, 192.168.1.2"},
	}
	if runtime.GOOS == "linux" {
		// After the hardware encoder: only VAAPI uses it.
		for i, x := range s {
			if x.Key == "COUCHSIDE_HWACCEL" {
				s = append(s[:i+1], append([]Setting{{Key: "COUCHSIDE_VAAPI_DEVICE", Group: "Encoding", Label: "VAAPI device", Kind: "text",
					Help: "The GPU's render node, for vaapi.", Default: "/dev/dri/renderD128"}}, s[i+1:]...)...)
				break
			}
		}
	}
	return s
}()

// EditableSetting finds key in Editable.
func EditableSetting(key string) (Setting, bool) {
	for _, s := range Editable {
		if s.Key == key {
			return s, true
		}
	}
	return Setting{}, false
}

// Env is key's value from the environment or the settings file, ignoring
// Settings → Server, so the page can say what a reset goes back to.
func Env(key string) string { return os.Getenv(key) }

// Check cleans a value for s and says what's wrong with it. An empty value
// is always fine: it means "not set" (the default).
func (s Setting) Check(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil
	}
	switch s.Kind {
	case "int":
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 64 {
			return "", fmt.Errorf("%s: a whole number from 1 to 64", s.Label)
		}
	case "duration":
		if v == "0" {
			return v, nil
		}
		d, err := time.ParseDuration(v)
		if err != nil || d < 0 {
			return "", fmt.Errorf("%s: a length like 30m, 6h or 0", s.Label)
		}
		if d > 0 && d < time.Minute {
			return "", fmt.Errorf("%s: at least a minute", s.Label)
		}
	case "choice":
		for _, o := range s.Options {
			if v == o {
				return v, nil
			}
		}
		return "", fmt.Errorf("%s: one of %s", s.Label, strings.Join(s.Options, ", "))
	case "bool":
		switch strings.ToLower(v) {
		case "true", "1", "yes", "on":
			return "true", nil
		case "false", "0", "no", "off":
			return "false", nil
		}
		return "", fmt.Errorf("%s: true or false", s.Label)
	case "dir":
		if !filepath.IsAbs(v) {
			return "", fmt.Errorf("%s: a full folder path", s.Label)
		}
		v = filepath.Clean(v)
		if st, err := os.Stat(v); err != nil || !st.IsDir() {
			return "", fmt.Errorf("%s: the server can't open %s", s.Label, v)
		}
	case "file":
		// A bare name is looked up on the PATH, as the variable is.
		if _, err := exec.LookPath(v); err != nil {
			return "", fmt.Errorf("%s: the server can't find %s", s.Label, v)
		}
	}
	if s.Key == "COUCHSIDE_TRUSTED_PROXIES" {
		for _, p := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' }) {
			if _, _, err := net.ParseCIDR(p); err != nil && net.ParseIP(p) == nil {
				return "", fmt.Errorf("%s: %q isn't an address or CIDR", s.Label, p)
			}
		}
	}
	return v, nil
}
