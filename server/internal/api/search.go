package api

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/search"
)

// indexTTL is how long the search index is reused. Someone typing a query
// sends a request per pause, so they share one build; new titles show up
// within this long.
const indexTTL = 15 * time.Second

// searchIndex caches every searchable title in memory. A home library is a few
// thousand titles and tens of thousands of episodes, small enough to rank in Go
// with forgiving matching that SQL LIKE can't do.
type searchIndex struct {
	mu    sync.Mutex
	docs  []search.Doc
	built time.Time
	tv    bool
}

func (x *searchIndex) get(ctx context.Context, d *db.DB, withTV bool) ([]search.Doc, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.docs != nil && x.tv == withTV && time.Since(x.built) < indexTTL {
		return x.docs, nil
	}
	now := time.Now()
	rows, err := d.SearchRows(ctx, withTV, now.Unix())
	if err != nil {
		return nil, err
	}
	docs := make([]search.Doc, 0, len(rows))
	for _, r := range rows {
		docs = append(docs, search.NewDoc(r.Kind, r.ID, r.Ref, r.StartAt, r.EndAt, r.Names...))
	}
	x.docs, x.built, x.tv = docs, now, withTV
	return docs, nil
}

type programHit struct {
	Program db.Program  `json:"program"`
	Channel *db.Channel `json:"channel"`
}

type searchResult struct {
	Query    string           `json:"query"`
	Movies   []db.ItemSummary `json:"movies"`
	Series   []db.ItemSummary `json:"series"`
	Episodes []db.PlayInfo    `json:"episodes"`
	People   []db.Person      `json:"people"`
	Channels []db.Channel     `json:"channels"`
	Programs []programHit     `json:"programs"`
}

var searchKinds = []string{"movie", "series", "episode", "person", "channel", "program"}

// search finds movies, shows, episodes, people from the credits and (with
// live TV) channels and guide programs that haven't ended. ?q= is the text, ?limit= caps each group
// (default 10, at most 50), and ?kinds= (comma separated) narrows the groups.
// maxSearchQuery is how much of a query is used, in characters.
const maxSearchQuery = 100

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	qs := r.URL.Query()
	limit, _ := strconv.Atoi(qs.Get("limit"))
	if limit <= 0 {
		limit = 10
	}
	limit = min(limit, 50)
	kinds := map[string]bool{}
	for _, k := range strings.Split(qs.Get("kinds"), ",") {
		if k = strings.TrimSpace(k); k != "" {
			kinds[k] = true
		}
	}
	for k := range kinds {
		if !slices.Contains(searchKinds, k) {
			writeErr(w, badRequest("kinds must be from "+strings.Join(searchKinds, ", ")))
			return
		}
	}
	want := func(k string) bool { return len(kinds) == 0 || kinds[k] }

	// Nobody types a title this long; matching cost grows with every word.
	query := qs.Get("q")
	if r := []rune(query); len(r) > maxSearchQuery {
		query = string(r[:maxSearchQuery])
	}
	out := searchResult{Query: query, Movies: []db.ItemSummary{}, Series: []db.ItemSummary{},
		Episodes: []db.PlayInfo{}, People: []db.Person{}, Channels: []db.Channel{}, Programs: []programHit{}}
	q := search.NewQuery(out.Query)
	if q.Empty() {
		writeJSON(w, http.StatusOK, out)
		return
	}
	docs, err := s.index.get(ctx, s.db, s.tv != nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	hits := search.Find(docs, q, limit, time.Now().Unix())
	ids := func(kind string) []int64 {
		if !want(kind) {
			return nil
		}
		var out []int64
		for _, h := range hits[kind] {
			out = append(out, h.ID)
		}
		return out
	}

	if out.Movies, err = s.db.ItemsByID(ctx, ids("movie")); err != nil {
		writeErr(w, err)
		return
	}
	if out.Series, err = s.db.ItemsByID(ctx, ids("series")); err != nil {
		writeErr(w, err)
		return
	}
	if out.Episodes, err = s.db.EpisodePlays(ctx, ids("episode")); err != nil {
		writeErr(w, err)
		return
	}
	if out.People, err = s.db.PeopleByID(ctx, ids("person")); err != nil {
		writeErr(w, err)
		return
	}
	if s.tv == nil || (!want("channel") && !want("program")) {
		writeJSON(w, http.StatusOK, out)
		return
	}
	// Channels are a short list, so look them up all at once (with this
	// profile's favourites).
	chans, err := s.db.Channels(ctx)
	if err != nil {
		writeErr(w, err)
		return
	}
	byNumber := make(map[string]*db.Channel, len(chans))
	for i := range chans {
		byNumber[chans[i].Number] = &chans[i]
	}
	if want("channel") {
		for _, h := range hits["channel"] {
			if c := byNumber[h.Ref]; c != nil {
				out.Channels = append(out.Channels, *c)
			}
		}
	}
	progs, err := s.db.ProgramsByID(ctx, ids("program"))
	if err != nil {
		writeErr(w, err)
		return
	}
	for _, p := range progs {
		out.Programs = append(out.Programs, programHit{Program: p, Channel: byNumber[p.Channel]})
	}
	writeJSON(w, http.StatusOK, out)
}
