package db

import (
	"context"
	"path/filepath"
	"testing"
)

// Merging movies makes the others' files copies or extras of the one kept;
// merging shows moves episodes over by number. Files are pinned to the new
// title and the old titles go.
func TestMergeItems(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	lib, _ := d.CreateLibrary(ctx, "Films", dir, "movies")
	keep, _, _ := d.EnsureItem(ctx, lib, "movie", "Heat", 1995)
	other, _, _ := d.EnsureItem(ctx, lib, "movie", "Heat Making Of", 0)
	_ = d.ApplyMetadata(ctx, other, Metadata{Title: "Heat: Making Of"})
	f1, _ := d.UpsertFile(ctx, File{LibraryID: lib, MediaItemID: keep, Path: filepath.Join(dir, "heat.mkv"), Size: 1, Mtime: 1}, 1)
	f2, _ := d.UpsertFile(ctx, File{LibraryID: lib, MediaItemID: other, Path: filepath.Join(dir, "making.mkv"), Size: 1, Mtime: 1}, 1)
	_ = d.SetDetectedRole(ctx, f2, "copy", 0, "")

	if err := d.MergeItems(ctx, keep, []int64{other}, "extra"); err != nil {
		t.Fatal(err)
	}
	files, _ := d.ItemFiles(ctx, keep)
	if len(files) != 2 {
		t.Fatalf("files after merge = %d, want 2", len(files))
	}
	for _, f := range files {
		if f.ID == f2 && (f.Role != "extra" || f.ExtraTitle != "Heat: Making Of" || !f.RolePinned) {
			t.Fatalf("merged file = %+v, want a pinned extra named after its film", f)
		}
		if f.ID == f1 && f.Role != "copy" {
			t.Fatalf("kept film's own file changed: %+v", f)
		}
	}
	if _, err := d.Item(ctx, other); err == nil {
		t.Fatal("the merged-away title is still there")
	}
	st, _ := d.FileStamp(ctx, filepath.Join(dir, "making.mkv"))
	if !st.ItemPinned || st.MediaItemID != keep {
		t.Fatalf("stamp = %+v, want pinned to %d", st, keep)
	}
	// A changed pinned file keeps its title through UpsertFile.
	if _, err := d.UpsertFile(ctx, File{LibraryID: lib, MediaItemID: other + 100, Path: filepath.Join(dir, "making.mkv"), Size: 2, Mtime: 2}, 2); err != nil {
		t.Fatal(err)
	}
	if f, _ := d.File(ctx, f2); f.MediaItemID != keep {
		t.Fatalf("a rescan moved the pinned file to item %d", f.MediaItemID)
	}
	if err := d.MergeItems(ctx, keep, []int64{keep}, "copy"); err == nil {
		t.Fatal("merging a title into itself must fail")
	}

	// Shows: episodes come over by number, files point at the kept show's rows.
	tv, _ := d.CreateLibrary(ctx, "TV", filepath.Join(dir, "tv"), "tv")
	a, _, _ := d.EnsureItem(ctx, tv, "series", "MacGyver", 2016)
	b, _, _ := d.EnsureItem(ctx, tv, "series", "MacGyver", 0)
	ea, _ := d.EnsureEpisode(ctx, a, 1, 1, "", "")
	eb1, _ := d.EnsureEpisode(ctx, b, 1, 1, "", "")
	eb2, _ := d.EnsureEpisode(ctx, b, 1, 2, "", "")
	_, _ = d.UpsertFile(ctx, File{LibraryID: tv, MediaItemID: a, EpisodeID: &ea, Path: filepath.Join(dir, "a1.mkv"), Size: 1, Mtime: 1}, 1)
	fb1, _ := d.UpsertFile(ctx, File{LibraryID: tv, MediaItemID: b, EpisodeID: &eb1, Path: filepath.Join(dir, "b1.mkv"), Size: 1, Mtime: 1}, 1)
	fb2, _ := d.UpsertFile(ctx, File{LibraryID: tv, MediaItemID: b, EpisodeID: &eb2, Path: filepath.Join(dir, "b2.mkv"), Size: 1, Mtime: 1}, 1)
	if err := d.MergeItems(ctx, a, []int64{b}, ""); err != nil {
		t.Fatal(err)
	}
	if f, _ := d.File(ctx, fb1); f.MediaItemID != a || f.EpisodeID == nil || *f.EpisodeID != ea {
		t.Fatalf("S01E01 from the other show = %+v, want the kept show's episode %d", f, ea)
	}
	f, _ := d.File(ctx, fb2)
	var season, episode int
	if err := d.sql.QueryRow(`SELECT series_id, season, episode FROM episodes WHERE id = ?`, *f.EpisodeID).Scan(new(int64), &season, &episode); err != nil || season != 1 || episode != 2 {
		t.Fatalf("S01E02 moved: %+v (%v)", f, err)
	}
	if err := d.MergeItems(ctx, a, []int64{keep}, ""); err == nil {
		t.Fatal("a movie can't merge into a show")
	}
}
