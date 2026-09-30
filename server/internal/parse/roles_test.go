package parse

import "testing"

func TestMovieRole(t *testing.T) {
	cases := []struct {
		path string
		want Role
	}{
		{"Inception (2010)/Inception (2010).mkv", Role{Kind: "copy"}},
		{"Inception (2010)/Inception (2010) - Part 2.mkv", Role{Kind: "part", Part: 2}},
		{"Inception (2010)/Inception.2010.cd1.mkv", Role{Kind: "part", Part: 1}},
		{"Inception (2010)/Inception (2010) Disc Two.mkv", Role{Kind: "part", Part: 2}},
		{"Inception (2010)/Inception (2010)-trailer.mkv", Role{Kind: "extra", Extra: "Trailer"}},
		{"Inception (2010)/Dream Levels-featurette.mkv", Role{Kind: "extra", Extra: "Dream Levels"}},
		{"Inception (2010)/Inception (2010) - Behind the Scenes.mkv", Role{Kind: "extra", Extra: "Behind the Scenes"}},
		{"Inception (2010)/Making of Inception.mkv", Role{Kind: "extra", Extra: "Making of Inception"}},
		{"Inception (2010)/Featurettes/Dream Levels.mkv", Role{Kind: "extra", Extra: "Dream Levels"}},
		{"Inception (2010)/Trailers/Inception (2010).mkv", Role{Kind: "extra", Extra: "Trailer"}},
		// Markers that are part of the title don't count.
		{"Harry Potter and the Deathly Hallows Part 2 (2011).mkv", Role{Kind: "copy"}},
		{"The Interview (2014).mkv", Role{Kind: "copy"}},
		// A "Shorts" folder at the library root holds movies, not extras.
		{"Shorts/Paperman (2012).mkv", Role{Kind: "copy"}},
	}
	for _, c := range cases {
		if got := MovieRole(c.path, Movie(c.path).Title); got != c.want {
			t.Errorf("MovieRole(%q) = %+v, want %+v", c.path, got, c.want)
		}
	}
}

func TestMovieInExtrasFolder(t *testing.T) {
	r := Movie("Inception (2010)/Behind The Scenes/Dream Levels.mkv")
	if r.Title != "Inception" || r.Year != 2010 {
		t.Errorf("got %q %d, want Inception 2010", r.Title, r.Year)
	}
}
