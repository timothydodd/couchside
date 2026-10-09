package db

import (
	"context"
	"testing"
)

// A file's dynamic range is stored by the scan, left unknown for files that
// couldn't be read, filled in by SetDynamicRange, and a title's summary
// carries the best of its copies (extras aside).
func TestDynamicRange(t *testing.T) {
	d := openTest(t)
	bg := context.Background()
	lib, _ := d.CreateLibrary(bg, "Movies", "/movies", "movies")
	film, _, _ := d.EnsureItem(bg, lib, "movie", "Dune", 2021)

	sdr, err := d.UpsertFile(bg, File{LibraryID: lib, MediaItemID: film, Path: "/movies/Dune 1080p.mkv", Size: 1, Mtime: 1}, 1)
	if err != nil {
		t.Fatal(err)
	}
	bad, _ := d.UpsertFile(bg, File{LibraryID: lib, MediaItemID: film, Path: "/movies/Dune broken.mkv", Size: 1, Mtime: 1, Problem: "unreadable", DynamicRange: "hdr10"}, 1)
	if it, _ := d.Item(bg, film); it.DynamicRange != "" {
		t.Fatalf("SDR title: range %q", it.DynamicRange)
	}
	// The unreadable file is unknown (NULL), the SDR one is known.
	need, err := d.FilesNeedingDynamicRange(bg, lib)
	if err != nil || len(need) != 0 {
		t.Fatalf("needing: %v %v (unreadable files are re-probed by the scan, not here)", need, err)
	}

	hdr, _ := d.UpsertFile(bg, File{LibraryID: lib, MediaItemID: film, Path: "/movies/Dune 2160p.mkv", Size: 2, Mtime: 1, DynamicRange: "hdr10"}, 1)
	if it, _ := d.Item(bg, film); it.DynamicRange != "hdr10" {
		t.Fatalf("with an HDR10 copy: range %q", it.DynamicRange)
	}
	// An extra doesn't make the title Dolby Vision.
	extra, _ := d.UpsertFile(bg, File{LibraryID: lib, MediaItemID: film, Path: "/movies/Trailer.mkv", Size: 1, Mtime: 1, DynamicRange: "dv", DVProfile: 8}, 1)
	if err := d.SetDetectedRole(bg, extra, "extra", 0, "Trailer"); err != nil {
		t.Fatal(err)
	}
	if it, _ := d.Item(bg, film); it.DynamicRange != "hdr10" {
		t.Fatalf("a Dolby Vision extra changed the title's range to %q", it.DynamicRange)
	}

	// A file indexed before the column existed is NULL until read.
	if _, err := d.sql.ExecContext(bg, `UPDATE files SET dynamic_range = NULL WHERE id = ?`, sdr); err != nil {
		t.Fatal(err)
	}
	need, _ = d.FilesNeedingDynamicRange(bg, lib)
	if len(need) != 1 || need[0].ID != sdr {
		t.Fatalf("needing: %+v", need)
	}
	if err := d.SetDynamicRange(bg, sdr, "dv", 5); err != nil {
		t.Fatal(err)
	}
	f, _ := d.File(bg, sdr)
	if f.DynamicRange != "dv" || f.DVProfile != 5 {
		t.Fatalf("file: %q profile %d", f.DynamicRange, f.DVProfile)
	}
	if it, _ := d.Item(bg, film); it.DynamicRange != "dv" {
		t.Fatalf("with a Dolby Vision copy: range %q", it.DynamicRange)
	}
	_, _ = bad, hdr
}
