package api

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/netshare"
)

// Media locations are where media can be: folders and drives on the server,
// and network shares (signed in to with a saved user name and password on
// Windows). Libraries, the DVR folder and the folder browser stay inside
// them. COUCHSIDE_MEDIA_ROOT adds fixed ones from the environment
// (containers); the rest are saved in Settings → Server or the first-run
// setup.

// settingLocationsInit marks the one-time carry-over (PrepareLocations).
const settingLocationsInit = "media.locations_init"

// PrepareLocations runs at start-up, before anything reads media. The first
// time, it carries over a media folder from the Windows installer's settings
// file, and on a server with no locations at all, the folders of its
// libraries, so upgrading restricts nothing that worked before. Then it
// signs in to the network shares, returning each one's error by path ("" when
// signed in).
func PrepareLocations(ctx context.Context, d *db.DB, cfg config.Config) map[string]string {
	if done, err := d.Setting(ctx, settingLocationsInit); err == nil && done == "" {
		for _, p := range cfg.MediaRootsFromFile {
			if err := d.AddMediaLocation(ctx, p, "", nil); err == nil {
				slog.Info("media location carried over from the settings file", "path", p)
			}
		}
		saved, _ := d.MediaLocations(ctx)
		if len(saved) == 0 && len(cfg.MediaRoots) == 0 {
			libs, _ := d.Libraries(ctx)
			paths := make([]string, 0, len(libs))
			for _, l := range libs {
				paths = append(paths, l.Path)
			}
			for _, p := range outermost(paths) {
				if err := d.AddMediaLocation(ctx, p, "", nil); err == nil {
					slog.Info("media location added for an existing library", "path", p)
				}
			}
		}
		_ = d.SetSetting(ctx, settingLocationsInit, "1")
	}
	status := map[string]string{}
	if !netshare.Supported {
		return status
	}
	saved, err := d.MediaLocations(ctx)
	if err != nil {
		slog.Warn("media locations", "err", err)
		return status
	}
	for p, e := range netshare.ConnectAll(shares(saved)) {
		status[p] = e
		if e != "" {
			slog.Warn("network share sign-in failed", "share", p, "err", e)
		} else {
			slog.Info("network share signed in", "share", p)
		}
	}
	return status
}

// shares are the saved locations with a sign-in.
func shares(locs []db.MediaLocation) []netshare.Share {
	var out []netshare.Share
	for _, l := range locs {
		if isShare(l.Path) && (l.User != "" || len(l.Secret) > 0) {
			out = append(out, netshare.Share{Path: l.Path, User: l.User, Secret: l.Secret})
		}
	}
	return out
}

func isShare(p string) bool { return strings.HasPrefix(p, `\\`) }

// outermost drops paths inside others in the list.
func outermost(paths []string) []string {
	sort.Slice(paths, func(i, j int) bool { return len(paths[i]) < len(paths[j]) })
	var out []string
	for _, p := range paths {
		if !within(p, out) {
			out = append(out, p)
		}
	}
	return out
}

// UseShares hands over how each share's sign-in went at start-up.
func (s *Server) UseShares(status map[string]string) {
	s.sharesMu.Lock()
	s.shareStatus = maps.Clone(status)
	s.sharesMu.Unlock()
}

func (s *Server) setShareStatus(path, e string) {
	s.sharesMu.Lock()
	if s.shareStatus == nil {
		s.shareStatus = map[string]string{}
	}
	s.shareStatus[path] = e
	s.sharesMu.Unlock()
}

// mediaRoots are every location: the environment's, then the saved ones.
func (s *Server) mediaRoots(ctx context.Context) []string {
	out := append([]string(nil), s.cfg.MediaRoots...)
	saved, err := s.db.MediaLocations(ctx)
	if err != nil {
		slog.Warn("media locations", "err", err)
	}
	for _, l := range saved {
		out = append(out, l.Path)
	}
	return out
}

// within says p is one of roots or inside one. insideDir compares by path
// element with the platform's separator, so D:\Media\Movies is inside D:\Media.
func within(p string, roots []string) bool {
	p = filepath.Clean(p)
	for _, r := range roots {
		if p == filepath.Clean(r) || insideDir(p, r) {
			return true
		}
	}
	return false
}

func errOutsideLocations(p string) error {
	return badRequest(p + " isn't in a media location: add its drive, folder or share in Settings → Server first")
}

// first is the first of a list, or "".
func first(list []string) string {
	if len(list) == 0 {
		return ""
	}
	return list[0]
}

type locationRow struct {
	ID          int64  `json:"id"` // 0 for the environment's
	Path        string `json:"path"`
	Share       bool   `json:"share"`
	FromEnv     bool   `json:"fromEnv"`
	User        string `json:"user"`
	HasPassword bool   `json:"hasPassword"`
	Found       bool   `json:"found"` // the folder can be opened now
	Error       string `json:"error"` // the share's sign-in failed
}

func (s *Server) listLocations(w http.ResponseWriter, r *http.Request) {
	rows, err := s.locationRows(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"locations": rows, "shares": netshare.Supported, "envVar": "COUCHSIDE_MEDIA_ROOT"})
}

func (s *Server) locationRows(ctx context.Context) ([]locationRow, error) {
	saved, err := s.db.MediaLocations(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]locationRow, 0, len(saved)+len(s.cfg.MediaRoots))
	for _, p := range s.cfg.MediaRoots {
		rows = append(rows, locationRow{Path: p, Share: isShare(p), FromEnv: true})
	}
	s.sharesMu.Lock()
	for _, l := range saved {
		rows = append(rows, locationRow{ID: l.ID, Path: l.Path, Share: isShare(l.Path), User: l.User,
			HasPassword: len(l.Secret) > 0, Error: s.shareStatus[l.Path]})
	}
	s.sharesMu.Unlock()
	// A NAS that's off can keep a stat waiting for many seconds: check them
	// side by side, and give up on any that take too long.
	var wg sync.WaitGroup
	for i := range rows {
		wg.Add(1)
		go func(row *locationRow) {
			defer wg.Done()
			st, err := statWithin(row.Path, 3*time.Second)
			row.Found = err == nil && st.IsDir()
		}(&rows[i])
	}
	wg.Wait()
	return rows, nil
}

type locationIn struct {
	Path     string  `json:"path"`
	User     string  `json:"user"`
	Password *string `json:"password"` // nil keeps the saved one (editing)
}

// addLocation checks the folder can be opened (signing in to a share first
// when a user name or password is given) and saves it.
func (s *Server) addLocation(w http.ResponseWriter, r *http.Request) {
	var in locationIn
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	ctx := r.Context()
	p := strings.TrimSpace(in.Path)
	if netshare.Supported {
		p = strings.ReplaceAll(p, "/", `\`)
	}
	if p == "" || !filepath.IsAbs(p) {
		writeErr(w, badRequest(`enter a full folder path, like D:\Media, /mnt/media or \\nas\media`))
		return
	}
	p = filepath.Clean(p)
	for _, root := range s.mediaRoots(ctx) {
		if p == filepath.Clean(root) {
			writeErr(w, badRequest(p+" is already a media location"))
			return
		}
		if insideDir(p, root) {
			writeErr(w, badRequest(p+" is already inside "+root))
			return
		}
	}
	user := strings.TrimSpace(in.User)
	var secret []byte
	login := user != "" || (in.Password != nil && *in.Password != "")
	if login {
		if !isShare(p) || !netshare.Supported {
			writeErr(w, badRequest(`a user name and password are only for a network share (\\nas\media) on Windows; elsewhere, mount the share on the host`))
			return
		}
		pw := ""
		if in.Password != nil {
			pw = *in.Password
		}
		if err := netshare.Connect(p, user, pw); err != nil {
			writeErr(w, badRequest(err.Error()))
			return
		}
		var err error
		if secret, err = netshare.Protect(pw); err != nil {
			writeErr(w, err)
			return
		}
	}
	if st, err := statWithin(p, 10*time.Second); err != nil || !st.IsDir() {
		msg := "the server can't open " + p
		if isShare(p) && !login && netshare.Supported {
			msg += ": if the NAS asks for a user name and password, enter them too"
		}
		writeErr(w, badRequest(msg))
		return
	}
	if err := s.db.AddMediaLocation(ctx, p, user, secret); err != nil {
		writeErr(w, err)
		return
	}
	if login {
		s.setShareStatus(p, "")
	}
	slog.Info("media location added", "path", p, "signedIn", login)
	s.listLocations(w, r)
}

// setLocationLogin changes a share's user name or password, signing in with
// them first.
func (s *Server) setLocationLogin(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in locationIn
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	ctx := r.Context()
	l, err := s.db.MediaLocation(ctx, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !isShare(l.Path) || !netshare.Supported {
		writeErr(w, badRequest("only a network share on Windows has a sign-in"))
		return
	}
	user := strings.TrimSpace(in.User)
	pw, secret := "", []byte(nil)
	if in.Password != nil {
		pw = *in.Password
		if secret, err = netshare.Protect(pw); err != nil {
			writeErr(w, err)
			return
		}
		if secret == nil {
			secret = []byte{} // no password, rather than the saved one
		}
	} else if pw, err = netshare.Unprotect(l.Secret); err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	if err := netshare.Connect(l.Path, user, pw); err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	if err := s.db.SetMediaLocationLogin(ctx, id, user, secret); err != nil {
		writeErr(w, err)
		return
	}
	s.setShareStatus(l.Path, "")
	s.listLocations(w, r)
}

// deleteLocation removes a saved location, unless libraries are in it (and
// in no other location): they'd be left where nothing may be.
func (s *Server) deleteLocation(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	ctx := r.Context()
	l, err := s.db.MediaLocation(ctx, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	var others []string
	for _, p := range s.mediaRoots(ctx) {
		if p != l.Path {
			others = append(others, p)
		}
	}
	libs, err := s.db.Libraries(ctx)
	if err != nil {
		writeErr(w, err)
		return
	}
	var stuck []string
	for _, lib := range libs {
		if within(lib.Path, []string{l.Path}) && !within(lib.Path, others) {
			stuck = append(stuck, lib.Name)
		}
	}
	if len(stuck) > 0 {
		writeErr(w, badRequest("remove the libraries in it first: "+strings.Join(stuck, ", ")))
		return
	}
	if err := s.db.DeleteMediaLocation(ctx, id); err != nil {
		writeErr(w, err)
		return
	}
	// Sign out of the share unless another location is on it.
	if isShare(l.Path) && netshare.Supported && (l.User != "" || len(l.Secret) > 0) {
		root, _ := netshare.Root(l.Path)
		still := false
		for _, p := range others {
			if r, err := netshare.Root(p); err == nil && strings.EqualFold(r, root) {
				still = true
			}
		}
		if !still {
			_ = netshare.Disconnect(root)
		}
	}
	slog.Info("media location removed", "path", l.Path)
	s.listLocations(w, r)
}

// statWithin is os.Stat, giving up after d (an unreachable NAS can hang it).
func statWithin(p string, d time.Duration) (os.FileInfo, error) {
	type res struct {
		st  os.FileInfo
		err error
	}
	ch := make(chan res, 1)
	go func() {
		st, err := os.Stat(p)
		ch <- res{st, err}
	}()
	select {
	case x := <-ch:
		return x.st, x.err
	case <-time.After(d):
		return nil, errors.New("timed out")
	}
}
