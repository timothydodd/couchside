package parse

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Role is what a file is to its movie: another copy, one part of a movie
// split across files, or an extra (featurette, trailer, deleted scene...).
type Role struct {
	Kind  string // copy | part | extra
	Part  int    // parts: 1, 2, ...
	Extra string // extras: a title such as "Behind the Scenes"
}

// extrasDirs are Plex's folder names for bonus material, with the title an
// extra gets when its own name says nothing more.
var extrasDirs = map[string]string{
	"extras": "Extra", "featurettes": "Featurette", "behind the scenes": "Behind the Scenes",
	"deleted scenes": "Deleted Scene", "interviews": "Interview", "scenes": "Scene", "shorts": "Short",
	"trailers": "Trailer", "other": "Extra", "others": "Extra", "bonus": "Bonus", "bonus features": "Bonus",
}

// ExtrasDir reports whether a folder name is an extras folder, and the
// default title for what's in it.
func ExtrasDir(name string) (string, bool) {
	label, ok := extrasDirs[strings.ToLower(strings.TrimSpace(name))]
	return label, ok
}

var (
	// Plex's extras suffixes: "Inception (2010)-behindthescenes.mkv".
	reExtraSuffix = regexp.MustCompile(`(?i)-(behindthescenes|deleted|featurette|interview|scene|short|trailer|other|extra|bonus)$`)
	suffixLabels  = map[string]string{"behindthescenes": "Behind the Scenes", "deleted": "Deleted Scene", "featurette": "Featurette",
		"interview": "Interview", "scene": "Scene", "short": "Short", "trailer": "Trailer", "other": "Extra", "extra": "Extra", "bonus": "Bonus"}
	reExtraWord = regexp.MustCompile(`(?i)(?:^|[^a-z])(behind[ ._-]?the[ ._-]?scenes|making[ ._-]of|featurettes?|deleted[ ._-]scenes?|trailers?|teaser|bonus|interviews?|gag[ ._-]reel|bloopers?|extras?)(?:[^a-z]|$)`)
	// Stacked files: "Movie (2001) - Part 2", "movie.cd1", "Disc 2".
	rePart     = regexp.MustCompile(`(?i)(?:^|[ ._\-(\[])(?:part|pt|cd|disc|disk)[ ._-]*(\d{1,2}|one|two|three|four)(?:$|[ ._\-)\]])`)
	partWords  = map[string]int{"one": 1, "two": 2, "three": 3, "four": 4}
	reLeadYear = regexp.MustCompile(`^[ ._\-(\[]*(?:19|20)\d{2}[ ._\-)\]]*`)
)

// Lister lists the video files directly in a folder (relative to the
// library root, like the paths given to MovieRoleIn).
type Lister func(dir string) []string

// inExtrasDir reports whether a file sits in an extras folder inside a movie
// folder. At the library root, "Shorts" or "Other" is just a folder of movies,
// and so it is under a folder that isn't a movie's ("Pixar/Shorts"): the
// parent must be named like a film with a year, or (when list can tell)
// hold a film file itself.
func inExtrasDir(path string, list Lister) (string, bool) {
	dir := filepath.Dir(path)
	up := filepath.Dir(dir)
	if up == "." || up == "/" || up == dir {
		return "", false
	}
	label, ok := ExtrasDir(filepath.Base(dir))
	if !ok || list == nil || Name(filepath.Base(up)).Year > 0 {
		return label, ok
	}
	return label, len(list(up)) > 0
}

// MovieRole guesses a movie file's role from its path, relative to the
// library root. title is the movie's
// parsed title: a marker that is part of the title ("Deathly Hallows Part 2",
// "The Interview", "The-Big-Short") doesn't count.
func MovieRole(path, title string) Role { return MovieRoleIn(path, title, nil) }

// MovieRoleIn is MovieRole with a look at the folders around the file (list
// may be nil): an extras folder needs a movie around it, and a part marker
// counts only when another file in the folder has one too.
func MovieRoleIn(path, title string, list Lister) Role {
	name := stem(path)
	if label, ok := inExtrasDir(path, list); ok {
		return Role{Kind: "extra", Extra: extraName(name, title, label)}
	}
	if m := reExtraSuffix.FindStringSubmatchIndex(name); m != nil && !reExtraSuffix.MatchString(title) {
		label := suffixLabels[strings.ToLower(name[m[2]:m[3]])]
		return Role{Kind: "extra", Extra: extraName(name[:m[0]], title, label)}
	}
	if m := reExtraWord.FindStringSubmatch(name); m != nil && !reExtraWord.MatchString(title) {
		return Role{Kind: "extra", Extra: extraName(name, title, cleanTitle(m[1]))}
	}
	if m := rePart.FindStringSubmatch(name); m != nil && !rePart.MatchString(title) {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			n = partWords[strings.ToLower(m[1])]
		}
		if n > 0 && (list == nil || otherPart(path, list)) {
			return Role{Kind: "part", Part: n}
		}
	}
	return Role{Kind: "copy"}
}

// otherPart reports whether another video next to path has a part marker:
// "Deathly.Hallows.Part.2.mkv" on its own is a film, not half of one.
func otherPart(path string, list Lister) bool {
	for _, f := range list(filepath.Dir(path)) {
		if filepath.Base(f) != filepath.Base(path) && rePart.MatchString(stem(f)) {
			return true
		}
	}
	return false
}

// extraName turns an extra's file name into its title: "Inception (2010) -
// Dream Levels" → "Dream Levels". label is used when nothing is left.
func extraName(name, title, label string) string {
	s := cleanTitle(reBracket.ReplaceAllString(name, " "))
	if title != "" && len(s) >= len(title) && strings.EqualFold(s[:len(title)], title) {
		s = s[len(title):]
	}
	s = reLeadYear.ReplaceAllString(s, "")
	if s = strings.Trim(s, " -._()[]"); s == "" {
		return label
	}
	return s
}
