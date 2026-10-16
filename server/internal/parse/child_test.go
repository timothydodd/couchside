package parse

import "testing"

func TestChildTitles(t *testing.T) {
	const set = "Looney Tunes Platinum Collection - Volume 1 (2011)"
	for _, c := range []struct {
		path   string
		title  string
		year   int
		parent string
		group  string
		sortNo int
	}{
		// A bonus disc: a child of the set, named after the file without its track number.
		{set + "/Disc 1 - Behind the Tunes/01 - A Greeting from Chuck Jones.mp4", "A Greeting from Chuck Jones", 0, "Looney Tunes Platinum Collection - Volume 1", "Behind the Tunes", 1},
		// Two levels down, with a year of its own: a two-segment group label.
		{set + "/Disc 3 - Bonus Materials/Bonus Cartoons/02 - Chuck Amuck - The Movie (1991).mp4", "Chuck Amuck - The Movie", 1991, "Looney Tunes Platinum Collection - Volume 1", "Bonus Materials · Bonus Cartoons", 2},
		{set + "/Disc 3 - Bonus Materials/Bonus Cartoons/05 - The Door (1968).mp4", "The Door", 1968, "Looney Tunes Platinum Collection - Volume 1", "Bonus Materials · Bonus Cartoons", 5},
		// No track number.
		{set + "/Disc 2 - Behind the Tunes/The Animated World of Chuck Jones.mp4", "The Animated World of Chuck Jones", 0, "Looney Tunes Platinum Collection - Volume 1", "Behind the Tunes", 0},
		// "3.10 to Yuma" isn't track 3.
		{"Westerns (2007)/Bonus Discs/3.10 to Yuma.mkv", "3.10 to Yuma", 0, "Westerns", "Bonus Discs", 0},
		// Not children: a movie folder's own file, a disc folder, a folder with no year above, a nested year folder.
		{"Inception (2010)/Inception (2010).mkv", "Inception", 2010, "", "", 0},
		{"Kill Bill (2003)/Disc 1/Kill Bill cd1.mkv", "Kill Bill cd1", 0, "", "", 0},
		{"Pixar/Shorts/Bao (2018).mkv", "Bao", 2018, "", "", 0},
		{"Alien (1979)/Aliens (1986)/Aliens.mkv", "Aliens", 1986, "", "", 0},
		// A top-level movie keeps its number.
		{"2001 - A Space Odyssey (1968).mkv", "2001 - A Space Odyssey", 1968, "", "", 0},
		{"12 Angry Men (1957)/12 Angry Men.mkv", "12 Angry Men", 1957, "", "", 0},
	} {
		r := MovieIn(c.path, folders(nil))
		if r.Title != c.title || r.Year != c.year {
			t.Errorf("%s: %q %d, want %q %d", c.path, r.Title, r.Year, c.title, c.year)
		}
		switch {
		case c.parent == "" && r.Child != nil:
			t.Errorf("%s: child of %q, want a movie of its own", c.path, r.Child.ParentTitle)
		case c.parent != "" && r.Child == nil:
			t.Errorf("%s: a movie of its own, want a child of %q", c.path, c.parent)
		case c.parent != "":
			if r.Child.ParentTitle != c.parent || r.Child.ParentYear != 2011 && r.Child.ParentYear != 2007 || r.Child.Group != c.group || r.Child.SortNo != c.sortNo {
				t.Errorf("%s: child %+v, want parent %q group %q no %d", c.path, *r.Child, c.parent, c.group, c.sortNo)
			}
		}
		// With the rule off, the file is a movie named after itself.
		if flat := MovieWith(c.path, folders(nil), false); flat.Child != nil {
			t.Errorf("%s: nesting off still gave a child", c.path)
		} else if c.sortNo > 0 && flat.Title == r.Title {
			t.Errorf("%s: nesting off should keep the track number, got %q", c.path, flat.Title)
		}
	}
}

func TestChildRole(t *testing.T) {
	set := "Looney Tunes Platinum Collection - Volume 1 (2011)"
	lib := folders(map[string][]string{
		set + "/Disc 1 - Behind the Tunes":         {"01 - The Making of Duck Dodgers.mp4", "Looney Tunes (2011) - Behind the Scenes.mp4"},
		set + "/Disc 3 - Bonus Materials/Trailers": {"Trailer.mp4"},
	})
	// A child's file is its own copy even when named like an extra.
	for _, p := range []string{set + "/Disc 1 - Behind the Tunes/01 - The Making of Duck Dodgers.mp4", set + "/Disc 1 - Behind the Tunes/Looney Tunes (2011) - Behind the Scenes.mp4"} {
		if r := MovieRoleIn(p, MovieIn(p, lib).Title, lib); r.Kind != "copy" {
			t.Errorf("%s: child role = %+v, want copy", p, r)
		}
	}
	// With nesting off, a name with a year and an extras word is an extra as before.
	p := set + "/Disc 1 - Behind the Tunes/Looney Tunes (2011) - Behind the Scenes.mp4"
	if r := MovieRoleWith(p, MovieWith(p, lib, false).Title, lib, false); r.Kind != "extra" {
		t.Errorf("nesting off: role = %+v, want extra", r)
	}
	// A Plex extras folder inside the set is still extras of the set itself.
	p = set + "/Disc 3 - Bonus Materials/Trailers/Trailer.mp4"
	m := MovieIn(p, lib)
	if r := MovieRoleIn(p, m.Title, lib); r.Kind != "extra" || m.Child != nil || m.Title != "Looney Tunes Platinum Collection - Volume 1" || r.Extra != "Trailer" {
		t.Errorf("trailers folder: role %+v under %q (child %v), want an extra of the set", r, m.Title, m.Child != nil)
	}
}
