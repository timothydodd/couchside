package db

import (
	"context"
	"fmt"
	"path/filepath"
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

// "My list" is per profile, newest first, and shows on a title's summary.
func TestWatchlist(t *testing.T) {
	d := openTest(t)
	bg := context.Background()
	me, _ := d.SetupAdmin(bg, "Me", "hash")
	other, _ := d.CreateAccount(bg, "Other", "accent", "user", false, "h", false)
	mine, theirs := WithProfile(bg, me.ID), WithProfile(bg, other.ID)
	lib, _ := d.CreateLibrary(bg, "Films", "/films", "movies")
	heat, _, _ := d.EnsureItem(bg, lib, "movie", "Heat", 1995)
	ronin, _, _ := d.EnsureItem(bg, lib, "movie", "Ronin", 1998)

	for i, id := range []int64{heat, ronin} {
		if err := d.SetWatchlist(mine, id, true); err != nil {
			t.Fatal(err)
		}
		if _, err := d.sql.ExecContext(bg, `UPDATE profile_items SET added_at = ? WHERE item_id = ?`, 100+i, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.SetWatchlist(mine, heat, true); err != nil { // twice is fine
		t.Fatal(err)
	}
	list, err := d.Watchlist(mine, 10)
	if err != nil || len(list) != 2 || list[0].ID != ronin || !list[0].InWatchlist {
		t.Fatalf("my list = %+v %v", list, err)
	}
	if l, _ := d.Watchlist(theirs, 10); len(l) != 0 {
		t.Fatalf("another profile's list: %+v", l)
	}
	if it, _ := d.Item(theirs, heat); it.InWatchlist {
		t.Fatal("a title on my list shows as on someone else's")
	}
	if err := d.SetWatchlist(mine, ronin, false); err != nil {
		t.Fatal(err)
	}
	if list, _ := d.Watchlist(mine, 10); len(list) != 1 || list[0].ID != heat {
		t.Fatalf("after removing one: %+v", list)
	}
}

// A database that already has data is copied to backups/ before a new
// migration changes it; a brand-new one isn't.
func TestBackupBeforeUpgrade(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/couchside.db"
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.SetupAdmin(context.Background(), "Me", "hash"); err != nil {
		t.Fatal(err)
	}
	copies := func() []string {
		got, _ := filepath.Glob(dir + "/" + BackupDir + "/couchside-upgrade-*.db")
		return got
	}
	if got := copies(); len(got) != 0 {
		t.Fatalf("a new database was backed up: %v", got)
	}
	d.Close()
	// The next release brings a new migration.
	withExtraMigration(t, "9998_next_release.sql", "CREATE TABLE next_release (x INTEGER);")
	if d, err = Open(path); err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	got := copies()
	if len(got) != 1 {
		t.Fatalf("upgrade backups = %v, want one", got)
	}
	if err := Check(got[0]); err != nil {
		t.Fatalf("the upgrade backup can't be read: %v", err)
	}
	old, err := Open(got[0]) // it has the data from before
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	if ps, _ := old.Profiles(context.Background()); len(ps) != 1 {
		t.Fatalf("profiles in the backup: %+v", ps)
	}
}

// A profile's chosen copy of a title is remembered, per profile, and only a
// file of that title can be chosen.
func TestPreferredVersion(t *testing.T) {
	d := openTest(t)
	bg := context.Background()
	me, _ := d.SetupAdmin(bg, "Me", "hash")
	other, _ := d.CreateAccount(bg, "Other", "accent", "user", false, "h", false)
	mine, theirs := WithProfile(bg, me.ID), WithProfile(bg, other.ID)
	lib, _ := d.CreateLibrary(bg, "Films", "/films", "movies")
	aliens, _, _ := d.EnsureItem(bg, lib, "movie", "Aliens", 1986)
	heat, _, _ := d.EnsureItem(bg, lib, "movie", "Heat", 1995)
	theatrical, _ := d.UpsertFile(bg, File{LibraryID: lib, MediaItemID: aliens, Path: "/films/Aliens (1986).mkv", Size: 2, Mtime: 1}, 1)
	extended, _ := d.UpsertFile(bg, File{LibraryID: lib, MediaItemID: aliens, Path: "/films/Aliens (1986) Extended.mkv", Size: 1, Mtime: 1}, 1)
	heatFile, _ := d.UpsertFile(bg, File{LibraryID: lib, MediaItemID: heat, Path: "/films/Heat (1995).mkv", Size: 1, Mtime: 1}, 1)
	if err := d.SetFileEdition(bg, extended, "Extended"); err != nil {
		t.Fatal(err)
	}
	if v, err := d.PreferredVersion(mine, aliens); err != nil || v != 0 {
		t.Fatalf("before choosing: %d %v", v, err)
	}
	if err := d.SetPreferredVersion(mine, aliens, extended); err != nil {
		t.Fatal(err)
	}
	if err := d.SetPreferredVersion(mine, aliens, heatFile); err == nil {
		t.Fatal("chose another title's file as a version")
	}
	if v, _ := d.PreferredVersion(mine, aliens); v != extended {
		t.Fatalf("my version = %d, want %d", v, extended)
	}
	if v, _ := d.PreferredVersion(theirs, aliens); v != 0 {
		t.Fatalf("another profile's version = %d", v)
	}
	if err := d.SetPreferredVersion(mine, aliens, theatrical); err != nil {
		t.Fatal(err)
	}
	if v, _ := d.PreferredVersion(mine, aliens); v != theatrical {
		t.Fatalf("after changing: %d", v)
	}
	if f, _ := d.File(bg, extended); f.Edition != "Extended" {
		t.Fatalf("edition = %q", f.Edition)
	}
	// Two cuts are one "edition" each: the Manage view doesn't call them duplicates.
	rows, err := d.ManageRows(bg, lib)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.ID == aliens && (r.Editions != 2 || r.FileCount != 2) {
			t.Fatalf("manage row: %+v", r)
		}
	}
}
