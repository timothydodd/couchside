package api

import (
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/timothydodd/couchside/internal/db"
)

// presence remembers who has the app open and what they're playing, for the
// Settings page. A client is a profile on one browser (IP + user agent). The
// UI polls /api/status at least every 30s, so silence means the tab closed.
type presence struct {
	mu      sync.Mutex
	clients map[string]*client
}

type client struct {
	profileID int64
	ip        string
	device    string
	first     time.Time
	seen      time.Time
	play      *playing
}

// playing is what a client is watching. Movies and episodes report progress
// every 10s while playing; live TV and recordings poll their playlist every
// few seconds, which refreshes at.
type playing struct {
	Kind     string  `json:"kind"` // file | live | recording
	FileID   int64   `json:"fileId,omitempty"`
	Title    string  `json:"title"`
	Subtitle string  `json:"subtitle"`
	Position float64 `json:"positionSec"`
	Duration float64 `json:"durationSec"`
	Mode     string  `json:"mode"` // how it's being delivered, e.g. "Direct play", "Transcoding 720p"
	Paused   bool    `json:"paused"`
	at       time.Time
}

const (
	connectedFor = 75 * time.Second // longer than the UI's slowest status poll
	playingFor   = 35 * time.Second // progress reports come every 10s
	liveFor      = 20 * time.Second // live playlists are polled every 2-6s
	forgetAfter  = 10 * time.Minute
)

func newPresence() *presence { return &presence{clients: map[string]*client{}} }

func clientIP(r *http.Request) string {
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return h
	}
	return r.RemoteAddr
}

func clientKey(r *http.Request) string {
	return strconv.FormatInt(db.ProfileID(r.Context()), 10) + "|" + clientIP(r) + "|" + r.UserAgent()
}

// track marks the requesting client as connected.
func (p *presence) track(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.touch(r)
		next.ServeHTTP(w, r)
	})
}

func (p *presence) touch(r *http.Request) *client {
	now := time.Now()
	p.mu.Lock()
	defer p.mu.Unlock()
	k := clientKey(r)
	c := p.clients[k]
	if c == nil || now.Sub(c.seen) > forgetAfter {
		c = &client{profileID: db.ProfileID(r.Context()), ip: clientIP(r), device: deviceName(r.UserAgent()), first: now}
		p.clients[k] = c
	}
	c.seen = now
	return c
}

// setPlaying records what the client is watching; nil means it stopped.
func (p *presence) setPlaying(r *http.Request, pl *playing) {
	c := p.touch(r)
	p.mu.Lock()
	defer p.mu.Unlock()
	if pl != nil {
		pl.at = time.Now()
	}
	c.play = pl
}

// keepPlaying refreshes a live or recording stream the client is polling.
func (p *presence) keepPlaying(r *http.Request) {
	c := p.touch(r)
	p.mu.Lock()
	defer p.mu.Unlock()
	if c.play != nil && c.play.Kind != "file" {
		c.play.at = time.Now()
	}
}

// clientView is a connected client as the Settings page shows it.
type clientView struct {
	ProfileID    int64    `json:"profileId"`
	Device       string   `json:"device"`
	IP           string   `json:"ip"`
	ConnectedSec int64    `json:"connectedSec"`
	You          bool     `json:"you"`
	Playing      *playing `json:"playing"`
}

// connected lists connected clients, people watching something first.
func (p *presence) connected(r *http.Request) []clientView {
	now := time.Now()
	me := clientKey(r)
	p.mu.Lock()
	defer p.mu.Unlock()
	out := []clientView{}
	for k, c := range p.clients {
		if now.Sub(c.seen) > forgetAfter {
			delete(p.clients, k)
			continue
		}
		if now.Sub(c.seen) > connectedFor {
			continue
		}
		v := clientView{ProfileID: c.profileID, Device: c.device, IP: c.ip, ConnectedSec: int64(now.Sub(c.first).Seconds()), You: k == me}
		if pl := c.play; pl != nil {
			age := now.Sub(pl.at)
			switch {
			case pl.Kind != "file" && age < liveFor, pl.Kind == "file" && (age < playingFor || pl.Paused):
				cp := *pl
				if pl.Kind == "file" && !pl.Paused {
					cp.Position = min(pl.Position+age.Seconds(), max(pl.Duration, pl.Position))
				}
				v.Playing = &cp
			}
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if (out[i].Playing != nil) != (out[j].Playing != nil) {
			return out[i].Playing != nil
		}
		return out[i].ConnectedSec > out[j].ConnectedSec
	})
	return out
}

// deviceName turns a user agent into "Chrome on Windows".
func deviceName(ua string) string {
	has := func(s string) bool { return strings.Contains(ua, s) }
	browser := "Browser"
	switch {
	case has("Edg/"):
		browser = "Edge"
	case has("OPR/"):
		browser = "Opera"
	case has("Firefox/"):
		browser = "Firefox"
	case has("Chrome/"), has("CriOS/"):
		browser = "Chrome"
	case has("Safari/"):
		browser = "Safari"
	case ua != "" && !has("Mozilla/"):
		browser, _, _ = strings.Cut(ua, "/") // curl, apps
	}
	os := ""
	switch {
	case has("Tizen"), has("Web0S"), has("SmartTV"), has("SMART-TV"), has("AFT"):
		os = "TV"
	case has("iPad"):
		os = "iPad"
	case has("iPhone"):
		os = "iPhone"
	case has("Android"):
		os = "Android"
	case has("Windows"):
		os = "Windows"
	case has("CrOS"):
		os = "ChromeOS"
	case has("Mac OS X"), has("Macintosh"):
		os = "Mac"
	case has("Linux"):
		os = "Linux"
	}
	if os == "" {
		return browser
	}
	return browser + " on " + os
}
