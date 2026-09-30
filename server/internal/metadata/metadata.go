// Package metadata defines the provider plugin contract and a chain that
// asks each configured provider in order.
package metadata

import (
	"context"
	"errors"
)

// Details is a provider's answer for one movie or series.
type Details struct {
	ID           string // provider id; for OMDb this is the IMDb id
	Title        string
	Year         int
	Plot         string
	Genres       []string
	Rated        string
	Rating       *float64
	RuntimeMin   *int
	ImdbID       string
	TotalSeasons *int
	PosterURL    string
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

// Lookup returns the details and the provider that supplied them.
func (c *Chain) Lookup(ctx context.Context, kind Kind, title string, year int, imdbID string) (*Details, Provider, error) {
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
			return d, p, nil
		}
	}
	return nil, nil, errors.Join(errs...)
}
