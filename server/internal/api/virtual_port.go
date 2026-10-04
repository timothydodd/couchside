package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/livetv"
)

// Exporting and importing virtual channels, to copy a line-up to another
// server or keep it beside a deployment. A channel's settings name libraries
// and titles by id, which mean nothing elsewhere, so the file names them
// instead: libraries by name, titles by title, year and kind. Importing looks
// them up again; a channel whose number already belongs to one of your
// channels replaces it.

const channelFileKind = "couchside-channels"

type channelFile struct {
	Kind     string            `json:"kind"` // channelFileKind
	Version  int               `json:"version"`
	Channels []portableChannel `json:"channels"`
}

type portableChannel struct {
	Number string         `json:"number"`
	Name   string         `json:"name"`
	Config portableConfig `json:"config"`
}

// portableConfig is livetv.VirtualConfig with names in place of ids.
type portableConfig struct {
	Libraries     []string        `json:"libraries"`
	Kinds         []string        `json:"kinds"`
	Genres        []string        `json:"genres"`
	ExcludeGenres []string        `json:"excludeGenres"`
	YearFrom      int             `json:"yearFrom"`
	YearTo        int             `json:"yearTo"`
	MinRating     float64         `json:"minRating"`
	Items         []portableTitle `json:"items"`
	Order         string          `json:"order"`
	Filler        livetv.Filler   `json:"filler"`
}

type portableTitle struct {
	Title string `json:"title"`
	Year  int    `json:"year,omitempty"`
	Kind  string `json:"kind"` // movie | series
}

func (t portableTitle) key() string {
	return fmt.Sprintf("%s|%d|%s", strings.ToLower(strings.TrimSpace(t.Title)), t.Year, t.Kind)
}

// channelNames is what ids are swapped for names with: this server's
// libraries and the titles a channel can play.
type channelNames struct {
	libraries map[int64]string
	titles    map[int64]portableTitle
}

func (s *Server) channelNames(r *http.Request) (channelNames, error) {
	n := channelNames{libraries: map[int64]string{}, titles: map[int64]portableTitle{}}
	libs, err := s.db.Libraries(r.Context())
	if err != nil {
		return n, err
	}
	for _, l := range libs {
		n.libraries[l.ID] = l.Name
	}
	files, err := s.db.VirtualFiles(r.Context(), nil)
	if err != nil {
		return n, err
	}
	for _, f := range files {
		n.titles[f.ItemID] = portableTitle{Title: f.Title, Year: f.Year, Kind: f.Kind}
	}
	return n, nil
}

// export swaps a config's ids for names. Ids that no longer exist are left out.
func (n channelNames) export(c livetv.VirtualConfig) portableConfig {
	p := portableConfig{Libraries: []string{}, Kinds: c.Kinds, Genres: c.Genres, ExcludeGenres: c.ExcludeGenres,
		YearFrom: c.YearFrom, YearTo: c.YearTo, MinRating: c.MinRating, Items: []portableTitle{}, Order: c.Order, Filler: c.Filler}
	for _, id := range c.Libraries {
		if name, ok := n.libraries[id]; ok {
			p.Libraries = append(p.Libraries, name)
		}
	}
	for _, id := range c.Items {
		if t, ok := n.titles[id]; ok {
			p.Items = append(p.Items, t)
		}
	}
	return p
}

// resolve swaps names back for this server's ids. Names it can't find come
// back as warnings; when none of a channel's libraries or none of its titles
// are here it's an error, because the channel would play everything instead.
func (n channelNames) resolve(p portableConfig) (livetv.VirtualConfig, []string, error) {
	c := livetv.VirtualConfig{Kinds: p.Kinds, Genres: p.Genres, ExcludeGenres: p.ExcludeGenres,
		YearFrom: p.YearFrom, YearTo: p.YearTo, MinRating: p.MinRating, Order: p.Order, Filler: p.Filler}
	var warnings []string
	libs := map[string]int64{}
	for id, name := range n.libraries {
		libs[strings.ToLower(name)] = id
	}
	for _, name := range p.Libraries {
		if id, ok := libs[strings.ToLower(strings.TrimSpace(name))]; ok {
			c.Libraries = append(c.Libraries, id)
		} else {
			warnings = append(warnings, fmt.Sprintf("there's no library called %q", name))
		}
	}
	if len(p.Libraries) > 0 && len(c.Libraries) == 0 {
		return c, nil, errors.New("none of its libraries are here (" + strings.Join(p.Libraries, ", ") + "): add them first, with the same names")
	}
	titles := map[string]int64{}
	for id, t := range n.titles {
		titles[t.key()] = id
	}
	for _, t := range p.Items {
		if id, ok := titles[t.key()]; ok {
			c.Items = append(c.Items, id)
		} else {
			warnings = append(warnings, fmt.Sprintf("%q isn't in the library", t.Title))
		}
	}
	if len(p.Items) > 0 && len(c.Items) == 0 {
		return c, nil, errors.New("none of its picked titles are in the library yet")
	}
	return c, warnings, nil
}

func (s *Server) exportVirtual(w http.ResponseWriter, r *http.Request) {
	chans, err := s.db.VirtualChannels(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	names, err := s.channelNames(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	out := channelFile{Kind: channelFileKind, Version: 1, Channels: []portableChannel{}}
	for _, ch := range chans {
		cfg, err := livetv.ParseVirtualConfig(ch.Config)
		if err != nil {
			continue // a stored config today's rules reject: nothing worth copying
		}
		out.Channels = append(out.Channels, portableChannel{Number: ch.Number, Name: ch.Name, Config: names.export(cfg)})
	}
	body, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="couchside-channels-`+time.Now().Format("2006-01-02")+`.json"`)
	_, _ = w.Write(append(body, '\n'))
}

type importResult struct {
	Created  int      `json:"created"`
	Updated  int      `json:"updated"`
	Skipped  []string `json:"skipped"`  // channels left out, and why
	Warnings []string `json:"warnings"` // imported, but without something the file named
}

func (s *Server) importVirtual(w http.ResponseWriter, r *http.Request) {
	var in channelFile
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if in.Kind != channelFileKind || len(in.Channels) == 0 {
		writeErr(w, badRequest("that isn't a Couchside channels file (export one from Your channels to see the format)"))
		return
	}
	if len(in.Channels) > 200 {
		writeErr(w, badRequest("that file has more than 200 channels"))
		return
	}
	ctx := r.Context()
	names, err := s.channelNames(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	existing, err := s.db.VirtualChannels(ctx)
	if err != nil {
		writeErr(w, err)
		return
	}
	byNumber := map[string]int64{}
	for _, ch := range existing {
		byNumber[ch.Number] = ch.ID
	}
	res := importResult{Skipped: []string{}, Warnings: []string{}}
	for _, pc := range in.Channels {
		label := strings.TrimSpace(pc.Number + " " + pc.Name)
		skip := func(why string) { res.Skipped = append(res.Skipped, label+": "+why) }
		cfg, warnings, err := names.resolve(pc.Config)
		if err != nil {
			skip(err.Error())
			continue
		}
		v := virtualIn{Number: pc.Number, Name: pc.Name, Config: cfg}
		if err := s.checkVirtual(&v); err != nil {
			var he httpError
			if !errors.As(err, &he) {
				writeErr(w, err)
				return
			}
			skip(he.msg)
			continue
		}
		id := byNumber[v.Number]
		newID, err := s.tv.SaveVirtual(ctx, id, v.Number, v.Name, v.Config)
		switch {
		case errors.Is(err, db.ErrNumberTaken):
			skip("a tuner channel already has that number")
			continue
		case err != nil:
			writeErr(w, err)
			return
		}
		byNumber[v.Number] = newID
		if id == 0 {
			res.Created++
		} else {
			res.Updated++
		}
		for _, wn := range warnings {
			res.Warnings = append(res.Warnings, label+": "+wn)
		}
	}
	writeJSON(w, http.StatusOK, res)
}
