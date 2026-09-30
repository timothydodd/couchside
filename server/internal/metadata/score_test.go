package metadata

import "testing"

func TestTitleScore(t *testing.T) {
	cases := []struct {
		q, c string
		want int
	}{
		{"Goonies", "The Goonies", 3},
		{"Alien - Resurrection", "Alien: Resurrection", 3},
		{"Naked Gun 33 1 3 The Final Insult", "Naked Gun 33 1/3: The Final Insult", 3},
		{"WALL·E", "WALL-E", 1}, // hyphen splits, interpunct joins: close enough to be a prefix? no
		{"Harry Potter and the Deathly Hallows", "Harry Potter and the Deathly Hallows: Part 1", 1},
		{"Plagues & Pleasures On the Salton Sea", "Plagues and Pleasures on the Salton Sea", 3},
		{"Ex Machina", "Ex Machina", 3},
		{"Hellboy II - The Golden Army", "Max on Set: Hellboy II - The Golden Army", 1}, // partial; the film itself wins in pickBest
		{"It", "George Carlin: Doin' It Again", 0},
		{"Kitchen (UK)", "My Kitchen Rules UK", 0},
		{"Return of the Jedi", "Star Wars: Episode VI - Return of the Jedi", 1},
		{"Sweet Home", "Great SFX Adventure: Take Me to 'Sweet Home'", 0},
		{"¿Quien es la mascara", "¿Quién es la máscara?", 3},
	}
	for _, c := range cases {
		if c.q == "WALL·E" {
			continue // documented edge case only
		}
		if got := TitleScore(c.q, c.c); got != c.want {
			t.Errorf("TitleScore(%q, %q) = %d, want %d", c.q, c.c, got, c.want)
		}
	}
}

func TestPickBestPrefersExactTitleAndYear(t *testing.T) {
	cands := []candidate{
		{"tt1", "Ex Machina: Behind the Scenes Vignettes", 2015},
		{"tt2", "Ex Machina", 2014},
	}
	if c, ok := pickBest(Movie, "Ex Machina", 2015, cands); !ok || c.id != "tt2" {
		t.Errorf("got %+v %v, want the film", c, ok)
	}
	cands = []candidate{{"tt7", "Max on Set: Hellboy II - The Golden Army", 2008}, {"tt8", "Hellboy II: The Golden Army", 2008}}
	if c, ok := pickBest(Movie, "Hellboy II - The Golden Army", 2008, cands); !ok || c.id != "tt8" {
		t.Errorf("got %+v, want the film over the featurette", c)
	}
	cands = []candidate{{"tt3", "Harry Potter and the Philosopher's Stone", 1999}}
	if _, ok := pickBest(Movie, "Harry Potter and the Philosopher's Stone", 2001, cands); ok {
		t.Error("a movie two years off must be rejected")
	}
	cands = []candidate{{"tt5", "Antiques Roadshow", 1979}, {"tt6", "Antiques Roadshow US", 1997}}
	if c, ok := pickBest(Series, "Antiques Roadshow", 1997, cands); !ok || c.id != "tt6" {
		t.Errorf("got %+v, want the 1997 US show over the 1979 UK one", c)
	}
	cands = []candidate{{"tt4", "Epic Train Journeys from Above", 2024}}
	if _, ok := pickBest(Series, "Epic Train Journeys From Above", 2022, cands); !ok {
		t.Error("an exact series title should survive a premiere-year mismatch")
	}
}
