package metadata

import (
	"context"
	"errors"
	"time"
)

// SeriesMatch is one series a provider knows by a title, with the years it ran.
type SeriesMatch struct {
	ImdbID    string
	Title     string
	StartYear int
	EndYear   int  // 0 while still running
	Shared    bool // another series has the same title, so the year tells them apart
}

// SeriesSearcher is implemented by providers that can list every series with
// a title and a season's episodes. OMDb does.
type SeriesSearcher interface {
	SearchSeries(ctx context.Context, title string) ([]SeriesMatch, error)
	Season(ctx context.Context, seriesImdbID string, season int) ([]Episode, error)
}

// EpisodeHint is what a TV guide says about one airing.
type EpisodeHint struct {
	Title        string    // series title as the guide has it
	Season       int       // 0 when unknown
	Episode      int       // 0 when unknown
	EpisodeTitle string    // "" when unknown
	AirDate      time.Time // the episode's original air date; zero when unknown
}

// maxEpisodeChecks bounds the season lookups for one airing.
const maxEpisodeChecks = 4

// IdentifySeries works out which series an airing belongs to when several
// share its title (MacGyver 1985 and 2016). It returns nil when it can't tell.
//
// A single exact-title series wins outright. Otherwise the original air date
// must fall in a series' run, and when that still leaves several, the season's
// episode list decides: a matching episode title or release date.
func IdentifySeries(ctx context.Context, p SeriesSearcher, h EpisodeHint) (*SeriesMatch, error) {
	all, err := p.SearchSeries(ctx, h.Title)
	if err != nil {
		return nil, err
	}
	var exact []SeriesMatch
	for _, m := range all {
		if TitleScore(h.Title, m.Title) == 3 {
			exact = append(exact, m)
		}
	}
	if len(exact) <= 1 {
		if len(exact) == 1 {
			return &exact[0], nil
		}
		return nil, nil
	}
	for i := range exact {
		exact[i].Shared = true
	}

	cands := exact
	if !h.AirDate.IsZero() {
		y := h.AirDate.Year()
		var inRun []SeriesMatch
		for _, m := range exact {
			if m.StartYear > 0 && y >= m.StartYear-1 && (m.EndYear == 0 || y <= m.EndYear+1) {
				inRun = append(inRun, m)
			}
		}
		if len(inRun) == 1 {
			return &inRun[0], nil
		}
		if len(inRun) > 1 {
			cands = inRun
		}
	}
	if h.Season <= 0 || (h.Episode <= 0 && h.EpisodeTitle == "") {
		return nil, nil
	}

	var best *SeriesMatch
	bestScore, tie := 0, false
	for i := range cands {
		if i >= maxEpisodeChecks {
			break
		}
		eps, err := p.Season(ctx, cands[i].ImdbID, h.Season)
		if err != nil {
			return nil, err
		}
		sc := episodeScore(eps, h)
		switch {
		case sc > bestScore:
			best, bestScore, tie = &cands[i], sc, false
		case sc == bestScore && sc > 0:
			tie = true
		}
	}
	if best == nil || tie {
		return nil, nil
	}
	return best, nil
}

// episodeScore rates how well a season's episodes fit the airing: 2 for a
// matching episode title, 2 for a release within two days of the air date.
func episodeScore(eps []Episode, h EpisodeHint) int {
	score := 0
	for _, e := range eps {
		if h.Episode > 0 && e.Episode != h.Episode {
			continue
		}
		s := 0
		if h.EpisodeTitle != "" && Normalize(e.Title) == Normalize(h.EpisodeTitle) {
			s += 2
		}
		if !h.AirDate.IsZero() {
			if rel, err := time.Parse("2006-01-02", e.Released); err == nil {
				if d := rel.Sub(h.AirDate.UTC().Truncate(24 * time.Hour)); d > -48*time.Hour && d < 48*time.Hour {
					s += 2
				}
			}
		}
		score = max(score, s)
	}
	return score
}

// IdentifySeries asks the first provider that can search series.
func (c *Chain) IdentifySeries(ctx context.Context, h EpisodeHint) (*SeriesMatch, error) {
	if c == nil {
		return nil, nil
	}
	var errs []error
	for _, p := range c.Providers {
		s, ok := p.(SeriesSearcher)
		if !ok {
			continue
		}
		m, err := IdentifySeries(ctx, s, h)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if m != nil {
			return m, nil
		}
	}
	return nil, errors.Join(errs...)
}
