package api

import (
	"context"
	"fmt"
	"testing"

	"github.com/timothydodd/couchside/internal/db"
)

// A profile limited to one library and a PG rating can't list, open, search
// for or play anything outside them. An admin, and an unlimited profile, see
// everything.
func TestProfileLibraryAndRatingLimits(t *testing.T) {
	s, ts, admin := passwordlessServer(t)
	ctx := context.Background()
	kids, _ := s.db.CreateLibrary(ctx, "Kids", "/kids", "movies")
	films, _ := s.db.CreateLibrary(ctx, "Films", "/films", "movies")
	dur := 6000.0
	add := func(lib int64, title, rated string) (item, file int64) {
		t.Helper()
		item, _, _ = s.db.EnsureItem(ctx, lib, "movie", title, 2000)
		if rated != "" {
			if err := s.db.ApplyMetadata(ctx, item, db.Metadata{Title: title, Year: 2000, Rated: rated, ImdbID: "tt" + title}); err != nil {
				t.Fatal(err)
			}
		}
		file, err := s.db.UpsertFile(ctx, db.File{LibraryID: lib, MediaItemID: item, Path: fmt.Sprintf("/x/%s.mkv", title), Size: 1, Mtime: 1, DurationSec: &dur}, 1)
		if err != nil {
			t.Fatal(err)
		}
		return item, file
	}
	okItem, okFile := add(kids, "Bluey", "TV-Y")
	_, _ = add(kids, "Coraline", "PG")
	tooOld, tooOldFile := add(kids, "Jaws", "PG-13")
	unrated, _ := add(kids, "HomeVideo", "")
	otherLib, otherFile := add(films, "Babe", "G")

	var kid struct {
		ID        int64
		Libraries []int64
		MaxRating string
	}
	if code := admin.do("POST", "/api/accounts", map[string]any{"name": "Kid", "libraries": []int64{kids}, "maxRating": "PG"}, &kid); code != 201 {
		t.Fatalf("create = %d", code)
	}
	if len(kid.Libraries) != 1 || kid.MaxRating != "PG" {
		t.Fatalf("created account: %+v", kid)
	}
	c := newClient(t, ts.URL)
	if code := c.do("POST", "/api/auth/pick", map[string]any{"profileId": kid.ID}, nil); code != 200 {
		t.Fatalf("pick = %d", code)
	}
	titles := func(cl *testClient) map[string]bool {
		t.Helper()
		var items []struct{ Title string }
		if code := cl.do("GET", "/api/items?kind=movie", nil, &items); code != 200 {
			t.Fatalf("items = %d", code)
		}
		out := map[string]bool{}
		for _, i := range items {
			out[i.Title] = true
		}
		return out
	}
	if got := titles(c); len(got) != 2 || !got["Bluey"] || !got["Coraline"] {
		t.Fatalf("the limited profile lists %v, want Bluey and Coraline", got)
	}
	if got := titles(admin); len(got) != 5 {
		t.Fatalf("the admin lists %v, want all five", got)
	}
	get := func(path string) int { return c.do("GET", path, nil, nil) }
	if code := get(fmt.Sprintf("/api/items/%d", okItem)); code != 200 {
		t.Fatalf("an allowed title = %d", code)
	}
	if code := get(fmt.Sprintf("/api/files/%d", okFile)); code != 200 {
		t.Fatalf("an allowed file = %d", code)
	}
	for what, path := range map[string]string{
		"a title rated above the limit": fmt.Sprintf("/api/items/%d", tooOld),
		"its file":                      fmt.Sprintf("/api/files/%d", tooOldFile),
		"its stream":                    fmt.Sprintf("/api/files/%d/stream", tooOldFile),
		"its tracks":                    fmt.Sprintf("/api/files/%d/streams", tooOldFile),
		"an unrated title":              fmt.Sprintf("/api/items/%d", unrated),
		"a title in another library":    fmt.Sprintf("/api/items/%d", otherLib),
		"a file in another library":     fmt.Sprintf("/api/files/%d", otherFile),
	} {
		if code := get(path); code != 404 {
			t.Errorf("%s: %d, want 404", what, code)
		}
	}
	if code := c.do("POST", fmt.Sprintf("/api/files/%d/hls", tooOldFile), map[string]any{"height": 720}, nil); code != 404 {
		t.Errorf("starting a stream of a hidden file = %d, want 404", code)
	}
	var found struct{ Movies []struct{ Title string } }
	if code := c.do("GET", "/api/search?q=jaws", nil, &found); code != 200 || len(found.Movies) != 0 {
		t.Errorf("search for a hidden title = %d %+v", code, found)
	}
	var home struct{ RecentMovies []struct{ Title string } }
	if code := c.do("GET", "/api/home", nil, &home); code != 200 || len(home.RecentMovies) != 2 {
		t.Errorf("home = %d %+v", code, home)
	}

	// Lifting the limits takes effect at once (the session cache is dropped).
	if code := admin.do("PUT", fmt.Sprintf("/api/accounts/%d", kid.ID), map[string]any{"name": "Kid", "role": "user"}, nil); code != 200 {
		t.Fatalf("update = %d", code)
	}
	if got := titles(c); len(got) != 5 {
		t.Fatalf("after lifting the limits: %v", got)
	}
	// An admin account can't be given limits.
	var a struct {
		Libraries []int64
		MaxRating string
	}
	if code := admin.do("PUT", "/api/accounts/1", map[string]any{"name": "Me", "role": "admin", "libraries": []int64{kids}, "maxRating": "G"}, &a); code != 200 || len(a.Libraries) != 0 || a.MaxRating != "" {
		t.Fatalf("limits on an admin: %d %+v", code, a)
	}
	if code := admin.do("PUT", fmt.Sprintf("/api/accounts/%d", kid.ID), map[string]any{"name": "Kid", "maxRating": "X"}, nil); code != 400 {
		t.Fatalf("an unknown rating = %d, want 400", code)
	}
}
