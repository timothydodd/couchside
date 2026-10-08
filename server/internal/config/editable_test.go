package config

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLoadWithOverrides(t *testing.T) {
	t.Setenv("COUCHSIDE_MAX_TRANSCODES", "3")
	t.Setenv("COUCHSIDE_DATA_DIR", "/env/data")
	t.Setenv("COUCHSIDE_SCAN_INTERVAL", "")

	c := LoadWith(map[string]string{
		"COUCHSIDE_MAX_TRANSCODES": "5",
		"COUCHSIDE_SCAN_INTERVAL":  "0", // off
		"COUCHSIDE_DATA_DIR":       "/web/data",
		"COUCHSIDE_AUTH":           "false",
	})
	if c.MaxTranscodes != 5 || c.ScanInterval != 0 {
		t.Fatalf("overrides not applied: %+v", c)
	}
	if c.DataDir != "/env/data" {
		t.Fatalf("data dir = %q: only Editable settings can be overridden", c.DataDir)
	}

	// Without overrides the environment applies again (a restart after a reset).
	c = Load()
	if c.MaxTranscodes != 3 || c.ScanInterval != 6*time.Hour {
		t.Fatalf("environment after overrides: %+v", c)
	}
}

func TestCheck(t *testing.T) {
	get := func(k string) Setting {
		s, ok := EditableSetting(k)
		if !ok {
			t.Fatalf("%s isn't editable", k)
		}
		return s
	}
	for _, c := range []struct {
		key, in, want string
		ok            bool
	}{
		{"COUCHSIDE_WORKERS", " 4 ", "4", true},
		{"COUCHSIDE_WORKERS", "0", "", false},
		{"COUCHSIDE_WORKERS", "lots", "", false},
		{"COUCHSIDE_SCAN_INTERVAL", "0", "0", true},
		{"COUCHSIDE_SCAN_INTERVAL", "90m", "90m", true},
		{"COUCHSIDE_SCAN_INTERVAL", "5s", "", false},
		{"COUCHSIDE_HWACCEL", "nvenc", "nvenc", true},
		{"COUCHSIDE_HWACCEL", "cuda", "", false},
		{"COUCHSIDE_DISCOVERY", "off", "false", true},
		{"COUCHSIDE_COMSKIP_INI", "", "", true}, // empty: not set
		{"COUCHSIDE_TRUSTED_PROXIES", "10.0.0.0/8, 192.168.1.2", "10.0.0.0/8, 192.168.1.2", true},
		{"COUCHSIDE_TRUSTED_PROXIES", "10.0.0.0/8, proxy", "", false},
		{"COUCHSIDE_FFMPEG", "/no/such/ffmpeg", "", false},
	} {
		got, err := get(c.key).Check(c.in)
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("%s %q = %q, %v", c.key, c.in, got, err)
		}
	}
}

func TestMediaRoots(t *testing.T) {
	sep := string(filepath.ListSeparator)
	t.Setenv("COUCHSIDE_MEDIA_ROOT", "/srv/movies/"+sep+" "+sep+"/srv/tv")
	c := Load()
	if len(c.MediaRoots) != 2 || c.MediaRoots[0] != filepath.Clean("/srv/movies") || c.MediaRoots[1] != filepath.Clean("/srv/tv") || len(c.MediaRootsFromFile) != 0 {
		t.Fatalf("roots = %q, from file %q", c.MediaRoots, c.MediaRootsFromFile)
	}
	if _, ok := EditableSetting("COUCHSIDE_MEDIA_ROOT"); ok {
		t.Fatal("media locations have their own list, not a Settings → Server field")
	}
}
