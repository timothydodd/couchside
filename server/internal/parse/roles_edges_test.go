package parse

import (
	"path/filepath"
	"testing"
)

// folders fakes a library: folder → the video files directly in it.
func folders(m map[string][]string) Lister {
	return func(dir string) []string {
		var out []string
		for _, f := range m[dir] {
			out = append(out, filepath.Join(dir, f))
		}
		return out
	}
}

func TestRoleEdgeCases(t *testing.T) {
	lib := folders(map[string][]string{
		"Pixar/Shorts":            {"Bao (2018).mkv"},
		"Heat/Featurettes":        {"Making Of.mkv"},
		"Heat":                    {"Heat.mkv"},
		"Harry Potter 7 (2011)":   {"Harry.Potter.and.the.Deathly.Hallows.Part.2.mkv"},
		"Kill Bill (2003)":        {"Kill Bill Part 1.mkv", "Kill Bill Part 2.mkv"},
		"Inception (2010)":        {"Inception (2010).mkv", "Inception (2010)-trailer.mkv"},
		"Inception (2010)/Shorts": {"Dream.mkv"},
	})
	for _, c := range []struct {
		path  string
		kind  string
		part  int
		title string // the movie it's filed under
	}{
		{"The-Big-Short.mkv", "copy", 0, ""},                                                     // "-Short" is the title's last word
		{"Pixar/Shorts/Bao (2018).mkv", "copy", 0, "Bao"},                                        // "Pixar" isn't a movie
		{"Heat/Featurettes/Making Of.mkv", "extra", 0, "Heat"},                                   // a film file sits above it
		{"Inception (2010)/Shorts/Dream.mkv", "extra", 0, "Inception"},                           // named like a film
		{"Harry Potter 7 (2011)/Harry.Potter.and.the.Deathly.Hallows.Part.2.mkv", "copy", 0, ""}, // no part 1 beside it
		{"Kill Bill (2003)/Kill Bill Part 1.mkv", "part", 1, ""},
		{"Inception (2010)/Inception (2010)-trailer.mkv", "extra", 0, ""},
	} {
		m := MovieIn(c.path, lib)
		r := MovieRoleIn(c.path, m.Title, lib)
		if r.Kind != c.kind || r.Part != c.part || (c.title != "" && m.Title != c.title) {
			t.Errorf("%s: role %+v under %q, want %s %d under %q", c.path, r, m.Title, c.kind, c.part, c.title)
		}
	}
}
