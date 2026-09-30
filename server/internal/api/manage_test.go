package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInsideDir(t *testing.T) {
	for _, c := range []struct {
		p, dir string
		want   bool
	}{
		{"/media/TV/Show/S01E01.mkv", "/media/TV", true},
		{"/media/TV", "/media/TV", false},
		{"/media/TV Shows/x.mkv", "/media/TV", false}, // sibling with a shared prefix
		{"/media/TV/../Movies/x.mkv", "/media/TV", false},
		{"/media/TV/..x/y.mkv", "/media/TV", true}, // a folder named "..x" is fine
	} {
		if got := insideDir(c.p, c.dir); got != c.want {
			t.Errorf("insideDir(%q, %q) = %v, want %v", c.p, c.dir, got, c.want)
		}
	}
}

func TestRemoveEmptyDirs(t *testing.T) {
	root := t.TempDir()
	mk := func(parts ...string) string {
		p := filepath.Join(append([]string{root}, parts...)...)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	// An emptied season and show folder both go; the root stays.
	ep := mk("Show", "Season 1", "e1.mkv")
	os.Remove(ep)
	if kept := removeEmptyDirs(filepath.Dir(ep), root); kept != "" {
		t.Fatalf("kept %q", kept)
	}
	if _, err := os.Stat(filepath.Join(root, "Show")); !os.IsNotExist(err) {
		t.Fatal("empty show folder should be gone")
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatal("library root must stay")
	}
	// A movie folder left with only a poster is reported, not removed.
	mv := mk("Heat (1995)", "Heat.mkv")
	mk("Heat (1995)", "poster.jpg")
	os.Remove(mv)
	if kept := removeEmptyDirs(filepath.Dir(mv), root); kept != filepath.Join(root, "Heat (1995)") {
		t.Fatalf("kept = %q", kept)
	}
	// A season with other episodes left is normal.
	e1 := mk("Other", "Season 1", "e1.mkv")
	mk("Other", "Season 1", "e2.mkv")
	os.Remove(e1)
	if kept := removeEmptyDirs(filepath.Dir(e1), root); kept != "" {
		t.Fatalf("season with episodes left reported %q", kept)
	}
}
