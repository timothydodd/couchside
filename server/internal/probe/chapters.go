package probe

import (
	"context"
	"encoding/json"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// Chapter is one chapter mark in a file.
type Chapter struct {
	Title      string
	Start, End float64 // seconds
}

// Chapters reads a file's chapter marks (none is not an error).
func Chapters(ctx context.Context, bin, path string) ([]Chapter, error) {
	out, err := exec.CommandContext(ctx, bin, "-v", "error", "-print_format", "json", "-show_chapters", path).Output()
	if err != nil {
		return nil, err
	}
	var p struct {
		Chapters []struct {
			Start string            `json:"start_time"`
			End   string            `json:"end_time"`
			Tags  map[string]string `json:"tags"`
		} `json:"chapters"`
	}
	if err := json.Unmarshal(out, &p); err != nil {
		return nil, err
	}
	var cs []Chapter
	for _, c := range p.Chapters {
		start, err1 := strconv.ParseFloat(c.Start, 64)
		end, err2 := strconv.ParseFloat(c.End, 64)
		if err1 != nil || err2 != nil || end <= start {
			continue
		}
		title := ""
		for k, v := range c.Tags {
			if strings.EqualFold(k, "title") {
				title = v
			}
		}
		cs = append(cs, Chapter{Title: strings.TrimSpace(title), Start: start, End: end})
	}
	return cs, nil
}

// Chapter names that mean the opening titles or the end credits. Whole
// names only ("Intro", "Opening Credits", "OP", "ED 2"): a chapter called
// "The Opening Move" is part of the show.
var (
	reIntroChapter   = regexp.MustCompile(`(?i)^(intro(duction)?|opening( (credits|titles?|theme|sequence))?|main titles?|title sequence|titles|theme song|op)( ?\d+)?$`)
	reCreditsChapter = regexp.MustCompile(`(?i)^((end|closing|ending) ?(credits|titles?|theme)?|credits|outro|ed)( ?\d+)?$`)
)

// Span is a stretch of a file, in seconds.
type Span struct{ Start, End float64 }

// IntroAndCredits picks the intro and the end credits out of a file's
// chapters by their names. An intro counts only in the first half of the
// file and when it's under five minutes; credits only when they run to the
// end (allowing a short chapter after them, a studio logo or a preview).
func IntroAndCredits(chapters []Chapter, duration float64) (intro, credits *Span) {
	if duration <= 0 && len(chapters) > 0 {
		duration = chapters[len(chapters)-1].End
	}
	for i, c := range chapters {
		switch {
		case intro == nil && reIntroChapter.MatchString(c.Title) && c.Start < duration/2 && c.End-c.Start >= 5 && c.End-c.Start <= 300:
			intro = &Span{c.Start, c.End}
		case reCreditsChapter.MatchString(c.Title) && c.Start > duration/2:
			// Everything from here on, if what follows is short.
			if i == len(chapters)-1 || duration-c.End <= 120 {
				credits = &Span{c.Start, duration}
			}
		}
	}
	return intro, credits
}
