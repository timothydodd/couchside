package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/imaging"
	"github.com/timothydodd/couchside/internal/metadata"
	"github.com/timothydodd/couchside/internal/remoteimg"
)

func (w *Worker) match(ctx context.Context, itemID int64) error {
	item, err := w.db.Item(ctx, itemID)
	if err != nil {
		return err
	}
	label := "Artwork " + item.Title
	if w.providers.Empty() {
		if err := w.db.ClearMatch(ctx, itemID); err != nil {
			return err
		}
		return w.Enqueue(ctx, KindArtwork, itemID, label)
	}
	kind := metadata.Movie
	if item.Kind == "series" {
		kind = metadata.Series
	}
	pinned := ""
	if item.ImdbPinned {
		pinned = item.ImdbID
	}
	d, p, skipped, err := w.providers.Lookup(ctx, kind, item.ParsedTitle, item.ParsedYear, pinned)
	if err != nil {
		return err
	}
	if pinned == "" && (d == nil || metadata.TitleScore(item.ParsedTitle, d.Title) < 3) {
		// No match, or only a partial one (a featurette, a franchise title):
		// filenames can't hold ":" and often spell things differently than
		// the database ("3.10 to Yuma", "Hellboy II - The Golden Army").
		// Keep whichever answer fits its query best.
		bestScore := 0
		if d != nil {
			bestScore = metadata.TitleScore(item.ParsedTitle, d.Title)
		}
		for _, v := range metadata.TitleVariants(item.ParsedTitle) {
			vd, vp, vskipped, err := w.providers.Lookup(ctx, kind, v, item.ParsedYear, "")
			if err != nil {
				return err
			}
			if vd == nil {
				continue
			}
			if sc := metadata.TitleScore(v, vd.Title); sc > bestScore {
				d, p, skipped, bestScore = vd, vp, vskipped, sc
				slog.Info("matched using a title variant", "file_title", item.ParsedTitle, "variant", v, "match", vd.Title)
				if sc == 3 {
					break
				}
			}
		}
	}
	if d == nil {
		if err := w.db.ClearMatch(ctx, itemID); err != nil {
			return err
		}
		return w.Enqueue(ctx, KindArtwork, itemID, label)
	}
	if err := w.db.ApplyMetadata(ctx, itemID, db.Metadata{
		Title: d.Title, Year: d.Year, Plot: d.Plot, Genres: d.Genres, Rated: d.Rated, Rating: d.Rating,
		RuntimeMin: d.RuntimeMin, ImdbID: d.ImdbID, TotalSeasons: d.TotalSeasons, PosterURL: d.PosterURL, BackdropURL: d.BackdropURL,
		Provider: p.Name(), Partial: skipped != nil,
	}); err != nil {
		return err
	}
	if skipped != nil {
		// A provider ahead of this one is down or rate-limiting. Its cast,
		// crew and backdrop stay as they are, and the item stays pending so
		// the next scan asks again.
		slog.Warn("matched without a provider that couldn't be reached; will retry", "title", d.Title, "used", p.Name(), "err", skipped)
	}
	if skipped == nil || len(d.Credits) > 0 {
		credits := make([]db.Credit, len(d.Credits))
		for i, c := range d.Credits {
			credits[i] = db.Credit{PersonID: c.PersonID, Name: c.Name, ProfilePath: c.ProfilePath, Kind: c.Kind, Role: c.Role, Order: c.Order}
		}
		if err := w.db.SetCredits(ctx, itemID, credits); err != nil {
			return err
		}
	}
	if item.Kind == "series" && d.ImdbID != "" {
		seasons, err := w.db.SeriesSeasons(ctx, itemID)
		if err != nil {
			return err
		}
		for _, s := range seasons {
			eps, err := p.Season(ctx, d.ImdbID, s)
			if err != nil {
				slog.Warn("season lookup failed", "series", d.Title, "season", s, "err", err)
				continue
			}
			meta := make([]db.EpisodeMeta, len(eps))
			for i, e := range eps {
				meta[i] = db.EpisodeMeta{Season: e.Season, Episode: e.Episode, Title: e.Title, Released: e.Released, Rating: e.Rating, ImdbID: e.ImdbID}
			}
			if err := w.db.ApplyEpisodeMeta(ctx, itemID, meta); err != nil {
				return err
			}
		}
	}
	return w.Enqueue(ctx, KindArtwork, itemID, "Artwork "+d.Title)
}

// ItemArtDir is where an item's poster and backdrop live.
func ItemArtDir(cacheDir string, itemID int64) string {
	return filepath.Join(cacheDir, "items", fmt.Sprint(itemID))
}

func FileStillPath(cacheDir string, fileID int64) string {
	return filepath.Join(cacheDir, "files", fmt.Sprint(fileID), "still.webp")
}

// artwork downloads and resizes the poster and the provider's backdrop. With
// no backdrop (OMDb has none, and not every TMDB title does) it grabs a frame
// from the video itself.
func (w *Worker) artwork(ctx context.Context, itemID int64) error {
	item, err := w.db.Item(ctx, itemID)
	if err != nil {
		return err
	}
	dir := ItemArtDir(w.cfg.CacheDir, itemID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	hasPoster, hasBackdrop := false, false
	var errs []string
	// Uploaded artwork stays; only the rest is fetched or grabbed.
	customPoster, customBackdrop, err := w.db.CustomArtwork(ctx, itemID)
	if err != nil {
		return err
	}
	hasPoster, hasBackdrop = customPoster, customBackdrop

	if item.PosterURL != "" && !customPoster {
		switch err := w.poster(ctx, item.PosterURL, dir); {
		case errors.Is(err, errGone):
			// OMDb has plenty of dead poster links; show the placeholder, don't fail.
			slog.Info("poster link is dead, using placeholder", "title", item.Title)
		case err != nil:
			errs = append(errs, "poster: "+err.Error())
		default:
			hasPoster = true
		}
	}
	if item.BackdropURL != "" && !customBackdrop {
		switch err := w.backdrop(ctx, item.BackdropURL, dir); {
		case errors.Is(err, errGone):
			slog.Info("backdrop link is dead, grabbing a frame instead", "title", item.Title)
		case err != nil:
			errs = append(errs, "backdrop: "+err.Error())
		default:
			hasBackdrop = true
		}
	}
	if src, err := w.db.BackdropSource(ctx, itemID); err == nil && src != nil && !customBackdrop && !hasBackdrop {
		at := imaging.GrabOffset(src.DurationSec, 0.2)
		_ = os.Remove(filepath.Join(dir, "backdrop.src")) // a frame grab, not the provider's image
		if err := w.ff.FrameGrab(ctx, src.Path, filepath.Join(dir, "backdrop.webp"), at, 1280); err != nil {
			errs = append(errs, "backdrop: "+err.Error())
		} else {
			hasBackdrop = true
		}
	}
	if err := w.db.SetArtwork(ctx, itemID, hasPoster, hasBackdrop); err != nil {
		return err
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// fetchedFrom records which link an artwork file came from, so a re-match
// that finds the same link doesn't download it again.
func fetchedFrom(dir, name, url string) bool {
	src, err := os.ReadFile(filepath.Join(dir, name+".src"))
	if err != nil || string(src) != url {
		return false
	}
	_, err = os.Stat(filepath.Join(dir, name+".webp"))
	return err == nil
}

func markFetched(dir, name, url string) {
	_ = os.WriteFile(filepath.Join(dir, name+".src"), []byte(url), 0o644)
}

func (w *Worker) poster(ctx context.Context, url, dir string) error {
	if fetchedFrom(dir, "poster", url) {
		return nil
	}
	orig := filepath.Join(dir, "poster.orig")
	err := download(ctx, url, orig)
	if err != nil && strings.Contains(url, "._V1_SX800.jpg") {
		// The upscaled rendition isn't always available; fall back to OMDb's.
		err = download(ctx, strings.Replace(url, "._V1_SX800.jpg", "._V1_SX300.jpg", 1), orig)
	}
	if err != nil {
		return err
	}
	defer os.Remove(orig)
	if err := w.ff.Resize(ctx, orig, filepath.Join(dir, "poster.webp"), 780); err != nil {
		return err
	}
	if err := w.ff.Resize(ctx, orig, filepath.Join(dir, "poster-thumb.webp"), 360); err != nil {
		return err
	}
	markFetched(dir, "poster", url)
	return nil
}

// PersonPhotoPath is where a cast or crew member's photo is cached.
func PersonPhotoPath(cacheDir string, personID int64) string {
	return filepath.Join(cacheDir, "people", fmt.Sprintf("%d.webp", personID))
}

var (
	photoLocks sync.Map // person id → *sync.Mutex: one fetch per person, others in parallel
	photoMiss  sync.Map // person id → time.Time of a failed fetch
)

// photoMissFor is how long a person whose photo couldn't be fetched isn't tried again.
const photoMissFor = 24 * time.Hour

// ErrNoPhoto means the photo failed recently and isn't being tried again yet.
var ErrNoPhoto = errors.New("no photo (failed recently)")

// PersonPhoto fetches a person's TMDB profile photo into the cache the first
// time it's asked for, so browsers and TVs never call TMDB themselves.
func (w *Worker) PersonPhoto(ctx context.Context, personID int64, profilePath string) (string, error) {
	dst := PersonPhotoPath(w.cfg.CacheDir, personID)
	if _, err := os.Stat(dst); err == nil {
		return dst, nil
	}
	if at, ok := photoMiss.Load(personID); ok {
		if time.Since(at.(time.Time)) < photoMissFor {
			return "", ErrNoPhoto
		}
		photoMiss.Delete(personID)
	}
	lk, _ := photoLocks.LoadOrStore(personID, &sync.Mutex{})
	mu := lk.(*sync.Mutex)
	mu.Lock()
	defer func() {
		mu.Unlock()
		photoLocks.Delete(personID)
	}()
	if _, err := os.Stat(dst); err == nil {
		return dst, nil // fetched while we waited
	}
	path, err := w.fetchPersonPhoto(ctx, dst, profilePath)
	if err != nil && ctx.Err() == nil {
		photoMiss.Store(personID, time.Now())
	}
	return path, err
}

func (w *Worker) fetchPersonPhoto(ctx context.Context, dst, profilePath string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	orig := dst + ".orig"
	if err := download(ctx, "https://image.tmdb.org/t/p/w185"+profilePath, orig); err != nil {
		return "", err
	}
	defer os.Remove(orig)
	if err := w.ff.Resize(ctx, orig, dst, 185); err != nil {
		return "", err
	}
	return dst, nil
}

func (w *Worker) backdrop(ctx context.Context, url, dir string) error {
	if fetchedFrom(dir, "backdrop", url) {
		return nil
	}
	orig := filepath.Join(dir, "backdrop.orig")
	if err := download(ctx, url, orig); err != nil {
		return err
	}
	defer os.Remove(orig)
	if err := w.ff.Resize(ctx, orig, filepath.Join(dir, "backdrop.webp"), 1280); err != nil {
		return err
	}
	markFetched(dir, "backdrop", url)
	return nil
}

func (w *Worker) still(ctx context.Context, fileID int64) error {
	f, err := w.db.File(ctx, fileID)
	if err != nil {
		if err == db.ErrNotFound {
			return nil // file was pruned before we got to it
		}
		return err
	}
	dst := FileStillPath(w.cfg.CacheDir, fileID)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := w.ff.FrameGrab(ctx, f.Path, dst, imaging.GrabOffset(f.DurationSec, 0.25), 480); err != nil {
		return err
	}
	return w.db.SetFileStill(ctx, fileID, true)
}

// Artwork comes only from the providers' image hosts, redirects included.
var httpClient = remoteimg.NewClient(30 * time.Second)

// errGone means the remote image doesn't exist (404/410).
var errGone = errors.New("remote image not found")

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func download(ctx context.Context, url, dst string) error {
	if u, err := neturl.Parse(url); err != nil || !remoteimg.Allowed(u) {
		return fmt.Errorf("not an image host Couchside fetches from: %s", url)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return errGone
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, io.LimitReader(resp.Body, 15<<20))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}
