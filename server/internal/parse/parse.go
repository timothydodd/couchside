// Package parse turns media file paths into a title, year and episode number.
package parse

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Result struct {
	Title        string
	Year         int    // 0 when unknown
	Season       int    // episodes only
	Episode      int    // episodes only
	EpisodeTitle string // episodes only, from "Show - S01E02 - Title" style names
	AirDate      string // date-named recordings: "2006-01-02 15:04" (time optional)
}

var (
	reSxxExx = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])s(\d{1,4})[ ._-]*e(\d{1,4})`)
	// DVR recordings named by air date: "Show - 2024-11-10 03 30 00 - Title".
	reAirDate = regexp.MustCompile(`(?:^|[^0-9])((?:19|20)\d{2})[-._ ](\d{2})[-._ ](\d{2})(?:[ ._T-]+(\d{2})[ ._:-](\d{2})(?:[ ._:-]\d{2})?)?(?:[^0-9]|$)`)
	// MakeMKV names titles "<disc>_t03"; the suffix isn't part of the name.
	reDiscTitle = regexp.MustCompile(`(?i)[_ ]t\d{2,3}$`)
	reNxNN      = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(\d{1,2})x(\d{2,3})(?:[^0-9]|$)`)
	reEpOnly    = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:e|ep|episode)[ ._-]*(\d{1,3})(?:[^0-9]|$)`)
	reSeason    = regexp.MustCompile(`(?i)^(?:season|series|s)[ ._-]*(\d{1,4})$`)
	reYear      = regexp.MustCompile(`(?:19|20)\d{2}`)
	reBracket   = regexp.MustCompile(`\[[^\]]*\]|\{[^}]*\}`)
	reJunk      = regexp.MustCompile(`(?i)(?:^|[ ._-])(2160p|1080p|1080i|720p|576p|480p|4k|uhd|hdr|hdr10|dv|bluray|blu-ray|bdrip|brrip|webrip|web-dl|webdl|web|hdtv|dvdrip|dvd|remux|x264|x265|h\.?264|h\.?265|hevc|avc|xvid|aac|ac3|eac3|dts|atmos|truehd|10bit|proper|repack|extended|unrated|remastered|imax|multi|dual|complete)(?:$|[ ._\-\])])`)
	reSpaces    = regexp.MustCompile(`\s+`)
)

// Movie parses a movie file. When the filename has no year but its folder
// does ("Inception (2010)/movie.mkv"), the folder wins.
//
// path is relative to the library root. Files in an extras folder
// ("Inception (2010)/Featurettes/x.mkv") belong to the movie folder above it.
func Movie(path string) Result {
	if _, ok := inExtrasDir(path); ok {
		if d := Name(filepath.Base(filepath.Dir(filepath.Dir(path)))); d.Title != "" {
			return d
		}
	}
	r := Name(stem(path))
	if r.Year == 0 {
		dir := filepath.Base(filepath.Dir(path))
		if d := Name(dir); d.Year != 0 && d.Title != "" {
			return d
		}
	}
	return r
}

// Episode parses a TV file given its path relative to the library root.
// The first folder under the root is the series; the file (or a "Season N"
// folder) supplies the numbers. ok is false when no episode number is found.
//
// Supported: S01E02 (seasons up to 4 digits, e.g. S2024E04), 1x02,
// date-named DVR recordings, and "E03"/"Episode 3" either inside a "Season N"
// folder or directly in the show folder (season 1).
func Episode(rel string) (r Result, ok bool) {
	rel = filepath.ToSlash(rel)
	parts := strings.Split(rel, "/")
	file := stem(parts[len(parts)-1])
	clean := reBracket.ReplaceAllString(file, " ")
	seasonDir := 0
	if len(parts) >= 3 {
		if sm := reSeason.FindStringSubmatch(parts[len(parts)-2]); sm != nil {
			seasonDir, _ = strconv.Atoi(sm[1])
		}
	}

	var titleEnd int // where the show name ends in a root-level file name
	if m := reSxxExx.FindStringSubmatchIndex(clean); m != nil {
		r.Season, _ = strconv.Atoi(clean[m[2]:m[3]])
		r.Episode, _ = strconv.Atoi(clean[m[4]:m[5]])
		r.EpisodeTitle = episodeTitle(clean[m[1]:])
		ok, titleEnd = true, m[0]
	} else if m := reNxNN.FindStringSubmatchIndex(clean); m != nil {
		r.Season, _ = strconv.Atoi(clean[m[2]:m[3]])
		r.Episode, _ = strconv.Atoi(clean[m[4]:m[5]])
		r.EpisodeTitle = episodeTitle(clean[m[5]:])
		ok, titleEnd = true, m[0]
	} else if m := reAirDate.FindStringSubmatchIndex(clean); m != nil {
		y, _ := strconv.Atoi(clean[m[2]:m[3]])
		mo, _ := strconv.Atoi(clean[m[4]:m[5]])
		d, _ := strconv.Atoi(clean[m[6]:m[7]])
		hh, mm := 0, 0
		r.AirDate = fmt.Sprintf("%04d-%02d-%02d", y, mo, d)
		if m[8] >= 0 {
			hh, _ = strconv.Atoi(clean[m[8]:m[9]])
			mm, _ = strconv.Atoi(clean[m[10]:m[11]])
			r.AirDate += fmt.Sprintf(" %02d:%02d", hh, mm)
		}
		if mo >= 1 && mo <= 12 && d >= 1 && d <= 31 {
			// Date recordings have no episode number. Use YYMMDDHHMM: unique
			// per airing and sorts chronologically within the season.
			r.Season = seasonDir
			if r.Season == 0 {
				r.Season = y
			}
			r.Episode = (y%100)*100000000 + mo*1000000 + d*10000 + hh*100 + mm
			r.EpisodeTitle = episodeTitle(clean[m[1]:])
			ok, titleEnd = true, m[0]
		}
	} else if m := reEpOnly.FindStringSubmatchIndex(clean); m != nil {
		switch {
		case seasonDir > 0:
			r.Season = seasonDir
		case len(parts) == 2:
			r.Season = 1 // "Show/Show E01.mkv": a single-season show
		}
		if r.Season > 0 {
			r.Episode, _ = strconv.Atoi(clean[m[2]:m[3]])
			r.EpisodeTitle = episodeTitle(clean[m[1]:])
			ok = true
		}
	}
	if len(parts) > 1 {
		s := Name(parts[0])
		r.Title, r.Year = s.Title, s.Year
	} else if ok && titleEnd > 0 {
		s := Name(clean[:titleEnd])
		r.Title, r.Year = s.Title, s.Year
	}
	// DVR files repeat the show name as the "episode title".
	if strings.EqualFold(r.EpisodeTitle, r.Title) {
		r.EpisodeTitle = ""
	}
	return r, ok && r.Title != ""
}

// episodeTitle pulls "Title" out of the remainder after an episode marker,
// e.g. " - The Wall" or ".The.Wall.720p.WEB".
func episodeTitle(rest string) string {
	if m := reJunk.FindStringIndex(rest); m != nil {
		rest = rest[:m[0]]
	}
	rest = strings.Trim(rest, " -._")
	if rest == "" || strings.EqualFold(rest, "proper") {
		return ""
	}
	return cleanTitle(rest)
}

// Name extracts a title and year from a single file or folder name.
func Name(name string) Result {
	s := reBracket.ReplaceAllString(name, " ")
	s = reDiscTitle.ReplaceAllString(s, "")
	var r Result
	cut := len(s)
	// Take the last year that isn't the start of the name, so
	// "2001 A Space Odyssey (1968)" → 1968 and "1917" stays a title.
	for _, m := range reYear.FindAllStringIndex(s, -1) {
		if m[0] == 0 || !isSep(s[m[0]-1]) || (m[1] < len(s) && !isSep(s[m[1]])) {
			continue
		}
		r.Year, _ = strconv.Atoi(s[m[0]:m[1]])
		cut = m[0]
	}
	if m := reJunk.FindStringIndex(s); m != nil && m[0] < cut && m[0] > 0 {
		cut = m[0]
		if r.Year != 0 && !strings.Contains(s[cut:], strconv.Itoa(r.Year)) {
			r.Year = 0
		}
	}
	r.Title = cleanTitle(s[:cut])
	if r.Title == "" {
		r.Title = cleanTitle(s)
	}
	return r
}

func isSep(c byte) bool { return strings.IndexByte(" ._-()[]", c) >= 0 }

func cleanTitle(s string) string {
	s = strings.Trim(s, " -.([")
	if !strings.Contains(s, " ") {
		s = strings.ReplaceAll(s, ".", " ")
	}
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.Trim(s, " -.([")
	return strings.TrimSpace(reSpaces.ReplaceAllString(s, " "))
}

func stem(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

var videoExts = map[string]bool{
	".mkv": true, ".mp4": true, ".m4v": true, ".avi": true, ".mov": true, ".wmv": true,
	".webm": true, ".ts": true, ".m2ts": true, ".mpg": true, ".mpeg": true, ".flv": true,
}

func IsVideo(path string) bool { return videoExts[strings.ToLower(filepath.Ext(path))] }
