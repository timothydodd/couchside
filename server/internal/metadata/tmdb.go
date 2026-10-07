package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TMDB talks to The Movie Database (https://www.themoviedb.org), free for
// non-commercial use with attribution. It's the primary provider: it has
// backdrops, works without anyone signing up (the release builds carry an
// app key), and answers fast. Couchside keeps the IMDb id as an item's
// identity, so TMDB answers are keyed by IMDb id when TMDB knows one, and by
// "tmdb:movie:<id>" / "tmdb:tv:<id>" otherwise (see TMDBID).
//
// The key may be a v3 API key or a v4 read access token (a JWT, sent as a
// Bearer token). Responses are cached like OMDb's; TV data for less long,
// because running shows gain episodes.
type TMDB struct {
	key      string
	cache    Cache
	client   *http.Client
	base     string
	language string

	mu   sync.Mutex
	next time.Time
}

func NewTMDB(key string, cache Cache) *TMDB {
	return &TMDB{key: key, cache: cache, client: &http.Client{Timeout: 15 * time.Second},
		base: "https://api.themoviedb.org/3", language: "en-US"}
}

func (t *TMDB) Name() string { return "tmdb" }

const (
	tmdbImages     = "https://image.tmdb.org/t/p/"
	tmdbMovieTTL   = 30 * 24 * time.Hour
	tmdbTVTTL      = 3 * 24 * time.Hour
	tmdbSpacing    = 30 * time.Millisecond // TMDB allows about 40 requests a second
	tmdbSearchTop  = 10                    // candidates kept per search
	tmdbPickerSize = 12                    // results shown when fixing a match by hand
)

var reTMDBID = regexp.MustCompile(`tmdb:(movie|tv):(\d+)`)
var reTMDBURL = regexp.MustCompile(`themoviedb\.org/(movie|tv)/(\d+)`)

// TMDBID finds a TMDB reference in s: "tmdb:movie:603", or a themoviedb.org
// URL. It returns the normalized form, or "".
func TMDBID(s string) string {
	if m := reTMDBID.FindStringSubmatch(s); m != nil {
		return "tmdb:" + m[1] + ":" + m[2]
	}
	if m := reTMDBURL.FindStringSubmatch(s); m != nil {
		return "tmdb:" + m[1] + ":" + m[2]
	}
	return ""
}

func tmdbKind(k Kind) string {
	if k == Series {
		return "tv"
	}
	return "movie"
}

// --- responses ---------------------------------------------------------------

type tmdbSearchResult struct {
	ID           int    `json:"id"`
	Title        string `json:"title"` // movies
	Name         string `json:"name"`  // tv
	ReleaseDate  string `json:"release_date"`
	FirstAirDate string `json:"first_air_date"`
	PosterPath   string `json:"poster_path"`
}

func (r tmdbSearchResult) title() string { return firstNonEmpty(r.Title, r.Name) }
func (r tmdbSearchResult) date() string  { return firstNonEmpty(r.ReleaseDate, r.FirstAirDate) }

type tmdbSearch struct {
	Results []tmdbSearchResult `json:"results"`
}

type tmdbDetails struct {
	ID              int                     `json:"id"`
	Title           string                  `json:"title"`
	Name            string                  `json:"name"`
	ReleaseDate     string                  `json:"release_date"`
	FirstAirDate    string                  `json:"first_air_date"`
	LastAirDate     string                  `json:"last_air_date"`
	InProduction    bool                    `json:"in_production"`
	Overview        string                  `json:"overview"`
	Runtime         int                     `json:"runtime"`
	EpisodeRunTime  []int                   `json:"episode_run_time"`
	VoteAverage     float64                 `json:"vote_average"`
	VoteCount       int                     `json:"vote_count"`
	PosterPath      string                  `json:"poster_path"`
	BackdropPath    string                  `json:"backdrop_path"`
	NumberOfSeasons int                     `json:"number_of_seasons"`
	ImdbID          string                  `json:"imdb_id"`
	Genres          []struct{ Name string } `json:"genres"`
	ExternalIDs     struct {
		ImdbID string `json:"imdb_id"`
	} `json:"external_ids"`
	ReleaseDates struct {
		Results []struct {
			Country  string `json:"iso_3166_1"`
			Releases []struct {
				Certification string `json:"certification"`
			} `json:"release_dates"`
		} `json:"results"`
	} `json:"release_dates"`
	Credits          tmdbCredits `json:"credits"`           // movies
	AggregateCredits tmdbCredits `json:"aggregate_credits"` // tv: every season
	CreatedBy        []struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		ProfilePath string `json:"profile_path"`
	} `json:"created_by"`
	ContentRatings struct {
		Results []struct {
			Country string `json:"iso_3166_1"`
			Rating  string `json:"rating"`
		} `json:"results"`
	} `json:"content_ratings"`
}

type tmdbCredits struct {
	Cast []struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		ProfilePath string `json:"profile_path"`
		Character   string `json:"character"` // movies
		Order       int    `json:"order"`
		Roles       []struct {
			Character string `json:"character"`
		} `json:"roles"` // tv aggregate credits
	} `json:"cast"`
	Crew []struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		ProfilePath string `json:"profile_path"`
		Job         string `json:"job"` // movies
	} `json:"crew"`
}

// castLimit is how many billed actors are kept; crewJobs are the crew worth
// showing (a film's crew list runs to hundreds).
const castLimit = 20

var crewJobs = map[string]int{"Director": 0, "Screenplay": 1, "Writer": 2, "Story": 3, "Novel": 4,
	"Original Music Composer": 5, "Director of Photography": 6}

func (d tmdbDetails) credits(kind Kind) []Credit {
	var out []Credit
	src := d.Credits
	if kind == Series {
		src = d.AggregateCredits
		for i, c := range d.CreatedBy {
			out = append(out, Credit{PersonID: c.ID, Name: c.Name, ProfilePath: c.ProfilePath, Kind: "crew", Role: "Creator", Order: i})
		}
	}
	for i, c := range src.Cast {
		if i >= castLimit {
			break
		}
		role := c.Character
		if role == "" && len(c.Roles) > 0 {
			role = c.Roles[0].Character
		}
		out = append(out, Credit{PersonID: c.ID, Name: c.Name, ProfilePath: c.ProfilePath, Kind: "cast", Role: role, Order: i})
	}
	if kind == Movie {
		seen := map[string]bool{}
		for _, c := range src.Crew {
			rank, ok := crewJobs[c.Job]
			key := fmt.Sprintf("%d|%s", c.ID, c.Job)
			if !ok || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, Credit{PersonID: c.ID, Name: c.Name, ProfilePath: c.ProfilePath, Kind: "crew", Role: c.Job, Order: 100 + rank})
		}
	}
	return out
}

type tmdbSeason struct {
	Episodes []struct {
		ID          int64   `json:"id"`
		Episode     int     `json:"episode_number"`
		Season      int     `json:"season_number"`
		Name        string  `json:"name"`
		AirDate     string  `json:"air_date"`
		Overview    string  `json:"overview"`
		Runtime     int     `json:"runtime"`
		StillPath   string  `json:"still_path"`
		VoteAverage float64 `json:"vote_average"`
		VoteCount   int     `json:"vote_count"`
		GuestStars  []struct {
			ID          int64  `json:"id"`
			Name        string `json:"name"`
			Character   string `json:"character"`
			ProfilePath string `json:"profile_path"`
			Order       int    `json:"order"`
		} `json:"guest_stars"`
		Crew []struct {
			ID          int64  `json:"id"`
			Name        string `json:"name"`
			Job         string `json:"job"`
			ProfilePath string `json:"profile_path"`
		} `json:"crew"`
	} `json:"episodes"`
}

type tmdbFind struct {
	MovieResults []struct{ ID int } `json:"movie_results"`
	TVResults    []struct{ ID int } `json:"tv_results"`
}

// --- Provider ------------------------------------------------------------------

// Lookup searches with the year and without it, then scores the candidates
// the same way as OMDb's (pickBest), and fetches details for the winner.
func (t *TMDB) Lookup(ctx context.Context, kind Kind, title string, year int) (*Details, error) {
	var cands []candidate
	seen := map[string]bool{}
	add := func(rs []tmdbSearchResult) {
		for i, r := range rs {
			if i >= tmdbSearchTop {
				break
			}
			id := strconv.Itoa(r.ID)
			if seen[id] {
				continue
			}
			seen[id] = true
			cands = append(cands, candidate{id: id, title: r.title(), year: yearOf(r.date())})
		}
	}
	if year > 0 {
		rs, err := t.search(ctx, kind, title, year)
		if err != nil {
			return nil, err
		}
		add(rs)
	}
	good := func() bool {
		best, ok := pickBest(kind, title, year, cands)
		return ok && TitleScore(title, best.title) == 3 && (year == 0 || abs(best.year-year) <= 1)
	}
	if !good() {
		rs, err := t.search(ctx, kind, title, 0)
		if err != nil {
			return nil, err
		}
		add(rs)
	}
	best, ok := pickBest(kind, title, year, cands)
	if !ok {
		return nil, nil
	}
	id, _ := strconv.Atoi(best.id)
	return t.details(ctx, kind, id)
}

// ByImdbID resolves an IMDb id through TMDB's find endpoint, or a TMDB
// reference ("tmdb:tv:1399") directly.
func (t *TMDB) ByImdbID(ctx context.Context, kind Kind, id string) (*Details, error) {
	k, n, err := t.resolve(ctx, kind, id)
	if err != nil || n == 0 {
		return nil, err
	}
	return t.details(ctx, k, n)
}

// Season lists a season's episodes. seriesID is an IMDb id or "tmdb:tv:<id>".
func (t *TMDB) Season(ctx context.Context, seriesID string, season int) ([]Episode, error) {
	_, n, err := t.resolve(ctx, Series, seriesID)
	if err != nil || n == 0 {
		return nil, err
	}
	var s tmdbSeason
	if err := t.get(ctx, fmt.Sprintf("/tv/%d/season/%d", n, season), nil, tmdbTVTTL, &s); err != nil {
		if errors.Is(err, errNotFound) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]Episode, 0, len(s.Episodes))
	for _, e := range s.Episodes {
		ep := Episode{Season: e.Season, Episode: e.Episode, Title: e.Name, Released: e.AirDate,
			Rating: rating(e.VoteAverage, e.VoteCount), Plot: e.Overview, StillURL: poster(e.StillPath, "w780"), TMDBID: e.ID}
		if e.Runtime > 0 {
			rt := e.Runtime
			ep.RuntimeMin = &rt
		}
		for i, c := range e.GuestStars {
			if i >= castLimit {
				break
			}
			ep.Credits = append(ep.Credits, Credit{PersonID: c.ID, Name: c.Name, ProfilePath: c.ProfilePath, Kind: "cast", Role: c.Character, Order: i})
		}
		seen := map[string]bool{}
		for _, c := range e.Crew {
			rank, ok := crewJobs[c.Job]
			key := fmt.Sprintf("%d|%s", c.ID, c.Job)
			if !ok || seen[key] {
				continue
			}
			seen[key] = true
			ep.Credits = append(ep.Credits, Credit{PersonID: c.ID, Name: c.Name, ProfilePath: c.ProfilePath, Kind: "crew", Role: c.Job, Order: 100 + rank})
		}
		out = append(out, ep)
	}
	return out, nil
}

// SearchTitles lists matches for picking one by hand. Each result carries an
// IMDb id when TMDB has one (so pins and IMDb links keep working), else a
// TMDB reference. An IMDb id, a TMDB reference or a themoviedb.org URL is
// looked up directly.
func (t *TMDB) SearchTitles(ctx context.Context, kind Kind, query string, year int) ([]SearchResult, error) {
	if id := firstNonEmpty(reImdbID.FindString(query), TMDBID(query)); id != "" {
		d, err := t.ByImdbID(ctx, kind, id)
		if err != nil || d == nil {
			return []SearchResult{}, err
		}
		return []SearchResult{{ImdbID: d.ImdbID, Title: d.Title, Year: strconv.Itoa(d.Year), Poster: d.PosterURL}}, nil
	}
	rs, err := t.search(ctx, kind, query, year)
	if err == nil && len(rs) == 0 && year > 0 {
		rs, err = t.search(ctx, kind, query, 0)
	}
	if err != nil {
		return nil, err
	}
	out := []SearchResult{}
	for i, r := range rs {
		if i >= tmdbPickerSize {
			break
		}
		// Details are cached and cheap, and give the IMDb id the pin needs.
		d, err := t.details(ctx, kind, r.ID)
		if err != nil {
			return nil, err
		}
		if d == nil {
			continue
		}
		out = append(out, SearchResult{ImdbID: d.ImdbID, Title: d.Title, Year: yearText(r.date()), Poster: poster(r.PosterPath, "w185")})
	}
	return out, nil
}

// SearchSeries lists the series TMDB knows by a title, with the years each
// ran, for telling same-titled shows apart (IdentifySeries). Exact-title
// matches are looked up in full for their end year.
func (t *TMDB) SearchSeries(ctx context.Context, title string) ([]SeriesMatch, error) {
	rs, err := t.search(ctx, Series, title, 0)
	if err != nil {
		return nil, err
	}
	out := make([]SeriesMatch, 0, len(rs))
	for _, r := range rs {
		m := SeriesMatch{ImdbID: fmt.Sprintf("tmdb:tv:%d", r.ID), Title: r.title(), StartYear: yearOf(r.date())}
		if TitleScore(title, r.title()) == 3 {
			var d tmdbDetails
			if err := t.get(ctx, fmt.Sprintf("/tv/%d", r.ID), url.Values{"append_to_response": {"external_ids"}}, tmdbTVTTL, &d); err == nil {
				if d.ExternalIDs.ImdbID != "" {
					m.ImdbID = d.ExternalIDs.ImdbID
				}
				if !d.InProduction {
					m.EndYear = yearOf(d.LastAirDate)
				}
			}
		}
		out = append(out, m)
	}
	return out, nil
}

// --- helpers -------------------------------------------------------------------

func (t *TMDB) search(ctx context.Context, kind Kind, query string, year int) ([]tmdbSearchResult, error) {
	q := url.Values{"query": {query}, "include_adult": {"false"}}
	if year > 0 {
		if kind == Series {
			q.Set("first_air_date_year", strconv.Itoa(year))
		} else {
			q.Set("primary_release_year", strconv.Itoa(year))
		}
	}
	var s tmdbSearch
	ttl := tmdbMovieTTL
	if kind == Series {
		ttl = tmdbTVTTL
	}
	if err := t.get(ctx, "/search/"+tmdbKind(kind), q, ttl, &s); err != nil && !errors.Is(err, errNotFound) {
		return nil, err
	}
	return s.Results, nil
}

// resolve turns an IMDb id or TMDB reference into TMDB's kind and number.
func (t *TMDB) resolve(ctx context.Context, kind Kind, id string) (Kind, int, error) {
	if ref := TMDBID(id); ref != "" {
		parts := strings.Split(ref, ":")
		n, _ := strconv.Atoi(parts[2])
		if parts[1] == "tv" {
			return Series, n, nil
		}
		return Movie, n, nil
	}
	imdb := reImdbID.FindString(id)
	if imdb == "" {
		return kind, 0, nil
	}
	var f tmdbFind
	if err := t.get(ctx, "/find/"+imdb, url.Values{"external_source": {"imdb_id"}}, tmdbMovieTTL, &f); err != nil {
		if errors.Is(err, errNotFound) {
			return kind, 0, nil
		}
		return kind, 0, err
	}
	// Prefer the kind asked for; an IMDb id names exactly one title anyway.
	if kind == Series && len(f.TVResults) > 0 {
		return Series, f.TVResults[0].ID, nil
	}
	if len(f.MovieResults) > 0 {
		return Movie, f.MovieResults[0].ID, nil
	}
	if len(f.TVResults) > 0 {
		return Series, f.TVResults[0].ID, nil
	}
	return kind, 0, nil
}

func (t *TMDB) details(ctx context.Context, kind Kind, id int) (*Details, error) {
	var d tmdbDetails
	extra := "external_ids,release_dates,credits"
	ttl := tmdbMovieTTL
	if kind == Series {
		extra, ttl = "external_ids,content_ratings,aggregate_credits", tmdbTVTTL
	}
	if err := t.get(ctx, fmt.Sprintf("/%s/%d", tmdbKind(kind), id), url.Values{"append_to_response": {extra}}, ttl, &d); err != nil {
		if errors.Is(err, errNotFound) {
			return nil, nil
		}
		return nil, err
	}
	out := &Details{
		ID: fmt.Sprintf("tmdb:%s:%d", tmdbKind(kind), d.ID), Title: firstNonEmpty(d.Title, d.Name),
		Year: yearOf(firstNonEmpty(d.ReleaseDate, d.FirstAirDate)), Plot: d.Overview,
		Rating: rating(d.VoteAverage, d.VoteCount), PosterURL: poster(d.PosterPath, "w780"),
		BackdropURL: poster(d.BackdropPath, "w1280"),
	}
	out.ImdbID = firstNonEmpty(d.ExternalIDs.ImdbID, d.ImdbID, out.ID)
	for _, g := range d.Genres {
		out.Genres = append(out.Genres, g.Name)
	}
	runtime := d.Runtime
	if runtime == 0 && len(d.EpisodeRunTime) > 0 {
		runtime = d.EpisodeRunTime[0]
	}
	if runtime > 0 {
		out.RuntimeMin = &runtime
	}
	if kind == Series && d.NumberOfSeasons > 0 {
		n := d.NumberOfSeasons
		out.TotalSeasons = &n
	}
	out.Rated = usCertification(d)
	out.Credits = d.credits(kind)
	return out, nil
}

// usCertification is the US rating (PG-13, TV-MA), as OMDb gives it.
func usCertification(d tmdbDetails) string {
	for _, r := range d.ReleaseDates.Results {
		if r.Country != "US" {
			continue
		}
		for _, rel := range r.Releases {
			if rel.Certification != "" {
				return rel.Certification
			}
		}
	}
	for _, r := range d.ContentRatings.Results {
		if r.Country == "US" && r.Rating != "" {
			return r.Rating
		}
	}
	return ""
}

func poster(path, size string) string {
	if path == "" {
		return ""
	}
	return tmdbImages + size + path
}

// rating is TMDB's vote average (0-10) to one decimal, when anyone voted.
func rating(avg float64, votes int) *float64 {
	if votes == 0 || avg == 0 {
		return nil
	}
	r := math.Round(avg*10) / 10
	return &r
}

func yearOf(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, _ := strconv.Atoi(date[:4])
	return y
}

// yearText is how the match picker shows a year ("" when unknown).
func yearText(date string) string {
	if y := yearOf(date); y > 0 {
		return strconv.Itoa(y)
	}
	return ""
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// get performs a cached, spaced-out request and decodes into out. Not-found
// answers are cached; bad keys, rate limits and server errors are not.
func (t *TMDB) get(ctx context.Context, path string, q url.Values, ttl time.Duration, out any) error {
	full := url.Values{"language": {t.language}}
	for k, v := range q {
		full[k] = v
	}
	cacheKey := path + "?" + full.Encode() // never holds the key
	body, ok := t.cache.CacheGet(ctx, t.Name(), cacheKey, ttl)
	fresh := false
	if !ok {
		var status int
		var err error
		body, status, err = t.fetch(ctx, path, full)
		if err != nil {
			return err
		}
		switch {
		case status == http.StatusNotFound:
			_ = t.cache.CachePut(ctx, t.Name(), cacheKey, []byte(`{"status_code":34}`))
			return errNotFound
		case status == http.StatusUnauthorized:
			return errors.New("tmdb: the API key was refused (HTTP 401)")
		case status == http.StatusTooManyRequests:
			return errors.New("tmdb: rate limited (HTTP 429); will retry")
		case status != http.StatusOK:
			return fmt.Errorf("tmdb: HTTP %d", status)
		}
		fresh = true
	}
	var probe struct {
		StatusCode int `json:"status_code"`
	}
	if json.Unmarshal(body, &probe) == nil && probe.StatusCode == 34 {
		return errNotFound
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("tmdb: bad response: %w", err)
	}
	// Cached only once it decoded: a captive portal's HTML page or a body
	// cut off at the size cap would otherwise be served for days.
	if fresh {
		_ = t.cache.CachePut(ctx, t.Name(), cacheKey, body)
	}
	return nil
}

func (t *TMDB) fetch(ctx context.Context, path string, q url.Values) ([]byte, int, error) {
	t.mu.Lock()
	wait := time.Until(t.next)
	t.next = time.Now().Add(max(wait, 0) + tmdbSpacing)
	t.mu.Unlock()
	if wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		}
	}
	bearer := strings.HasPrefix(t.key, "eyJ") // a v4 read access token is a JWT
	if !bearer {
		q = cloneValues(q)
		q.Set("api_key", t.key)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.base+path+"?"+q.Encode(), nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	if bearer {
		req.Header.Set("Authorization", "Bearer "+t.key)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("tmdb: %w", redactKey(err, t.key))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return body, resp.StatusCode, err
}

func cloneValues(q url.Values) url.Values {
	out := url.Values{}
	for k, v := range q {
		out[k] = v
	}
	return out
}
