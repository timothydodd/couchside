package parse

import "testing"

func TestMovie(t *testing.T) {
	cases := []struct {
		path  string
		title string
		year  int
	}{
		{"/m/The.Matrix.1999.1080p.BluRay.x264.mkv", "The Matrix", 1999},
		{"/m/Inception (2010)/Inception (2010).mkv", "Inception", 2010},
		{"/m/Inception (2010)/movie.mkv", "Inception", 2010},
		{"/m/2001 A Space Odyssey (1968).mkv", "2001 A Space Odyssey", 1968},
		{"/m/1917 (2019).mp4", "1917", 2019},
		{"/m/1917.mp4", "1917", 0},
		{"/m/Blade_Runner_2049_2017_2160p.mkv", "Blade Runner 2049", 2017},
		{"/m/[Group] Spirited.Away.2001.720p.mkv", "Spirited Away", 2001},
		{"/m/Mr. Nobody (2009).mkv", "Mr. Nobody", 2009},
		{"/m/Heat.1080p.WEB-DL.mkv", "Heat", 0},
		{"/m/Alien - Director's Cut (1979).mkv", "Alien - Director's Cut", 1979},
		{"/m/THE BURBS/THE BURBS_t03.mkv", "THE BURBS", 0},
	}
	for _, c := range cases {
		r := Movie(c.path)
		if r.Title != c.title || r.Year != c.year {
			t.Errorf("Movie(%q) = %q %d, want %q %d", c.path, r.Title, r.Year, c.title, c.year)
		}
	}
}

func TestEpisode(t *testing.T) {
	cases := []struct {
		rel        string
		title      string
		year, s, e int
	}{
		{"Breaking Bad/Season 01/Breaking.Bad.S01E02.720p.mkv", "Breaking Bad", 0, 1, 2},
		{"Doctor Who (2005)/Season 3/Doctor Who - 3x07 - 42.mkv", "Doctor Who", 2005, 3, 7},
		{"The Office (US)/S02/The Office s02e10.mkv", "The Office (US)", 0, 2, 10},
		{"Severance/Season 2/Episode 4.mkv", "Severance", 0, 2, 4},
		{"Arrested.Development.S03E13.mkv", "Arrested Development", 0, 3, 13},
		{"Firefly/Firefly S01 E05.mkv", "Firefly", 0, 1, 5},
		{"The Three Stooges (1934)/Season 2024/The Three Stooges (1934) - S2024E04 - Fiddlers Three.ts", "The Three Stooges", 1934, 2024, 4},
		{"Good Morning America (1975)/Season 2025/Good Morning America (1975) - S2025E348 - Good Morning America.ts", "Good Morning America", 1975, 2025, 348},
		{"Garth Marenghi's DARKPLACE/Garth Marenghi's DARKPLACE E01 DVD-RIP x264-S4L.mkv", "Garth Marenghi's DARKPLACE", 0, 1, 1},
		{"Conquering Northen China/Episode 3 _ The Grasslands.mp4", "Conquering Northen China", 0, 1, 3},
		{"The Simpsons (1989)/Season 36/The Simpsons (1989) - 2024-11-10 03 30 00 - The Simpsons.ts", "The Simpsons", 1989, 36, 2411100330},
		{"Sherlock Holmes (2023)/Season 2025/Sherlock Holmes (2023) - 2025-02-08 12 00 00 - Sherlock Holmes.ts", "Sherlock Holmes", 2023, 2025, 2502081200},
	}
	for _, c := range cases {
		r, ok := Episode(c.rel)
		if !ok || r.Title != c.title || r.Year != c.year || r.Season != c.s || r.Episode != c.e {
			t.Errorf("Episode(%q) = %+v ok=%v, want %q %d S%dE%d", c.rel, r, ok, c.title, c.year, c.s, c.e)
		}
	}
	if _, ok := Episode("Some Show/Extras/Behind the scenes.mkv"); ok {
		t.Error("expected no episode number for an extras file")
	}
}

func TestEpisodeTitlesAndDates(t *testing.T) {
	r, _ := Episode("Copa Mundial de la FIFA 2026 (2026)/Season 2026/Copa Mundial de la FIFA 2026 (2026) - 2026-07-14 12 00 00 - Semifinal Francia vs. Espana.ts")
	if r.AirDate != "2026-07-14 12:00" || r.EpisodeTitle != "Semifinal Francia vs. Espana" {
		t.Errorf("date recording: %+v", r)
	}
	r, _ = Episode("The Simpsons (1989)/Season 36/The Simpsons (1989) - 2024-11-10 03 30 00 - The Simpsons.ts")
	if r.EpisodeTitle != "" {
		t.Errorf("show name repeated as episode title should be dropped: %q", r.EpisodeTitle)
	}
	r, _ = Episode("MacGyver (2016)/Season 06/MacGyver (2016) - S06E05 - The Wall.ts")
	if r.EpisodeTitle != "The Wall" || r.AirDate != "" {
		t.Errorf("sonarr style: %+v", r)
	}
	r, _ = Episode("Breaking Bad/Season 01/Breaking.Bad.S01E02.720p.mkv")
	if r.EpisodeTitle != "" {
		t.Errorf("scene name should have no episode title: %q", r.EpisodeTitle)
	}
	if _, ok := Episode("AquaTeen/AQUA_TEEN_HUNGER_FORCE_VOL4/title_t00.mkv"); ok {
		t.Error("disc rip title files have no episode number")
	}
}
