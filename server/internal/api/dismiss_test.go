package api

import (
	"context"
	"strconv"
	"testing"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/transcode"
	"github.com/timothydodd/couchside/internal/worker"
)

// "Not a commercial" hides a break for everyone, survives detection running
// again (with edges a little different), can be undone, and is admin-only.
func TestNotACommercial(t *testing.T) {
	s, ts, admin := passwordlessServer(t)
	s.worker = worker.New(s.db, config.Config{}, nil, transcode.Encoder{})
	ctx := context.Background()
	lib, _ := s.db.CreateLibrary(ctx, "TV", "/tv", "tv")
	item, _, _ := s.db.EnsureItem(ctx, lib, "series", "Show", 0)
	ep, _ := s.db.EnsureEpisode(ctx, item, 1, 1, "", "")
	fid, err := s.db.UpsertFile(ctx, db.File{LibraryID: lib, MediaItemID: item, EpisodeID: &ep, Path: "/tv/Show/S01E01.ts", Size: 1, Mtime: 1}, 1)
	if err != nil {
		t.Fatal(err)
	}
	detect := func(segs ...db.Segment) {
		if err := s.db.SetCommercials(ctx, fid, 1, 1, segs); err != nil {
			t.Fatal(err)
		}
	}
	// A lead-in break, then two breaks far apart (no merging, 1s trims).
	detect(db.Segment{Start: 0, End: 120}, db.Segment{Start: 300, End: 400}, db.Segment{Start: 700, End: 900})
	url := "/api/files/" + strconv.FormatInt(fid, 10) + "/commercials"
	breaks := func(c *testClient) []db.Segment {
		var out struct{ Segments []db.Segment }
		if code := c.do("GET", url, nil, &out); code != 200 {
			t.Fatalf("GET commercials = %d", code)
		}
		return out.Segments
	}
	shown := breaks(admin)
	if len(shown) != 3 {
		t.Fatalf("breaks = %v", shown)
	}
	wrong := shown[1] // as the player shows it: 301-399

	if code := admin.do("POST", url+"/dismissed", map[string]any{"start": wrong.Start, "end": wrong.End}, nil); code != 204 {
		t.Fatalf("dismiss = %d", code)
	}
	if got := breaks(admin); len(got) != 2 || got[1].Start != 701 {
		t.Fatalf("after dismissing: %v", got)
	}
	// Detection runs again and finds the same false break a few seconds off.
	detect(db.Segment{Start: 0, End: 120}, db.Segment{Start: 296, End: 404}, db.Segment{Start: 700, End: 900})
	if got := breaks(admin); len(got) != 2 {
		t.Fatalf("after detection ran again: %v", got)
	}
	// Undo.
	if code := admin.do("POST", url+"/dismissed", map[string]any{"start": wrong.Start, "end": wrong.End, "restore": true}, nil); code != 204 {
		t.Fatalf("restore = %d", code)
	}
	if got := breaks(admin); len(got) != 3 {
		t.Fatalf("after restore: %v", got)
	}

	// Someone who isn't an admin can see breaks but not change them.
	var kid db.Profile
	admin.do("POST", "/api/accounts", map[string]any{"name": "Kid"}, &kid)
	k := newClient(t, ts.URL)
	k.do("POST", "/api/auth/pick", map[string]any{"profileId": kid.ID}, nil)
	if code := k.do("POST", url+"/dismissed", map[string]any{"start": 1, "end": 119}, nil); code != 403 {
		t.Fatalf("non-admin dismiss = %d, want 403", code)
	}
}
