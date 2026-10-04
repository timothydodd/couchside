package parse

import "testing"

func TestEdition(t *testing.T) {
	for path, want := range map[string]string{
		"Aliens (1986)/Aliens (1986) Extended 1080p BluRay.mkv":          "Extended",
		"Blade Runner (1982) {edition-Final Cut}.mkv":                    "Final Cut",
		"Blade.Runner.1982.The.Final.Cut.2160p.UHD.mkv":                  "Final Cut",
		"Kingdom of Heaven (2005) Director's Cut.mkv":                    "Director's Cut",
		"Kingdom.of.Heaven.2005.Directors.Cut.1080p.mkv":                 "Director's Cut",
		"Oppenheimer (2023) IMAX 2160p.mkv":                              "IMAX",
		"Superbad (2007) Unrated Extended Edition.mkv":                   "Extended, Unrated",
		"Aliens (1986).mkv":                                              "",
		"The Final Cut (2004).mkv":                                       "",
		"Uncut Gems (2019) 1080p.mkv":                                    "",
		"Extended Stay.mkv":                                              "",
		"Apocalypse Now (1979) {edition-Redux}/Apocalypse Now Redux.mkv": "Redux",
	} {
		if got := Edition(path); got != want {
			t.Errorf("Edition(%q) = %q, want %q", path, got, want)
		}
	}
	// Still the same film: the cut doesn't change the title or year.
	if r := Movie("Aliens (1986)/Aliens (1986) Extended 1080p BluRay.mkv"); r.Title != "Aliens" || r.Year != 1986 {
		t.Errorf("Movie = %+v", r)
	}
	if r := Movie("Kingdom of Heaven (2005) Director's Cut.mkv"); r.Title != "Kingdom of Heaven" || r.Year != 2005 {
		t.Errorf("Movie = %+v", r)
	}
}
