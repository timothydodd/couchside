package db

import (
	"context"
	"fmt"
	"testing"
)

// Home's Continue Watching row: what's in progress, the next episode after
// one that was finished, and removing a title until it's watched again.
func TestContinueWatchingAndNextUp(t *testing.T) {
	d := openTest(t)
	bg := context.Background()
	me, _ := d.SetupAdmin(bg, "Me", "hash")
	ctx := WithProfile(bg, me.ID)
	other, _ := d.CreateAccount(bg, "Other", "accent", "user", false, "h", false)

	lib, _ := d.CreateLibrary(bg, "TV", "/tv", "tv")
	films, _ := d.CreateLibrary(bg, "Films", "/films", "movies")
	show, _, _ := d.EnsureItem(bg, lib, "series", "Ghosts", 2021)
	dur := 1800.0
	var eps []int64
	for i := 1; i <= 4; i++ {
		ep, _ := d.EnsureEpisode(bg, show, 1, i, "", "")
		f, err := d.UpsertFile(bg, File{LibraryID: lib, MediaItemID: show, EpisodeID: &ep, Path: fmt.Sprintf("/tv/Ghosts/S01E%02d.mkv", i), Size: 1, Mtime: 1, DurationSec: &dur}, 1)
		if err != nil {
			t.Fatal(err)
		}
		eps = append(eps, f)
	}
	film, _, _ := d.EnsureItem(bg, films, "movie", "Heat", 1995)
	heat, _ := d.UpsertFile(bg, File{LibraryID: films, MediaItemID: film, Path: "/films/Heat.mkv", Size: 1, Mtime: 1, DurationSec: &dur}, 1)

	row := func() []PlayInfo {
		t.Helper()
		out, err := d.ContinueWatching(ctx, 20)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	at := func(file int64, when int64) { // watch_state times are whole seconds: set them apart
		t.Helper()
		if _, err := d.sql.ExecContext(bg, `UPDATE watch_state SET updated_at = ? WHERE profile_id = ? AND file_id = ?`, when, me.ID, file); err != nil {
			t.Fatal(err)
		}
	}
	if got := row(); len(got) != 0 {
		t.Fatalf("nothing watched yet: %+v", got)
	}

	// Finish episode 1: episode 2 is next up.
	if err := d.SaveProgress(ctx, eps[0], 1790, 1800); err != nil {
		t.Fatal(err)
	}
	at(eps[0], 1000)
	got := row()
	if len(got) != 1 || got[0].FileID != eps[1] || !got[0].NextUp {
		t.Fatalf("after finishing episode 1: %+v", got)
	}
	// Part way through a film, more recently: it comes first.
	if err := d.SaveProgress(ctx, heat, 600, 1800); err != nil {
		t.Fatal(err)
	}
	at(heat, 2000)
	got = row()
	if len(got) != 2 || got[0].FileID != heat || got[0].NextUp || got[1].FileID != eps[1] {
		t.Fatalf("film in progress, then next up: %+v", got)
	}
	// Start episode 2: it's in the row as in progress, not twice.
	if err := d.SaveProgress(ctx, eps[1], 300, 1800); err != nil {
		t.Fatal(err)
	}
	at(eps[1], 3000)
	got = row()
	if len(got) != 2 || got[0].FileID != eps[1] || got[0].NextUp {
		t.Fatalf("episode 2 in progress: %+v", got)
	}
	// Another profile sees none of it.
	if o, _ := d.ContinueWatching(WithProfile(bg, other.ID), 20); len(o) != 0 {
		t.Fatalf("another profile's row: %+v", o)
	}

	// Remove the show from the row; the film stays, and so does the resume point.
	if err := d.HideFromHome(ctx, show); err != nil {
		t.Fatal(err)
	}
	if _, err := d.sql.ExecContext(bg, `UPDATE home_hidden SET hidden_at = 3500`); err != nil {
		t.Fatal(err)
	}
	got = row()
	if len(got) != 1 || got[0].FileID != heat {
		t.Fatalf("after removing the show: %+v", got)
	}
	if p, _ := d.PlayInfo(ctx, eps[1]); p.PositionSec != 300 {
		t.Fatalf("removing from the row lost the resume point: %v", p.PositionSec)
	}
	// Watching it again brings it back.
	if err := d.SaveProgress(ctx, eps[1], 1790, 1800); err != nil {
		t.Fatal(err)
	}
	at(eps[1], 4000)
	got = row()
	if len(got) != 2 || got[0].FileID != eps[2] || !got[0].NextUp {
		t.Fatalf("after finishing episode 2: %+v", got)
	}
	// Caught up: nothing is next.
	if err := d.SetWatched(ctx, eps[2:], true); err != nil {
		t.Fatal(err)
	}
	if got = row(); len(got) != 1 || got[0].FileID != heat {
		t.Fatalf("caught up: %+v", got)
	}
}
