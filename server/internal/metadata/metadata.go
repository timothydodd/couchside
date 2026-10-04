// Package metadata defines the provider plugin contract and a chain that
// asks each configured provider in order.
package metadata

import (
	"context"
	"errors"
)

// Details is a provider's answer for one movie or series.
type Details struct {
	ID           string // provider id; for OMDb this is the IMDb id, for TMDB "tmdb:movie:<n>"
	Title        string
	Year         int
	Plot         string
	Genres       []string
	Rated        string
	Rating       *float64
	RuntimeMin   *int
	ImdbID       string // the IMDb id, or the provider's id when it knows none ("tmdb:tv:<n>")
	TotalSeasons *int
	PosterURL    string
	BackdropURL  string   // empty when the provider has none (OMDb never does)
	Credits      []Credit // cast (in billing order) and key crew; TMDB only
}

// Credit is one person's part in a title. PersonID is TMDB's person id.
type Credit struct {
	PersonID    int64
	Name        string
	ProfilePath string // TMDB image path ("/abc.jpg"), "" when there's no photo
	Kind        string // cast | crew
	Role        string // the character for cast, the job for crew ("Director")
	Order       int
}

type Episode struct {
	Season, Episode int
	Title, Released string
	Rating          *float64
	ImdbID          string
}

// Kind is what we're looking up.
type Kind string

const (
	Movie  Kind = "movie"
	Series Kind = "series"
)

// Provider is the plugin contract. Lookup returns (nil, nil) when there is
// simply no match; errors mean "try again later" (network, quota, bad key).
type Provider interface {
	Name() string
	Lookup(ctx context.Context, kind Kind, title string, year int) (*Details, error)
	ByImdbID(ctx context.Context, kind Kind, imdbID string) (*Details, error)
	Season(ctx context.Context, seriesImdbID string, season int) ([]Episode, error)
}

// Chain tries providers in order; the first confident match wins.
type Chain struct{ Providers []Provider }

func (c *Chain) Empty() bool { return c == nil || len(c.Providers) == 0 }

func (c *Chain) Names() []string {
	out := []string{}
	if c != nil {
		for _, p := range c.Providers {
			out = append(out, p.Name())
		}
	}
	return out
}

// Lookup returns the details and the provider that supplied them. skipped is
// set when an earlier provider failed (an outage, a rate limit) and a later
// one answered: the answer is real but may lack what the earlier provider
// alone has (cast, crew, backdrops), so the caller shouldn't treat those as
// gone. err means no provider matched and at least one couldn't be asked.
func (c *Chain) Lookup(ctx context.Context, kind Kind, title string, year int, imdbID string) (d *Details, p Provider, skipped, err error) {
	var errs []error
	for _, p := range c.Providers {
		var d *Details
		var err error
		if imdbID != "" {
			d, err = p.ByImdbID(ctx, kind, imdbID)
		} else {
			d, err = p.Lookup(ctx, kind, title, year)
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if d != nil {
			return d, p, errors.Join(errs...), nil
		}
	}
	return nil, nil, nil, errors.Join(errs...)
}

// SearchResult is one candidate when picking a match by hand.
type SearchResult struct {
	ImdbID string `json:"imdbId"`
	Title  string `json:"title"`
	Year   string `json:"year"` // as the provider writes it, e.g. "2016–2021"
	Poster string `json:"poster"`
}

// TitleSearcher is implemented by providers that can list title matches.
type TitleSearcher interface {
	SearchTitles(ctx context.Context, kind Kind, query string, year int) ([]SearchResult, error)
}

// SearchTitles asks the first provider that can search.
func (c *Chain) SearchTitles(ctx context.Context, kind Kind, query string, year int) ([]SearchResult, error) {
	if c != nil {
		for _, p := range c.Providers {
			if s, ok := p.(TitleSearcher); ok {
				return s.SearchTitles(ctx, kind, query, year)
			}
		}
	}
	return nil, errors.New("no metadata provider can search; set TMDB_API_KEY or OMDB_API_KEY")
}
