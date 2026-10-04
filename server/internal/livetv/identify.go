package livetv

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/metadata"
)

const (
	identifyTimeout = 8 * time.Second  // when a recording starts and nothing is stored yet
	identifyRetry   = 10 * time.Minute // after a failed lookup, use what's stored (or the plain name) until then
)

// How long a stored answer is used before asking again. Tests shorten them.
var (
	recheckUnknown = 7 * 24 * time.Hour  // a series that couldn't be identified
	recheckKnown   = 30 * 24 * time.Hour // a new same-titled series may have appeared
)

// showYear is the premiere year to put in a recording's show folder, or 0.
// The guide names series without a year, so MacGyver (1985) and MacGyver
// (2016) look alike. The episode's original air date and number are checked
// against OMDb (metadata.IdentifySeries). The year is only used when the
// title is shared, so ordinary shows keep plain folder names.
//
// Results are stored per guide SeriesID. lookup=false only reads what's stored.
func (s *Service) showYear(ctx context.Context, r db.Recording, lookup bool) int {
	// known is the stored answer, used as it is while fresh and as the
	// fallback when it's due a recheck and the provider can't be reached: a
	// show that needed its year yesterday still needs it during an outage.
	known := 0
	if r.SeriesID != "" {
		if m, err := s.db.SeriesMatchFor(ctx, r.SeriesID); err == nil && m != nil {
			age, ttl := time.Since(time.Unix(m.CheckedAt, 0)), recheckKnown
			if m.ImdbID == "" {
				ttl = recheckUnknown
			}
			if !lookup || age < ttl {
				return m.Year
			}
			known = m.Year
		}
	}
	if !lookup || s.cfg.Metadata.Empty() {
		return known
	}
	failKey := r.SeriesID
	if failKey == "" {
		failKey = "t:" + r.Title
	}
	s.mu.Lock()
	failed, ok := s.lookupErr[failKey]
	if ok && time.Since(failed) >= identifyRetry {
		delete(s.lookupErr, failKey)
		ok = false
	}
	s.mu.Unlock()
	if ok {
		return known
	}
	h := metadata.EpisodeHint{Title: r.Title, EpisodeTitle: r.EpisodeTitle}
	if m := reEpisode.FindStringSubmatch(r.EpisodeNum); m != nil {
		h.Season, _ = strconv.Atoi(m[1])
		h.Episode, _ = strconv.Atoi(m[2])
	}
	if p, _ := s.db.ProgramAt(ctx, r.Channel, r.StartAt); p != nil && p.OriginalAirdate != nil && *p.OriginalAirdate > 0 {
		h.AirDate = time.Unix(*p.OriginalAirdate, 0).UTC()
	}
	lctx, cancel := context.WithTimeout(ctx, identifyTimeout)
	defer cancel()
	m, err := s.cfg.Metadata.IdentifySeries(lctx, h)
	if err != nil {
		// Network or quota trouble: record under the plain name, ask again next time.
		slog.Warn("dvr: couldn't identify series", "title", r.Title, "err", err)
		s.mu.Lock()
		s.lookupErr[failKey] = time.Now()
		s.mu.Unlock()
		return known
	}
	imdb, year := "", 0
	if m != nil {
		imdb = m.ImdbID
		if m.Shared {
			year = m.StartYear
		}
	}
	if r.SeriesID != "" {
		_ = s.db.SetSeriesMatch(ctx, r.SeriesID, imdb, year)
	}
	if year > 0 {
		slog.Info("dvr: identified series", "title", r.Title, "year", year, "imdb", imdb)
	}
	return year
}

// prefetchShow identifies a newly scheduled recording's series in the
// background, so the name is ready when it starts.
func (s *Service) prefetchShow(r db.Recording) {
	if r.SeriesID == "" || s.cfg.Metadata.Empty() {
		return
	}
	s.mu.Lock()
	if s.lookups[r.SeriesID] {
		s.mu.Unlock()
		return
	}
	s.lookups[r.SeriesID] = true
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.lookups, r.SeriesID)
			s.mu.Unlock()
		}()
		s.showYear(context.Background(), r, true)
	}()
}
