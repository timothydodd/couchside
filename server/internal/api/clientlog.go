package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/timothydodd/couchside/internal/db"
)

// Client reports: a TV app has no console anyone can read, so when playback
// fails it tells the server what its player said, and the report goes to the
// server log (System → Console and the diagnostics zip) beside the server's
// own lines about the same stream.

const (
	reportBody    = 16 << 10 // bytes
	reportKeys    = 40       // details kept per report
	reportValue   = 300      // characters kept per detail
	reportPerMin  = 20       // reports per client per minute; more are dropped
	reportMessage = 500
)

type clientReport struct {
	Level   string         `json:"level"` // error | warn | info
	Event   string         `json:"event"` // what happened, e.g. "playback_error"
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

// reportLimiter lets each client send reportPerMin reports a minute, so a
// player stuck retrying can't flood the log.
type reportLimiter struct {
	mu   sync.Mutex
	seen map[string][]time.Time
}

func (l *reportLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.seen == nil || len(l.seen) > presencePruneAt {
		l.seen = map[string][]time.Time{}
	}
	keep := l.seen[key][:0]
	for _, t := range l.seen[key] {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	if len(keep) >= reportPerMin {
		l.seen[key] = keep
		return false
	}
	l.seen[key] = append(keep, now)
	return true
}

func (s *Server) clientLog(w http.ResponseWriter, r *http.Request) {
	var in clientReport
	r.Body = http.MaxBytesReader(w, r.Body, reportBody)
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if !s.reports.allow(clientKey(r), time.Now()) {
		w.WriteHeader(http.StatusNoContent) // dropped; the client has nothing to do about it
		return
	}
	level := slog.LevelInfo
	switch in.Level {
	case "error":
		level = slog.LevelError
	case "warn":
		level = slog.LevelWarn
	}
	attrs := []any{
		"event", clip(oneLine(in.Event), 60),
		"message", clip(oneLine(in.Message), reportMessage),
		"profile", db.ProfileID(r.Context()),
		"device", deviceName(r.UserAgent()),
		"ua", clip(r.UserAgent(), 120),
		"ip", clientIP(r),
	}
	keys := make([]string, 0, len(in.Details))
	for k := range in.Details {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, k := range keys {
		if i == reportKeys {
			attrs = append(attrs, "dropped", len(keys)-reportKeys)
			break
		}
		attrs = append(attrs, clip(oneLine(k), 40), clip(oneLine(detailText(in.Details[k])), reportValue))
	}
	slog.Log(r.Context(), level, "client report", attrs...)
	w.WriteHeader(http.StatusNoContent)
}

// detailText is a detail's value as text: nested objects (a Roku player's
// errorInfo) are flattened to "k=v k=v".
func detailText(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return fmt.Sprint(t)
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, k+"="+detailText(t[k]))
		}
		return strings.Join(parts, " ")
	case []any:
		parts := make([]string, 0, len(t))
		for _, x := range t {
			parts = append(parts, detailText(x))
		}
		return strings.Join(parts, ",")
	}
	return fmt.Sprint(v)
}

// oneLine keeps a client's text to one line of the log.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
