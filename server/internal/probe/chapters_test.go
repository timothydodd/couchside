package probe

import "testing"

func TestIntroAndCredits(t *testing.T) {
	ep := []Chapter{
		{"Cold Open", 0, 95}, {"Opening Credits", 95, 155}, {"Act One", 155, 800}, {"The Opening Move", 800, 1200},
		{"End Credits", 1200, 1290}, {"Next Time", 1290, 1320},
	}
	intro, credits := IntroAndCredits(ep, 1320)
	if intro == nil || intro.Start != 95 || intro.End != 155 {
		t.Fatalf("intro = %+v", intro)
	}
	if credits == nil || credits.Start != 1200 || credits.End != 1320 {
		t.Fatalf("credits = %+v", credits)
	}

	for _, name := range []string{"Intro", "intro", "Opening", "OP", "OP 1", "Main Title", "Title Sequence"} {
		if in, _ := IntroAndCredits([]Chapter{{name, 30, 90}, {"Episode", 90, 1300}}, 1300); in == nil {
			t.Errorf("%q should be an intro", name)
		}
	}
	for _, name := range []string{"Credits", "ED", "Ending", "Closing Credits", "Outro", "End"} {
		if _, cr := IntroAndCredits([]Chapter{{"Episode", 0, 1200}, {name, 1200, 1300}}, 1300); cr == nil {
			t.Errorf("%q should be credits", name)
		}
	}
	// Numbered chapters, an "opening" that's most of the film, an intro in
	// the second half, credits followed by a long scene: none count.
	for _, cs := range [][]Chapter{
		{{"Chapter 1", 0, 600}, {"Chapter 2", 600, 1300}},
		{{"Opening", 0, 900}, {"Rest", 900, 1300}},
		{{"Part One", 0, 900}, {"Intro", 900, 960}, {"Part Two", 960, 1300}},
		{{"Operation", 0, 60}, {"Editing", 60, 1300}},
	} {
		if in, cr := IntroAndCredits(cs, 1300); in != nil || cr != nil {
			t.Errorf("%v: intro %+v credits %+v", cs, in, cr)
		}
	}
	if _, cr := IntroAndCredits([]Chapter{{"Film", 0, 5000}, {"Credits", 5000, 5300}, {"Epilogue", 5300, 6000}}, 6000); cr != nil {
		t.Errorf("credits before a long epilogue: %+v", cr)
	}
}
