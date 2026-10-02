package imaging

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckImage(t *testing.T) {
	dir := t.TempDir()
	for name, c := range map[string]struct {
		body string
		ok   bool
	}{
		"png":      {"\x89PNG\r\n\x1a\n0000", true},
		"jpeg":     {"\xff\xd8\xff\xe0\x00\x10JFIF", true},
		"playlist": {"#EXTM3U\n#EXTINF:10,\nfile:///etc/passwd\n", false},
		"concat":   {"ffconcat version 1.0\nfile /etc/passwd\n", false},
		"html":     {"<!DOCTYPE html><html></html>", false},
	} {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte(c.body), 0o644)
		if err := checkImage(p); (err == nil) != c.ok {
			t.Errorf("%s: checkImage = %v, want ok %v", name, err, c.ok)
		}
	}
}
