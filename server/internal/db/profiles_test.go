package db

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
)

func TestProfilesSeparateWatchStateAndFavourites(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	bg := context.Background()

	// The migration creates the first profile, which owns existing history.
	ps, err := d.Profiles(bg)
	if err != nil || len(ps) != 1 || ps[0].ID != 1 {
		t.Fatalf("profiles after migrate = %v, %v", ps, err)
	}
	kid, err := d.CreateProfile(bg, "Kid", "pink")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.CreateProfile(bg, "kid", "cyan"); !errors.Is(err, ErrProfileName) {
		t.Fatalf("duplicate name err = %v", err)
	}
	me, them := WithProfile(bg, 1), WithProfile(bg, kid.ID)

	lib, err := d.CreateLibrary(bg, "Movies", "/m", "movies")
	if err != nil {
		t.Fatal(err)
	}
	item, _, err := d.EnsureItem(bg, lib, "movie", "Heat", 1995)
	if err != nil {
		t.Fatal(err)
	}
	fid, err := d.UpsertFile(bg, File{LibraryID: lib, MediaItemID: item, Path: "/m/Heat.mkv"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.SaveProgress(me, fid, 600, 6000); err != nil {
		t.Fatal(err)
	}
	if f, _ := d.File(me, fid); f.PositionSec != 600 {
		t.Fatalf("my position = %v, want 600", f.PositionSec)
	}
	if f, _ := d.File(them, fid); f.PositionSec != 0 || f.Watched {
		t.Fatalf("other profile sees %v/%v", f.PositionSec, f.Watched)
	}
	if cw, _ := d.ContinueWatching(them, 10); len(cw) != 0 {
		t.Fatalf("other profile continue watching = %v", cw)
	}
	if err := d.SetWatched(them, []int64{fid}, true); err != nil {
		t.Fatal(err)
	}
	if items, _ := d.Items(them, "movie"); len(items) != 1 || items[0].WatchedCount != 1 {
		t.Fatalf("their watched count = %+v", items)
	}
	if items, _ := d.Items(me, "movie"); items[0].WatchedCount != 0 {
		t.Fatalf("my watched count = %d, want 0", items[0].WatchedCount)
	}

	if err := d.ReplaceChannels(bg, []Channel{{Number: "2.1", Name: "A", URL: "u"}, {Number: "4.1", Name: "B", URL: "u"}}, func(string) float64 { return 0 }); err != nil {
		t.Fatal(err)
	}
	if err := d.SetChannelPinned(them, "4.1", true); err != nil {
		t.Fatal(err)
	}
	if err := d.SetChannelPinned(them, "9.9", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("pin unknown channel err = %v", err)
	}
	if cs, _ := d.Channels(them); cs[0].Number != "4.1" || !cs[0].Pinned {
		t.Fatalf("their channels = %+v", cs)
	}
	if cs, _ := d.Channels(me); cs[0].Pinned || cs[1].Pinned {
		t.Fatalf("my channels = %+v", cs)
	}

	p, err := d.MergeProfilePrefs(bg, kid.ID, json.RawMessage(`{"theme":"light","autoplayNext":false}`))
	if err != nil {
		t.Fatal(err)
	}
	if p, err = d.MergeProfilePrefs(bg, kid.ID, json.RawMessage(`{"theme":null,"liveHeight":480}`)); err != nil {
		t.Fatal(err)
	}
	var prefs map[string]any
	_ = json.Unmarshal(p.Prefs, &prefs)
	if _, ok := prefs["theme"]; ok || prefs["autoplayNext"] != false || prefs["liveHeight"] != float64(480) {
		t.Fatalf("merged prefs = %s", p.Prefs)
	}
	if _, err := d.MergeProfilePrefs(bg, kid.ID, json.RawMessage(`[1]`)); !errors.Is(err, ErrPrefsNotAnObj) {
		t.Fatalf("array prefs err = %v", err)
	}

	if err := d.DeleteProfile(bg, kid.ID); err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteProfile(bg, 1); !errors.Is(err, ErrLastProfile) {
		t.Fatalf("delete last profile err = %v", err)
	}
}
