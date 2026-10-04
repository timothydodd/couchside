package api

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/timothydodd/couchside/internal/auth"
)

// Signing a TV in from a browser. The TV asks for a code and shows it ("go
// to couchside.example/link and enter ABCD-EFGH"); someone already signed in
// on a phone or computer enters it there; the TV, which has been asking,
// gets a session for that person's profile. No typing a password with a
// remote, and it works for accounts that sign in through a provider or with
// a second factor, since the browser did that part.
const (
	deviceCodeFor  = 10 * time.Minute
	devicePollSec  = 3
	deviceMaxCodes = 1000
)

type deviceCode struct {
	secretHash string // of the code the TV polls with
	device     string
	expires    time.Time
	profile    int64 // who approved it; 0 = nobody yet
}

type deviceState struct {
	mu    sync.Mutex
	codes map[string]*deviceCode // by the code a person types
}

// deviceStart gives a TV a code to show and a secret to ask with.
func (s *Server) deviceStart(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Device string `json:"device"`
	}
	if r.ContentLength != 0 {
		if err := decode(r, &in); err != nil {
			writeErr(w, err)
			return
		}
	}
	ip := clientIP(r)
	if wait := s.auth.picks.Wait(ip); wait > 0 {
		retryLater(w, wait)
		return
	}
	s.auth.picks.Fail(ip)                                 // every code counts, like every passwordless pick
	secret, user := auth.NewToken(), auth.SetupCode()[:9] // "ABCD-EFGH"
	now := time.Now()
	s.device.mu.Lock()
	if s.device.codes == nil {
		s.device.codes = map[string]*deviceCode{}
	}
	for k, c := range s.device.codes {
		if now.After(c.expires) {
			delete(s.device.codes, k)
		}
	}
	full := len(s.device.codes) >= deviceMaxCodes
	if !full {
		s.device.codes[user] = &deviceCode{secretHash: auth.HashToken(secret), device: clip(in.Device, 60), expires: now.Add(deviceCodeFor)}
	}
	s.device.mu.Unlock()
	if full {
		writeErr(w, httpError{http.StatusTooManyRequests, "too many devices are signing in; try again in a few minutes"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deviceCode": secret, "userCode": user, "verifyPath": "/link",
		"expiresIn": int(deviceCodeFor / time.Second), "interval": devicePollSec})
}

// deviceToken is the TV asking whether its code has been entered yet.
func (s *Server) deviceToken(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DeviceCode string `json:"deviceCode"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	hash := auth.HashToken(in.DeviceCode)
	s.device.mu.Lock()
	var found *deviceCode
	for k, c := range s.device.codes {
		if c.secretHash != hash {
			continue
		}
		if time.Now().After(c.expires) {
			delete(s.device.codes, k)
			break
		}
		found = c
		if c.profile != 0 {
			delete(s.device.codes, k) // one session per code
		}
	}
	s.device.mu.Unlock()
	if found == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "that code has expired; start again", "code": "expired"})
		return
	}
	if found.profile == 0 {
		writeJSON(w, http.StatusPreconditionRequired, map[string]string{"error": "waiting for the code to be entered", "code": "authorization_pending"})
		return
	}
	p, err := s.db.Profile(r.Context(), found.profile)
	if err != nil || p.Disabled {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "that account can't sign in", "code": "expired"})
		return
	}
	s.startSession(w, r, p, "tv", found.device)
}

// deviceApprove is a signed-in person entering the code their TV shows: the
// TV is signed in as them.
func (s *Server) deviceApprove(w http.ResponseWriter, r *http.Request) {
	var in struct {
		UserCode string `json:"userCode"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	u := currentUser(r.Context())
	ip := clientIP(r)
	if wait := s.auth.byIP.Wait(ip); wait > 0 {
		retryLater(w, wait)
		return
	}
	code := normalizeUserCode(in.UserCode)
	s.device.mu.Lock()
	c := s.device.codes[code]
	ok := c != nil && time.Now().Before(c.expires) && c.profile == 0
	device := ""
	if ok {
		c.profile, device = u.ID, c.device
	}
	s.device.mu.Unlock()
	if !ok {
		s.auth.byIP.Fail(ip) // guessing codes is guessing a way in
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "that code isn't right, or it has expired; check the TV"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"device": device})
}

// normalizeUserCode accepts the code as people type it: any case, with or
// without the dash or spaces.
func normalizeUserCode(c string) string {
	c = strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(c)))
	if len(c) != 8 {
		return ""
	}
	return c[:4] + "-" + c[4:]
}
