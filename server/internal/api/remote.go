package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// The remote image cache. Guide data and metadata providers hand us image
// URLs (channel logos, programme art, match-picker posters); clients load
// them through /api/artwork/remote?u=… (db.ImageURL writes those paths), and
// each image is fetched once, kept under $CACHE/remote and served from there.
// Only these hosts are fetched, so the endpoint can't be used as a proxy.
var remoteHosts = []string{"image.tmdb.org", ".media-amazon.com", ".media-imdb.com", ".hdhomerun.com", ".silicondust.com"}

const (
	remoteMaxBytes = 10 << 20
	remoteKeep     = 90 * 24 * time.Hour // unused images are pruned after this long
	remoteTouch    = 24 * time.Hour      // how often a served image's "last used" time is refreshed
)

var remoteClient = &http.Client{Timeout: 20 * time.Second}

func remoteAllowed(u *url.URL) bool {
	if u.Scheme != "https" && u.Scheme != "http" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, h := range remoteHosts {
		if host == strings.TrimPrefix(h, ".") || (strings.HasPrefix(h, ".") && strings.HasSuffix(host, h)) {
			return true
		}
	}
	return false
}

func (s *Server) remoteDir() string { return filepath.Join(s.cfg.CacheDir, "remote") }

func (s *Server) remotePath(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	key := hex.EncodeToString(sum[:])
	return filepath.Join(s.remoteDir(), key[:2], key)
}

// remoteImage serves a cached remote image, fetching it the first time.
func (s *Server) remoteImage(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("u")
	u, err := url.Parse(raw)
	if err != nil || !remoteAllowed(u) {
		http.Error(w, "not an image we fetch", http.StatusBadRequest)
		return
	}
	path := s.remotePath(raw)
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		if err := s.fetchRemote(r.Context(), raw, path); err != nil {
			slog.Debug("remote image", "url", raw, "err", err)
			http.NotFound(w, r)
			return
		}
	} else if err == nil && time.Since(info.ModTime()) > remoteTouch {
		now := time.Now()
		_ = os.Chtimes(path, now, now) // still in use: keep it past the next prune
	}
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	w.Header().Set("Content-Type", http.DetectContentType(head[:n]))
	w.Header().Set("Cache-Control", "public, max-age=604800")
	_, _ = f.Seek(0, io.SeekStart)
	http.ServeContent(w, r, "", time.Time{}, f)
}

// remoteLocks makes concurrent requests for one image share a single fetch
// (a guide page asks for dozens of logos at once).
var remoteLocks sync.Map

func (s *Server) fetchRemote(ctx context.Context, raw, path string) error {
	mu, _ := remoteLocks.LoadOrStore(path, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer func() {
		mu.(*sync.Mutex).Unlock()
		remoteLocks.Delete(path)
	}()
	if _, err := os.Stat(path); err == nil {
		return nil // another request fetched it while we waited
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Couchside/"+s.version)
	resp, err := remoteClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "image/") {
		return fmt.Errorf("not an image: %s", ct)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".fetch-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	n, err := io.Copy(tmp, io.LimitReader(resp.Body, remoteMaxBytes+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if n > remoteMaxBytes {
		return errors.New("image too large")
	}
	return os.Rename(tmp.Name(), path)
}

// pruneRemote deletes cached remote images nobody has asked for in a while
// (guide art comes and goes with the listings).
func (s *Server) pruneRemote() {
	cutoff := time.Now().Add(-remoteKeep)
	removed := 0
	_ = filepath.WalkDir(s.remoteDir(), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil && info.ModTime().Before(cutoff) {
			if os.Remove(p) == nil {
				removed++
			}
		}
		return nil
	})
	if removed > 0 {
		slog.Info("pruned unused cached images", "count", removed)
	}
}
