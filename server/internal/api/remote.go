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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/timothydodd/couchside/internal/remoteimg"
)

// The remote image cache. Guide data and metadata providers hand us image
// URLs (channel logos, programme art, match-picker posters); clients load
// them through /api/artwork/remote?u=… (db.ImageURL writes those paths), and
// each image is fetched once, kept under $CACHE/remote and served from there.
// Only remoteimg.Hosts are fetched, so the endpoint can't be used as a proxy.

// remoteMaxTotal is the cache's size cap. A fetch that takes it past the cap
// drops the least recently used images straight away (the route needs no
// sign-in, so the daily prune alone would let anyone fill the volume).
var remoteMaxTotal int64 = 2 << 30

const (
	remoteMaxBytes = 10 << 20
	remoteKeep     = 90 * 24 * time.Hour // unused images are pruned after this long
	remoteTouch    = 24 * time.Hour      // how often a served image's "last used" time is refreshed
	remoteMissFor  = 10 * time.Minute    // a failed fetch isn't retried for this long
	remoteMissMax  = 10000
)

var remoteClient = remoteimg.NewClient(20 * time.Second)

func (s *Server) remoteDir() string { return filepath.Join(s.cfg.CacheDir, "remote") }

// remotePath is the cache file for a canonical URL (remoteimg.Key).
func (s *Server) remotePath(key string) string {
	sum := sha256.Sum256([]byte(key))
	h := hex.EncodeToString(sum[:])
	return filepath.Join(s.remoteDir(), h[:2], h)
}

// remoteImage serves a cached remote image, fetching it the first time.
func (s *Server) remoteImage(w http.ResponseWriter, r *http.Request) {
	u, err := url.Parse(r.URL.Query().Get("u"))
	if err != nil || !remoteimg.Allowed(u) {
		http.Error(w, "not an image we fetch", http.StatusBadRequest)
		return
	}
	key := remoteimg.Key(u)
	path := s.remotePath(key)
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		if remoteMisses.recent(key) {
			http.NotFound(w, r)
			return
		}
		if err := s.fetchRemote(r.Context(), key, path); err != nil {
			slog.Debug("remote image", "url", key, "err", err)
			if r.Context().Err() == nil {
				remoteMisses.add(key)
			}
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
	ct := remoteimg.Sniff(head[:n])
	if ct == "" {
		// Cached before types were checked: never serve it as anything.
		_ = os.Remove(path)
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=604800")
	_, _ = f.Seek(0, io.SeekStart)
	http.ServeContent(w, r, "", time.Time{}, f)
}

// missCache remembers URLs whose fetch just failed, so a page full of dead
// links doesn't refetch each one on every load. Bounded.
type missCache struct {
	mu sync.Mutex
	m  map[string]time.Time
}

var remoteMisses = &missCache{m: map[string]time.Time{}}

func (c *missCache) recent(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.m[key]
	if ok && time.Since(t) >= remoteMissFor {
		delete(c.m, key)
		return false
	}
	return ok
}

func (c *missCache) add(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) >= remoteMissMax {
		for k, t := range c.m {
			if time.Since(t) >= remoteMissFor {
				delete(c.m, k)
			}
		}
		if len(c.m) >= remoteMissMax {
			c.m = map[string]time.Time{}
		}
	}
	c.m[key] = time.Now()
}

// remoteLocks makes concurrent requests for one image share a single fetch
// (a guide page asks for dozens of logos at once).
var remoteLocks sync.Map

func (s *Server) fetchRemote(ctx context.Context, key, path string) error {
	mu, _ := remoteLocks.LoadOrStore(path, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer func() {
		mu.(*sync.Mutex).Unlock()
		remoteLocks.Delete(path)
	}()
	if _, err := os.Stat(path); err == nil {
		return nil // another request fetched it while we waited
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, key, nil)
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
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "image/") || strings.HasPrefix(ct, "image/svg") {
		return fmt.Errorf("not an image we keep: %q", ct)
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
	// Whatever the server said, keep only bytes that are an image.
	head := make([]byte, 512)
	f, err := os.Open(tmp.Name())
	if err != nil {
		return err
	}
	hn, _ := io.ReadFull(f, head)
	f.Close()
	if remoteimg.Sniff(head[:hn]) == "" {
		return errors.New("not an image")
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	if s.remoteSize.Add(n) > remoteMaxTotal && s.remotePruning.TryLock() {
		s.pruneRemoteLocked()
		s.remotePruning.Unlock()
	}
	return nil
}

// pruneRemote deletes cached remote images nobody has asked for in a while
// (guide art comes and goes with the listings), then the least recently
// used ones while the cache is over remoteMaxTotal, down to nine tenths of it
// so a full cache isn't walked again on the next fetch. It also sets
// remoteSize, which fetches add to in between.
func (s *Server) pruneRemote() {
	s.remotePruning.Lock()
	defer s.remotePruning.Unlock()
	s.pruneRemoteLocked()
}

func (s *Server) pruneRemoteLocked() {
	type entry struct {
		path string
		size int64
		used time.Time
	}
	cutoff := time.Now().Add(-remoteKeep)
	removed := 0
	var keep []entry
	var total int64
	_ = filepath.WalkDir(s.remoteDir(), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			if os.Remove(p) == nil {
				removed++
			}
			return nil
		}
		keep = append(keep, entry{p, info.Size(), info.ModTime()})
		total += info.Size()
		return nil
	})
	if total > remoteMaxTotal {
		sort.Slice(keep, func(i, j int) bool { return keep[i].used.Before(keep[j].used) })
		for _, e := range keep {
			if total <= remoteMaxTotal/10*9 {
				break
			}
			if os.Remove(e.path) == nil {
				removed++
				total -= e.size
			}
		}
	}
	s.remoteSize.Store(total)
	if removed > 0 {
		slog.Info("pruned cached images", "count", removed)
	}
}
