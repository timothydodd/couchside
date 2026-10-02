package remoteimg

import (
	"net/url"
	"testing"
)

func TestAllowed(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://image.tmdb.org/t/p/w185/a.jpg":      true,
		"https://m.media-amazon.com/images/M/x.jpg":  true,
		"https://img.hdhomerun.com/channels/US1.png": true,
		"http://img.hdhomerun.com/channels/US1.png":  true,
		"http://image.tmdb.org/t/p/w185/a.jpg":       false,
		"https://evil.com/x.png":                     false,
		"https://image.tmdb.org.evil.com/x.png":      false,
		"https://notmedia-amazon.com/x.png":          false,
		"https://user:pw@image.tmdb.org/a.jpg":       false,
		"file:///etc/passwd":                         false,
		"http://169.254.169.254/latest/meta-data/":   false,
	} {
		u, _ := url.Parse(raw)
		if got := Allowed(u); got != want {
			t.Errorf("Allowed(%s) = %v, want %v", raw, got, want)
		}
	}
}

func TestKey(t *testing.T) {
	for raw, want := range map[string]string{
		"https://IMAGE.tmdb.org/t/p/w185/a.jpg?x=1#frag":   "https://image.tmdb.org/t/p/w185/a.jpg",
		"HTTPS://image.tmdb.org/t/p/w185/a.jpg":            "https://image.tmdb.org/t/p/w185/a.jpg",
		"https://img.hdhomerun.com/titles/C1.jpg?size=2#x": "https://img.hdhomerun.com/titles/C1.jpg?size=2",
	} {
		u, _ := url.Parse(raw)
		if got := Key(u); got != want {
			t.Errorf("Key(%s) = %s, want %s", raw, got, want)
		}
	}
}

func TestSniff(t *testing.T) {
	for name, c := range map[string]struct {
		body, want string
	}{
		"png":  {"\x89PNG\r\n\x1a\n0000", "image/png"},
		"jpeg": {"\xff\xd8\xff\xe0\x00\x10JFIF", "image/jpeg"},
		"gif":  {"GIF89a....", "image/gif"},
		"webp": {"RIFF\x00\x00\x00\x00WEBPVP8 ", "image/webp"},
		"svg":  {`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`, ""},
		"html": {"<!DOCTYPE html><html><script>alert(1)</script>", ""},
	} {
		if got := Sniff([]byte(c.body)); got != c.want {
			t.Errorf("%s: Sniff = %q, want %q", name, got, c.want)
		}
	}
}
