package fsx

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func isVideo(n string) bool { return strings.HasSuffix(n, ".mkv") }

func link(t *testing.T, target, at string) {
	t.Helper()
	if err := os.Symlink(target, at); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}
}

func TestAllowed(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "lib")
	other := filepath.Join(dir, "other")
	for _, d := range []string{lib, other} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p string) string {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	plain := write(filepath.Join(lib, "Heat (1995).mkv"))
	secret := write(filepath.Join(dir, "auth.key"))
	elsewhere := write(filepath.Join(other, "Alien (1979).mkv"))
	toSecret := filepath.Join(lib, "Planted.mkv")
	link(t, secret, toSecret)
	toVideo := filepath.Join(lib, "Alien (1979).mkv")
	link(t, elsewhere, toVideo)
	toDir := filepath.Join(lib, "Folder.mkv")
	link(t, other, toDir)
	inLib := filepath.Join(lib, "Same.mkv")
	link(t, plain, inLib)

	cases := []struct {
		name string
		p    string
		want error
	}{
		{"plain file", plain, nil},
		{"link to a secret", toSecret, ErrRefused},
		{"link to a video elsewhere", toVideo, nil},
		{"link to a folder", toDir, ErrRefused},
		{"link inside the library", inLib, nil},
	}
	for _, c := range cases {
		if got := Allowed(c.p, lib, isVideo); !errors.Is(got, c.want) {
			t.Errorf("%s: Allowed = %v, want %v", c.name, got, c.want)
		}
	}
	if err := Allowed(filepath.Join(lib, "gone.mkv"), lib, isVideo); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing file: %v, want not-exist", err)
	}
	// A library whose own path is a link keeps working.
	libLink := filepath.Join(dir, "media")
	link(t, lib, libLink)
	if err := Allowed(filepath.Join(libLink, "Heat (1995).mkv"), libLink, func(string) bool { return false }); err != nil {
		t.Errorf("library behind a link: %v", err)
	}
}
