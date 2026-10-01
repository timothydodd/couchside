// Package search ranks titles against what someone typed. Matching is
// forgiving: case, accents and punctuation are ignored (via
// metadata.Normalize), and spaces don't matter either, so "spiderman" finds
// "Spider-Man" and "5.1" finds channel 5.1.
package search

import (
	"sort"
	"strings"

	"github.com/timothydodd/couchside/internal/metadata"
)

// Doc is one searchable thing. Keys are the texts it can be found by (a
// title, a channel's number and affiliate); the first one is its display name.
type Doc struct {
	Kind  string
	ID    int64
	Ref   string // a string id, e.g. a channel number
	Order int64  // ties sort by this before the name (programs: start time)
	Until int64  // when non-zero, the doc stops matching at this unix time
	keys  []key
}

type key struct{ norm, compact string }

func NewDoc(kind string, id int64, ref string, order, until int64, keys ...string) Doc {
	d := Doc{Kind: kind, ID: id, Ref: ref, Order: order, Until: until}
	for _, k := range keys {
		if n := metadata.Normalize(k); n != "" {
			d.keys = append(d.keys, key{n, compact(n)})
		}
	}
	return d
}

func compact(norm string) string { return strings.ReplaceAll(norm, " ", "") }

// Query is a normalized search string.
type Query struct {
	norm, compact string
	words         []string
}

func NewQuery(q string) Query {
	n := metadata.Normalize(q)
	return Query{n, compact(n), strings.Fields(n)}
}

func (q Query) Empty() bool { return q.norm == "" }

// Score says how well a doc matches, 0 for not at all. Whole matches beat
// prefixes, which beat a word somewhere in the title, which beats a fragment.
func (q Query) Score(d Doc) int {
	best := 0
	for _, k := range d.keys {
		best = max(best, q.scoreKey(k))
	}
	return best
}

func (q Query) scoreKey(k key) int {
	switch {
	case k.norm == q.norm:
		return 100
	case k.compact == q.compact:
		return 95 // "spiderman" = "spider man"
	case strings.HasPrefix(k.norm, q.norm):
		return 90
	case strings.HasPrefix(k.compact, q.compact):
		return 80
	case q.wordPrefixes(k.norm):
		return 60
	case len(q.compact) >= 3 && strings.Contains(k.compact, q.compact):
		return 30
	}
	return 0
}

// wordPrefixes is true when every query word starts some word of the title,
// in any order: "wars star" finds "Star Wars", "office" finds "The Office (US)".
func (q Query) wordPrefixes(norm string) bool {
	words := strings.Fields(norm)
	for _, qw := range q.words {
		found := false
		for _, w := range words {
			if strings.HasPrefix(w, qw) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return len(q.words) > 0
}

// Hit is a matching doc and its score.
type Hit struct {
	Doc
	Score int
}

// Find returns the best matches per kind, at most limit of each, best first.
// Docs whose Until has passed (at now) are skipped.
func Find(docs []Doc, q Query, limit int, now int64) map[string][]Hit {
	out := map[string][]Hit{}
	if q.Empty() {
		return out
	}
	for _, d := range docs {
		if d.Until != 0 && d.Until <= now {
			continue
		}
		if s := q.Score(d); s > 0 {
			out[d.Kind] = append(out[d.Kind], Hit{d, s})
		}
	}
	for kind, hits := range out {
		sort.SliceStable(hits, func(i, j int) bool {
			a, b := hits[i], hits[j]
			if a.Score != b.Score {
				return a.Score > b.Score
			}
			if a.Order != b.Order {
				return a.Order < b.Order
			}
			// Shorter names first: "Alien" before "Alien: Resurrection".
			if la, lb := len(a.keys[0].norm), len(b.keys[0].norm); la != lb {
				return la < lb
			}
			return a.keys[0].norm < b.keys[0].norm
		})
		if len(hits) > limit {
			out[kind] = hits[:limit]
		}
	}
	return out
}
