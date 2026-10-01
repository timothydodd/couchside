package search

import "testing"

func TestScore(t *testing.T) {
	cases := []struct {
		q, title string
		want     int
	}{
		{"alien", "Alien", 100},
		{"ALIEN", "Alien", 100},
		{"spiderman", "Spider-Man", 95},
		{"spider man", "Spider-Man", 100},
		{"spider", "Spider-Man: Homecoming", 90},
		{"spidermanh", "Spider-Man: Homecoming", 80},
		{"office", "The Office (US)", 90}, // the leading "the" is dropped
		{"the office", "The Office", 100},
		{"wars star", "Star Wars", 60},
		{"resurr", "Alien: Resurrection", 60},
		{"amelie", "Amélie", 100},
		{"5.1", "5.1", 100},
		{"ghost", "Ghostbusters", 90},
		{"busters", "Ghostbusters", 30},
		{"st", "Ghostbusters", 0}, // fragments need three letters
		{"zzz", "Alien", 0},
		{"", "Alien", 0},
	}
	for _, c := range cases {
		q := NewQuery(c.q)
		got := 0
		if !q.Empty() {
			got = q.Score(NewDoc("movie", 1, "", 0, 0, c.title))
		}
		if got != c.want {
			t.Errorf("Score(%q, %q) = %d, want %d", c.q, c.title, got, c.want)
		}
	}
}

func TestScoreBestKey(t *testing.T) {
	d := NewDoc("channel", 0, "4.1", 0, 0, "WTVJ-HD", "4.1", "NBC")
	if s := NewQuery("nbc").Score(d); s != 100 {
		t.Errorf("affiliate: got %d", s)
	}
	if s := NewQuery("4.1").Score(d); s != 100 {
		t.Errorf("number: got %d", s)
	}
}

func TestFind(t *testing.T) {
	docs := []Doc{
		NewDoc("movie", 1, "", 0, 0, "Alien: Resurrection"),
		NewDoc("movie", 2, "", 0, 0, "Aliens"),
		NewDoc("movie", 3, "", 0, 0, "Alien"),
		NewDoc("movie", 4, "", 0, 0, "Big Buck Bunny"),
		NewDoc("series", 5, "", 0, 0, "Alien Worlds"),
		NewDoc("program", 6, "", 200, 300, "Alien Nation"),
		NewDoc("program", 7, "", 100, 150, "Alien Nation"), // already over
		NewDoc("program", 8, "", 150, 400, "Alien Nation"),
	}
	got := Find(docs, NewQuery("alien"), 2, 160)
	ids := func(kind string) []int64 {
		var out []int64
		for _, h := range got[kind] {
			out = append(out, h.ID)
		}
		return out
	}
	if m := ids("movie"); len(m) != 2 || m[0] != 3 || m[1] != 2 {
		t.Errorf("movies = %v, want [3 2] (exact, then the shorter prefix match)", m)
	}
	if s := ids("series"); len(s) != 1 || s[0] != 5 {
		t.Errorf("series = %v", s)
	}
	if p := ids("program"); len(p) != 2 || p[0] != 8 || p[1] != 6 {
		t.Errorf("programs = %v, want [8 6] (by start time, ended ones dropped)", p)
	}
	if len(Find(docs, NewQuery("  "), 5, 0)) != 0 {
		t.Error("an empty query should find nothing")
	}
}
