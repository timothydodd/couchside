// Package remoteimg decides which internet images Couchside fetches, and
// how: only from the metadata providers' and SiliconDust's image hosts, over
// redirects that stay on them, and only bytes that really are an image.
package remoteimg

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Host is an image host Couchside fetches from. A name starting with "."
// matches its subdomains.
type Host struct {
	Name string
	// KeepQuery: the query picks the image, so it's part of the cache key.
	// Elsewhere it's dropped, so ?x=1, ?x=2… can't fill the cache with copies.
	KeepQuery bool
	// HTTP allows plain http (the HDHomeRun guide's URLs aren't confirmed to
	// all be https).
	HTTP bool
}

// Hosts is the allowlist. Tests may replace it.
var Hosts = []Host{
	{Name: "image.tmdb.org"},
	{Name: ".media-amazon.com"},
	{Name: ".media-imdb.com"},
	{Name: ".hdhomerun.com", KeepQuery: true, HTTP: true},
	{Name: ".silicondust.com", KeepQuery: true, HTTP: true},
}

func hostFor(u *url.URL) (Host, bool) {
	if u == nil || u.User != nil {
		return Host{}, false
	}
	name := strings.ToLower(u.Hostname())
	for _, h := range Hosts {
		if name == strings.TrimPrefix(h.Name, ".") || (strings.HasPrefix(h.Name, ".") && strings.HasSuffix(name, h.Name)) {
			return h, true
		}
	}
	return Host{}, false
}

// Allowed reports whether u is an image Couchside may fetch.
func Allowed(u *url.URL) bool {
	h, ok := hostFor(u)
	if !ok {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return true
	case "http":
		return h.HTTP
	}
	return false
}

// Key is u in canonical form, for caching and fetching: lower-case scheme and host, no
// fragment, and no query unless the host needs it.
func Key(u *url.URL) string {
	c := url.URL{Scheme: strings.ToLower(u.Scheme), Host: strings.ToLower(u.Host), Path: u.Path, RawPath: u.RawPath}
	if h, _ := hostFor(u); h.KeepQuery {
		c.RawQuery = u.RawQuery
	}
	return c.String()
}

// ErrRedirect is returned when a redirect leaves the allowlist or loops.
var ErrRedirect = errors.New("redirect to a host Couchside doesn't fetch from")

// NewClient is an HTTP client whose redirects must stay on allowed hosts,
// three hops at most.
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || !Allowed(req.URL) {
			return ErrRedirect
		}
		return nil
	}}
}

// Sniff returns the image type of b (its first bytes), or "" for anything
// that isn't a JPEG, PNG, WebP or GIF. SVG is refused: it can carry script.
func Sniff(b []byte) string {
	switch t := http.DetectContentType(b); t {
	case "image/jpeg", "image/png", "image/webp", "image/gif":
		return t
	}
	return ""
}
