package metadata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeTMDB answers the handful of endpoints the provider uses.
func fakeTMDB(t *testing.T, hits *[]string) *TMDB {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		*hits = append(*hits, r.URL.Path+"?"+q.Get("query")+q.Get("primary_release_year")+q.Get("first_air_date_year"))
		if q.Get("api_key") != "k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/search/movie" && q.Get("query") == "The Matrix":
			// The documentary comes first, as TMDB's ranking sometimes does.
			w.Write([]byte(`{"results":[
				{"id":11111,"title":"The Matrix Revisited","release_date":"2001-11-19"},
				{"id":603,"title":"The Matrix","release_date":"1999-03-30","poster_path":"/m.jpg"}]}`))
		case r.URL.Path == "/movie/603":
			if !strings.Contains(q.Get("append_to_response"), "external_ids") {
				t.Errorf("movie details without external ids: %v", q)
			}
			w.Write([]byte(`{"id":603,"title":"The Matrix","release_date":"1999-03-30","overview":"A hacker learns the truth.",
				"runtime":136,"vote_average":8.217,"vote_count":26000,"poster_path":"/m.jpg","backdrop_path":"/b.jpg",
				"genres":[{"name":"Action"},{"name":"Science Fiction"}],"external_ids":{"imdb_id":"tt0133093"},
				"release_dates":{"results":[{"iso_3166_1":"GB","release_dates":[{"certification":"15"}]},
					{"iso_3166_1":"US","release_dates":[{"certification":""},{"certification":"R"}]}]}}`))
		case r.URL.Path == "/find/tt0944947":
			w.Write([]byte(`{"movie_results":[],"tv_results":[{"id":1399}]}`))
		case r.URL.Path == "/tv/1399":
			w.Write([]byte(`{"id":1399,"name":"Game of Thrones","first_air_date":"2011-04-17","last_air_date":"2019-05-19",
				"in_production":false,"episode_run_time":[60],"number_of_seasons":8,"vote_average":8.4,"vote_count":20000,
				"external_ids":{"imdb_id":"tt0944947"},"content_ratings":{"results":[{"iso_3166_1":"US","rating":"TV-MA"}]}}`))
		case r.URL.Path == "/tv/1399/season/1":
			w.Write([]byte(`{"episodes":[{"id":63056,"episode_number":1,"season_number":1,"name":"Winter Is Coming","air_date":"2011-04-17","vote_average":7.9,"vote_count":300,
				"overview":"Ned Stark is summoned.","runtime":62,"still_path":"/w1.jpg",
				"guest_stars":[{"id":1,"name":"Joseph Mawle","character":"Benjen Stark","profile_path":"/jm.jpg","order":0}],
				"crew":[{"id":2,"name":"Tim Van Patten","job":"Director"},{"id":3,"name":"David Benioff","job":"Writer"},{"id":4,"name":"Someone","job":"Gaffer"}]},
				{"episode_number":2,"season_number":1,"name":"The Kingsroad","air_date":"2011-04-24","vote_average":0,"vote_count":0}]}`))
		case r.URL.Path == "/tv/77777":
			w.Write([]byte(`{"id":77777,"name":"Obscure Show","first_air_date":"2020-01-01","in_production":true,"external_ids":{"imdb_id":null}}`))
		case r.URL.Path == "/search/tv" && q.Get("query") == "MacGyver":
			w.Write([]byte(`{"results":[{"id":2,"name":"MacGyver","first_air_date":"2016-09-23"},{"id":1,"name":"MacGyver","first_air_date":"1985-09-29"}]}`))
		case r.URL.Path == "/tv/2":
			w.Write([]byte(`{"id":2,"name":"MacGyver","first_air_date":"2016-09-23","last_air_date":"2021-04-30","in_production":false,"external_ids":{"imdb_id":"tt1399045"}}`))
		case r.URL.Path == "/tv/1":
			w.Write([]byte(`{"id":1,"name":"MacGyver","first_air_date":"1985-09-29","last_air_date":"1992-05-21","in_production":false,"external_ids":{"imdb_id":"tt0088559"}}`))
		case r.URL.Path == "/movie/404":
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"status_code":34,"status_message":"The resource you requested could not be found."}`))
		default:
			w.Write([]byte(`{"results":[]}`))
		}
	}))
	t.Cleanup(srv.Close)
	p := NewTMDB("k", &memCache{m: map[string][]byte{}})
	p.base = srv.URL
	return p
}

func TestTMDBLookupMovie(t *testing.T) {
	var hits []string
	p := fakeTMDB(t, &hits)
	d, err := p.Lookup(context.Background(), Movie, "The Matrix", 1999)
	if err != nil || d == nil {
		t.Fatalf("lookup = %+v, %v", d, err)
	}
	if d.ImdbID != "tt0133093" || d.Title != "The Matrix" || d.Year != 1999 || d.Rated != "R" {
		t.Errorf("details = %+v", d)
	}
	if d.Rating == nil || *d.Rating != 8.2 || d.RuntimeMin == nil || *d.RuntimeMin != 136 {
		t.Errorf("rating/runtime = %v/%v", d.Rating, d.RuntimeMin)
	}
	if d.PosterURL != "https://image.tmdb.org/t/p/w780/m.jpg" || d.BackdropURL != "https://image.tmdb.org/t/p/w1280/b.jpg" {
		t.Errorf("art = %q %q", d.PosterURL, d.BackdropURL)
	}
	if len(d.Genres) != 2 {
		t.Errorf("genres = %v", d.Genres)
	}
	// A second lookup is served from the cache.
	n := len(hits)
	if _, err := p.Lookup(context.Background(), Movie, "The Matrix", 1999); err != nil || len(hits) != n {
		t.Errorf("cached lookup made %d more requests (%v)", len(hits)-n, err)
	}
}

func TestTMDBByImdbIDAndSeason(t *testing.T) {
	var hits []string
	p := fakeTMDB(t, &hits)
	ctx := context.Background()
	d, err := p.ByImdbID(ctx, Series, "tt0944947")
	if err != nil || d == nil || d.Title != "Game of Thrones" || d.Rated != "TV-MA" || d.TotalSeasons == nil || *d.TotalSeasons != 8 {
		t.Fatalf("by imdb = %+v, %v", d, err)
	}
	eps, err := p.Season(ctx, "tt0944947", 1)
	if err != nil || len(eps) != 2 || eps[0].Title != "Winter Is Coming" || eps[0].Released != "2011-04-17" {
		t.Fatalf("season = %+v, %v", eps, err)
	}
	if eps[1].Rating != nil {
		t.Errorf("an unrated episode got rating %v", *eps[1].Rating)
	}
	e := eps[0]
	if e.Plot != "Ned Stark is summoned." || e.RuntimeMin == nil || *e.RuntimeMin != 62 || e.StillURL != tmdbImages+"w780/w1.jpg" || e.TMDBID != 63056 {
		t.Errorf("episode details = %+v", e)
	}
	// Guest stars, then the director and writer; a gaffer isn't a key job.
	if len(e.Credits) != 3 || e.Credits[0].Role != "Benjen Stark" || e.Credits[0].Kind != "cast" || e.Credits[1].Role != "Director" || e.Credits[2].Role != "Writer" {
		t.Errorf("episode credits = %+v", e.Credits)
	}
	if eps[1].RuntimeMin != nil || eps[1].StillURL != "" || len(eps[1].Credits) != 0 {
		t.Errorf("an episode TMDB knows little about = %+v", eps[1])
	}
	// Titles TMDB knows no IMDb id for keep a TMDB reference, which works everywhere an IMDb id does.
	d, err = p.ByImdbID(ctx, Series, "tmdb:tv:77777")
	if err != nil || d == nil || d.ImdbID != "tmdb:tv:77777" {
		t.Fatalf("tmdb ref = %+v, %v", d, err)
	}
	if d, err := p.ByImdbID(ctx, Movie, "tmdb:movie:404"); d != nil || err != nil {
		t.Fatalf("missing title = %+v, %v; want nil, nil", d, err)
	}
}

func TestTMDBSearchSeriesYears(t *testing.T) {
	var hits []string
	p := fakeTMDB(t, &hits)
	ms, err := p.SearchSeries(context.Background(), "MacGyver")
	if err != nil || len(ms) != 2 {
		t.Fatalf("series = %+v, %v", ms, err)
	}
	if ms[0].ImdbID != "tt1399045" || ms[0].StartYear != 2016 || ms[0].EndYear != 2021 {
		t.Errorf("2016 series = %+v", ms[0])
	}
	if ms[1].StartYear != 1985 || ms[1].EndYear != 1992 {
		t.Errorf("1985 series = %+v", ms[1])
	}
}

func TestTMDBBadKeyIsNotCached(t *testing.T) {
	var hits []string
	p := fakeTMDB(t, &hits)
	p.key = "wrong"
	if _, err := p.Lookup(context.Background(), Movie, "The Matrix", 1999); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v", err)
	}
	// A new key comes with a restart, which starts a provider without the
	// wait a refused key sets.
	p.key = "k"
	p.keyAccepted()
	if d, err := p.Lookup(context.Background(), Movie, "The Matrix", 1999); err != nil || d == nil {
		t.Fatalf("after fixing the key: %+v, %v", d, err)
	}
}

func TestTMDBID(t *testing.T) {
	for in, want := range map[string]string{
		"tmdb:movie:603": "tmdb:movie:603",
		"https://www.themoviedb.org/tv/1399-game-of-thrones": "tmdb:tv:1399",
		"tt0133093": "",
	} {
		if got := TMDBID(in); got != want {
			t.Errorf("TMDBID(%q) = %q, want %q", in, got, want)
		}
	}
}

// A captive portal answers 200 with HTML: that mustn't be cached as TMDB's answer.
func TestTMDBUndecodableResponseIsNotCached(t *testing.T) {
	portal := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if portal {
			w.Write([]byte("<html><body>Sign in to the hotel Wi-Fi</body></html>"))
			return
		}
		w.Write([]byte(`{"results":[{"id":603,"title":"The Matrix","release_date":"1999-03-30"}]}`))
	}))
	defer srv.Close()
	cache := &memCache{m: map[string][]byte{}}
	p := NewTMDB("k", cache)
	p.base = srv.URL
	if _, err := p.SearchTitles(context.Background(), Movie, "The Matrix", 1999); err == nil {
		t.Fatal("an HTML page decoded as a TMDB response")
	}
	if len(cache.m) != 0 {
		t.Fatalf("the portal page was cached: %d entries", len(cache.m))
	}
	portal = false
	if res, err := p.SearchTitles(context.Background(), Movie, "The Matrix", 1999); err != nil || len(res) == 0 {
		t.Fatalf("after the portal: %v, %v", res, err)
	}
}

// A refused key isn't asked again until the wait is over, says so through
// Degraded, and works again as soon as TMDB answers.
func TestTMDBBadKeyBacksOff(t *testing.T) {
	calls := 0
	refuse := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if refuse {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"results":[{"id":603,"title":"The Matrix","release_date":"1999-03-30"}]}`))
	}))
	defer srv.Close()
	p := NewTMDB("k", &memCache{m: map[string][]byte{}})
	p.base = srv.URL
	ctx := context.Background()
	if _, err := p.SearchTitles(ctx, Movie, "The Matrix", 1999); !errors.Is(err, ErrBadKey) {
		t.Fatalf("first refusal = %v, want ErrBadKey", err)
	}
	if bad, why := p.Degraded(); !bad || why == "" {
		t.Fatal("not degraded after a 401")
	}
	if _, err := p.SearchTitles(ctx, Movie, "Heat", 1995); !errors.Is(err, ErrBadKey) || calls != 1 {
		t.Fatalf("second lookup = %v after %d calls; want ErrBadKey without asking TMDB", err, calls)
	}
	chain := &Chain{Providers: []Provider{p}}
	if got := chain.Degraded(); len(got) != 1 {
		t.Fatalf("chain degraded = %v", got)
	}
	// The wait runs out and TMDB accepts the key again.
	p.mu.Lock()
	p.badKeyUntil = time.Now().Add(-time.Second)
	p.mu.Unlock()
	refuse = false
	if _, err := p.SearchTitles(ctx, Movie, "The Matrix", 1999); err != nil {
		t.Fatalf("after the wait: %v", err)
	}
	if bad, _ := p.Degraded(); bad || len(chain.Degraded()) != 0 {
		t.Fatal("still degraded after TMDB answered")
	}
	p.keyRefused()
	p.keyRefused()
	p.mu.Lock()
	backoff := p.badKeyBackoff
	p.mu.Unlock()
	if backoff != 2*badKeyFirstWait {
		t.Fatalf("second refusal waits %v, want %v", backoff, 2*badKeyFirstWait)
	}
}
