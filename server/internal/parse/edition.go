package parse

import (
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// Plex's tag: "Blade Runner (1982) {edition-Final Cut}.mkv".
	reEditionTag = regexp.MustCompile(`(?i)\{edition-([^}]+)\}`)
	// Cuts named in the usual release-name way. Only looked for after the
	// year, so "The Final Cut (2004)" and "Uncut Gems (2019)" are titles.
	editions = []struct {
		re   *regexp.Regexp
		name string
	}{
		{regexp.MustCompile(`(?i)\bdirector'?s?[ ._-]cut\b`), "Director's Cut"},
		{regexp.MustCompile(`(?i)\bfinal[ ._-]cut\b`), "Final Cut"},
		{regexp.MustCompile(`(?i)\bextended([ ._-](cut|edition|version))?\b`), "Extended"},
		{regexp.MustCompile(`(?i)\bunrated\b`), "Unrated"},
		{regexp.MustCompile(`(?i)\buncut\b`), "Uncut"},
		{regexp.MustCompile(`(?i)\btheatrical([ ._-](cut|edition|version))?\b`), "Theatrical"},
		{regexp.MustCompile(`(?i)\bremastered\b`), "Remastered"},
		{regexp.MustCompile(`(?i)\bspecial[ ._-]edition\b`), "Special Edition"},
		{regexp.MustCompile(`(?i)\bimax\b`), "IMAX"},
	}
)

// Edition is which cut of a film a file is, from its name: Plex's
// {edition-…} tag, or a cut named after the year ("Aliens (1986) Extended
// 1080p.mkv"). "" for the ordinary cut.
func Edition(path string) string {
	name := stem(filepath.Base(path))
	if m := reEditionTag.FindStringSubmatch(name); m != nil {
		return strings.TrimSpace(m[1])
	}
	// The tag on the film's own folder covers the file inside it.
	if m := reEditionTag.FindStringSubmatch(filepath.Base(filepath.Dir(path))); m != nil {
		return strings.TrimSpace(m[1])
	}
	loc := reYear.FindAllStringIndex(name, -1)
	if len(loc) == 0 {
		return ""
	}
	rest := name[loc[len(loc)-1][1]:]
	var found []string
	for _, e := range editions {
		if e.re.MatchString(rest) {
			found = append(found, e.name)
		}
	}
	return strings.Join(found, ", ")
}
