package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Cache stores raw provider responses (backed by the provider_cache table).
type Cache interface {
	CacheGet(ctx context.Context, provider, key string, maxAge time.Duration) ([]byte, bool)
	CachePut(ctx context.Context, provider, key string, body []byte) error
}

// OMDb talks to https://www.omdbapi.com. The free tier allows 1,000
// requests a day, so every response is cached and requests are spaced out.
type OMDb struct {
	key    string
	cache  Cache
	client *http.Client
	base   string

	mu   sync.Mutex
	next time.Time
}

func NewOMDb(key string, cache Cache) *OMDb {
	return &OMDb{key: key, cache: cache, client: &http.Client{Timeout: 15 * time.Second}, base: "https://www.omdbapi.com/"}
}

func (o *OMDb) Name() string { return "omdb" }

const cacheTTL = 30 * 24 * time.Hour

type omdbTitle struct {
	Response     string `json:"Response"`
	Error        string `json:"Error"`
	Title        string `json:"Title"`
	Year         string `json:"Year"`
	Rated        string `json:"Rated"`
	Runtime      string `json:"Runtime"`
	Genre        string `json:"Genre"`
	Plot         string `json:"Plot"`
	Poster       string `json:"Poster"`
	ImdbRating   string `json:"imdbRating"`
	ImdbID       string `json:"imdbID"`
	Type         string `json:"Type"`
	TotalSeasons string `json:"totalSeasons"`
}

type omdbSearch struct {
	Response string `json:"Response"`
	Error    string `json:"Error"`
	Search   []struct {
		ImdbID string `json:"imdbID"`
		Title  string `json:"Title"`
		Year   string `json:"Year"`
	} `json:"Search"`
}

type omdbSeason struct {
	Response string `json:"Response"`
	Error    string `json:"Error"`
	Episodes []struct {
		Title      string `json:"Title"`
		Released   string `json:"Released"`
		Episode    string `json:"Episode"`
		ImdbRating string `json:"imdbRating"`
		ImdbID     string `json:"imdbID"`
	} `json:"Episodes"`
}

// errNotFound is OMDb's "Response: False" for an unknown title.
var errNotFound = errors.New("not found")

// Lookup gathers candidates from an exact-title query with and without the
// year and from a search, then picks the best title/year fit. OMDb's title
// lookup is fuzzy (it happily returns a featurette or podcast episode), so
// nothing is trusted without scoring.
func (o *OMDb) Lookup(ctx context.Context, kind Kind, title string, year int) (*Details, error) {
	var cands []candidate
	byID := map[string]*Details{}
	addTitle := func(q url.Values) error {
		d, err := o.title(ctx, q)
		if errors.Is(err, errNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if d.ImdbID != "" && byID[d.ImdbID] == nil {
			byID[d.ImdbID] = d
			cands = append(cands, candidate{id: d.ImdbID, title: d.Title, year: d.Year})
		}
		return nil
	}
	q := url.Values{"t": {title}, "type": {string(kind)}, "plot": {"full"}}
	if year > 0 {
		q.Set("y", strconv.Itoa(year))
		if err := addTitle(q); err != nil {
			return nil, err
		}
		q.Del("y")
	}
	if err := addTitle(q); err != nil {
		return nil, err
	}
	// Search results only carry title/year; details are fetched for the winner.
	// Search without the year first (catches release-year drift), then with it
	// (catches remakes buried past the first page).
	search := func(withYear bool) error {
		var s omdbSearch
		sq := url.Values{"s": {title}, "type": {string(kind)}}
		if withYear {
			sq.Set("y", strconv.Itoa(year))
		}
		if err := o.get(ctx, sq, &s); err != nil && !errors.Is(err, errNotFound) {
			return err
		}
		for i, r := range s.Search {
			if i >= 10 {
				break
			}
			if _, seen := byID[r.ImdbID]; seen || r.ImdbID == "" {
				continue
			}
			y := 0
			if len(r.Year) >= 4 {
				y, _ = strconv.Atoi(r.Year[:4])
			}
			byID[r.ImdbID] = nil
			cands = append(cands, candidate{id: r.ImdbID, title: r.Title, year: y})
		}
		return nil
	}
	good := func() bool {
		best, ok := pickBest(kind, title, year, cands)
		return ok && TitleScore(title, best.title) == 3 && (year == 0 || abs(best.year-year) <= 1)
	}
	if !good() {
		if err := search(false); err != nil {
			return nil, err
		}
	}
	if !good() && year > 0 {
		if err := search(true); err != nil {
			return nil, err
		}
	}
	best, ok := pickBest(kind, title, year, cands)
	if !ok {
		return nil, nil
	}
	if d := byID[best.id]; d != nil {
		return d, nil
	}
	return o.ByImdbID(ctx, kind, best.id)
}

func (o *OMDb) ByImdbID(ctx context.Context, kind Kind, imdbID string) (*Details, error) {
	d, err := o.title(ctx, url.Values{"i": {imdbID}, "plot": {"full"}})
	if errors.Is(err, errNotFound) {
		return nil, nil
	}
	return d, err
}

func (o *OMDb) Season(ctx context.Context, seriesID string, season int) ([]Episode, error) {
	var s omdbSeason
	if err := o.get(ctx, url.Values{"i": {seriesID}, "Season": {strconv.Itoa(season)}}, &s); err != nil {
		if errors.Is(err, errNotFound) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]Episode, 0, len(s.Episodes))
	for _, e := range s.Episodes {
		n, err := strconv.Atoi(e.Episode)
		if err != nil {
			continue
		}
		out = append(out, Episode{Season: season, Episode: n, Title: na(e.Title), Released: na(e.Released),
			Rating: parseFloat(e.ImdbRating), ImdbID: e.ImdbID})
	}
	return out, nil
}

func (o *OMDb) title(ctx context.Context, q url.Values) (*Details, error) {
	var t omdbTitle
	if err := o.get(ctx, q, &t); err != nil {
		return nil, err
	}
	d := &Details{
		ID: t.ImdbID, ImdbID: t.ImdbID, Title: t.Title, Plot: na(t.Plot), Rated: na(t.Rated),
		Rating: parseFloat(t.ImdbRating), PosterURL: upscalePoster(na(t.Poster)),
	}
	if len(t.Year) >= 4 {
		d.Year, _ = strconv.Atoi(t.Year[:4]) // series years look like "2008–2013"
	}
	if g := na(t.Genre); g != "" {
		for _, p := range strings.Split(g, ",") {
			d.Genres = append(d.Genres, strings.TrimSpace(p))
		}
	}
	if f := strings.Fields(t.Runtime); len(f) > 0 {
		if n, err := strconv.Atoi(f[0]); err == nil {
			d.RuntimeMin = &n
		}
	}
	if n, err := strconv.Atoi(t.TotalSeasons); err == nil {
		d.TotalSeasons = &n
	}
	return d, nil
}

// get performs a cached, rate-limited request and decodes into out.
func (o *OMDb) get(ctx context.Context, q url.Values, out any) error {
	cacheKey := q.Encode() // excludes the API key
	body, ok := o.cache.CacheGet(ctx, o.Name(), cacheKey, cacheTTL)
	if !ok {
		var err error
		if body, err = o.fetch(ctx, q); err != nil {
			return err
		}
	}
	var status struct{ Response, Error string }
	if err := json.Unmarshal(body, &status); err != nil {
		return fmt.Errorf("omdb: bad response: %w", err)
	}
	if status.Response != "True" {
		// "Too many results" comes back for very short searches ("It"): no usable answer.
		if e := strings.ToLower(status.Error); strings.Contains(e, "not found") || strings.Contains(e, "too many results") {
			if !ok {
				_ = o.cache.CachePut(ctx, o.Name(), cacheKey, body)
			}
			return errNotFound
		}
		return fmt.Errorf("omdb: %s", status.Error) // bad key, quota: don't cache
	}
	if !ok {
		_ = o.cache.CachePut(ctx, o.Name(), cacheKey, body)
	}
	return json.Unmarshal(body, out)
}

func (o *OMDb) fetch(ctx context.Context, q url.Values) ([]byte, error) {
	o.mu.Lock()
	wait := time.Until(o.next)
	o.next = time.Now().Add(max(wait, 0) + 250*time.Millisecond)
	o.mu.Unlock()
	if wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	full := url.Values{}
	for k, v := range q {
		full[k] = v
	}
	full.Set("apikey", o.key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.base+"?"+full.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("omdb: %w", redactKey(err, o.key))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	// OMDb answers 401 with a JSON error body; let get() report it.
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("omdb: HTTP %d", resp.StatusCode)
	}
	return body, nil
}

// redactKey keeps the API key out of error messages (url.Error includes the URL).
func redactKey(err error, key string) error {
	if key == "" {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), key, "***"))
}

func na(s string) string {
	if s == "N/A" {
		return ""
	}
	return s
}

func parseFloat(s string) *float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}

// upscalePoster asks Amazon's image CDN for a larger rendition: OMDb hands
// out 300px posters ("..._V1_SX300.jpg"), which look soft on hi-dpi grids.
func upscalePoster(u string) string {
	if i := strings.LastIndex(u, "._V1_"); i >= 0 && strings.HasSuffix(u, ".jpg") {
		return u[:i] + "._V1_SX800.jpg"
	}
	return u
}

// SearchSeries lists the series OMDb knows by a title (first page of
// results), with the years each ran, e.g. "1985–1992" or "2016–".
func (o *OMDb) SearchSeries(ctx context.Context, title string) ([]SeriesMatch, error) {
	var s omdbSearch
	if err := o.get(ctx, url.Values{"s": {title}, "type": {"series"}}, &s); err != nil && !errors.Is(err, errNotFound) {
		return nil, err
	}
	out := make([]SeriesMatch, 0, len(s.Search))
	for _, r := range s.Search {
		if r.ImdbID == "" {
			continue
		}
		start, end := yearRange(r.Year)
		out = append(out, SeriesMatch{ImdbID: r.ImdbID, Title: r.Title, StartYear: start, EndYear: end})
	}
	return out, nil
}

// yearRange reads OMDb's Year: "2010" (one year), "1985–1992", or "2016–"
// (still running, end 0).
func yearRange(s string) (start, end int) {
	s = strings.TrimSpace(s)
	if len(s) < 4 {
		return 0, 0
	}
	start, _ = strconv.Atoi(s[:4])
	rest := strings.TrimLeft(s[4:], "–-— ")
	switch {
	case len(s) == 4:
		end = start
	case len(rest) >= 4:
		end, _ = strconv.Atoi(rest[:4])
	}
	return start, end
}
