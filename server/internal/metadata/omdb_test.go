package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type memCache struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (c *memCache) CacheGet(_ context.Context, p, k string, _ time.Duration) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.m[p+k]
	return b, ok
}

func (c *memCache) CachePut(_ context.Context, p, k string, b []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[p+k] = b
	return nil
}

func TestOMDbLookupFallsBackToSearch(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		q := r.URL.Query()
		if q.Get("apikey") != "k" {
			t.Errorf("missing api key")
		}
		switch {
		case q.Get("t") != "":
			w.Write([]byte(`{"Response":"False","Error":"Movie not found!"}`))
		case q.Get("s") != "":
			w.Write([]byte(`{"Response":"True","Search":[{"imdbID":"tt9999999","Title":"Spirited Away: Making Of","Year":"2002"},{"imdbID":"tt0245429","Title":"Spirited Away","Year":"2001"}]}`))
		case q.Get("i") == "tt0245429":
			w.Write([]byte(`{"Response":"True","Title":"Spirited Away","Year":"2001","Rated":"PG","Runtime":"125 min",
				"Genre":"Animation, Adventure, Family","Plot":"N/A","imdbRating":"8.6","imdbID":"tt0245429",
				"Poster":"https://m.media-amazon.com/images/M/abc._V1_SX300.jpg"}`))
		default:
			t.Errorf("unexpected query %v", q)
		}
	}))
	defer srv.Close()

	o := NewOMDb("k", &memCache{m: map[string][]byte{}})
	o.base = srv.URL + "/"
	d, err := o.Lookup(context.Background(), Movie, "Spirited Away", 2001)
	if err != nil || d == nil {
		t.Fatalf("lookup: %v %v", d, err)
	}
	if d.Title != "Spirited Away" || d.Year != 2001 || d.Plot != "" || *d.Rating != 8.6 || *d.RuntimeMin != 125 || len(d.Genres) != 3 {
		t.Errorf("bad details: %+v", d)
	}
	if d.PosterURL != "https://m.media-amazon.com/images/M/abc._V1_SX800.jpg" {
		t.Errorf("poster not upscaled: %s", d.PosterURL)
	}
	// t+y, t, s, i requests; a second identical lookup must come entirely from cache.
	first := hits
	if _, err := o.Lookup(context.Background(), Movie, "Spirited Away", 2001); err != nil {
		t.Fatal(err)
	}
	if hits != first {
		t.Errorf("second lookup made %d network calls, want 0", hits-first)
	}
	// An unrelated title must not be accepted from search results.
	if d, err := o.Lookup(context.Background(), Movie, "Sen to Chihiro", 2001); err != nil || d != nil {
		t.Errorf("unrelated search hit accepted: %v %v", d, err)
	}
}

func TestOMDbBadKeyIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"Response":"False","Error":"Invalid API key!"}`))
	}))
	defer srv.Close()
	o := NewOMDb("bad", &memCache{m: map[string][]byte{}})
	o.base = srv.URL + "/"
	if d, err := o.Lookup(context.Background(), Movie, "Heat", 1995); err == nil || d != nil {
		t.Fatalf("want error, got %v %v", d, err)
	}
}

func TestOMDbSearchTitles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("s") == "Heat" && q.Get("y") == "1996":
			w.Write([]byte(`{"Response":"False","Error":"Movie not found!"}`))
		case q.Get("s") == "Heat":
			w.Write([]byte(`{"Response":"True","Search":[{"imdbID":"tt0113277","Title":"Heat","Year":"1995","Poster":"https://x/heat.jpg"},{"imdbID":"tt0093164","Title":"Heat","Year":"1986","Poster":"N/A"}]}`))
		case q.Get("i") == "tt0113277":
			w.Write([]byte(`{"Response":"True","Title":"Heat","Year":"1995","imdbID":"tt0113277","Poster":"N/A"}`))
		default:
			t.Errorf("unexpected query %v", q)
		}
	}))
	defer srv.Close()
	o := NewOMDb("k", &memCache{m: map[string][]byte{}})
	o.base = srv.URL + "/"

	// Nothing for the year: search again without it.
	res, err := o.SearchTitles(context.Background(), Movie, "Heat", 1996)
	if err != nil || len(res) != 2 || res[0].ImdbID != "tt0113277" || res[0].Poster != "https://x/heat.jpg" || res[1].Poster != "" {
		t.Fatalf("search = %+v, %v", res, err)
	}
	// An IMDb URL is looked up directly.
	res, err = o.SearchTitles(context.Background(), Movie, "https://www.imdb.com/title/tt0113277/", 0)
	if err != nil || len(res) != 1 || res[0].Title != "Heat" || res[0].Year != "1995" {
		t.Fatalf("by id = %+v, %v", res, err)
	}
}
