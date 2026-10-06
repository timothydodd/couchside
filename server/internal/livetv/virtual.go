package livetv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math/rand"
	"net/url"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/parse"
	"github.com/timothydodd/couchside/internal/probe"
	"github.com/timothydodd/couchside/internal/usererr"
)

// Virtual channels are Couchside's own channels: shows and movies from the
// library played back to back like broadcast TV, optionally with commercials
// from a folder of clips. Each has a lineup row, so the guide, channel list,
// favourites and players treat it like a tuner channel.
//
// The schedule ("playout") is built ahead and stored, then extended as time
// passes, so restarts and library changes never shift what's on now. A guide
// program covers its own commercials, as on real TV.

const (
	virtualAhead     = 36 * time.Hour // schedule this far ahead
	virtualMinAhead  = 24 * time.Hour // extend once less than this is left
	virtualMinFileMs = 10_000         // shorter files aren't programs
	maxAlignFillMs   = 10 * 60_000    // never pad more than this to reach a boundary
)

// VirtualConfig is what a virtual channel plays. Filters combine with AND;
// list filters match any of their values. Empty means no restriction.
type VirtualConfig struct {
	Libraries     []int64  `json:"libraries"`
	Kinds         []string `json:"kinds"` // movie, series
	Genres        []string `json:"genres"`
	ExcludeGenres []string `json:"excludeGenres"`
	YearFrom      int      `json:"yearFrom"`
	YearTo        int      `json:"yearTo"`
	MinRating     float64  `json:"minRating"`
	Items         []int64  `json:"items"` // only these movies and shows
	Order         string   `json:"order"` // shuffle | sequential
	Filler        Filler   `json:"filler"`
	// Stream makes this a stream channel: it shows a live stream from
	// somewhere else, and everything above is unused.
	Stream *StreamSource `json:"stream,omitempty"`
}

// StreamSource is a live stream a channel passes on: an IPTV feed, a camera
// behind a restreamer, another program's output. Couchside reads it only
// while someone is watching, and converts it like a tuner channel.
type StreamSource struct {
	URL string `json:"url"` // http or https: an HLS playlist or an MPEG-TS stream
}

// check validates the address. Only http(s): ffmpeg opens whatever it's
// given, and a "file:" or "concat:" address would read the server's disk.
func (s *StreamSource) check() error {
	s.URL = strings.TrimSpace(s.URL)
	u, err := url.Parse(s.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return usererr.New("the stream address must start with http:// or https://")
	}
	if len(s.URL) > 2000 {
		return usererr.New("the stream address is too long")
	}
	return nil
}

// Filler is the optional commercials: a folder of clips played between
// programs (to start the next one on a boundary, like :00 and :30) and in
// breaks inside them.
type Filler struct {
	Folder      string `json:"folder"`      // "" = no commercials
	Align       int    `json:"align"`       // minutes: start programs on these boundaries; 0 = back to back
	BreakEvery  int    `json:"breakEvery"`  // minutes between breaks inside a program; 0 = none
	BreakLength int    `json:"breakLength"` // seconds per break (whole clips, so about)
}

// Validate checks a config and fills in defaults.
func (c *VirtualConfig) Validate() error {
	// Lists are stored as [] and never null: the channel editor reads them.
	if c.Libraries == nil {
		c.Libraries = []int64{}
	}
	if c.Kinds == nil {
		c.Kinds = []string{}
	}
	if c.Genres == nil {
		c.Genres = []string{}
	}
	if c.ExcludeGenres == nil {
		c.ExcludeGenres = []string{}
	}
	if c.Items == nil {
		c.Items = []int64{}
	}
	if c.Stream != nil {
		if err := c.Stream.check(); err != nil {
			return err
		}
	}
	if c.Order == "" {
		c.Order = "shuffle"
	}
	if c.Order != "shuffle" && c.Order != "sequential" {
		return usererr.New("order must be shuffle or sequential")
	}
	for _, k := range c.Kinds {
		if k != "movie" && k != "series" {
			return usererr.New("kinds are movie and series")
		}
	}
	if c.YearFrom != 0 && c.YearTo != 0 && c.YearFrom > c.YearTo {
		return usererr.New("the year range is backwards")
	}
	f := &c.Filler
	if !slices.Contains([]int{0, 15, 30, 60}, f.Align) {
		return usererr.New("align to 15, 30 or 60 minutes, or 0")
	}
	if f.BreakEvery < 0 || f.BreakEvery > 120 || f.BreakLength < 0 || f.BreakLength > 600 {
		return usererr.New("breaks are every 0 to 120 minutes and up to 10 minutes long")
	}
	if f.BreakEvery > 0 && f.BreakLength == 0 {
		f.BreakLength = 120
	}
	return nil
}

// ParseVirtualConfig reads a stored config.
func ParseVirtualConfig(raw []byte) (VirtualConfig, error) {
	var c VirtualConfig
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, err
	}
	return c, c.Validate()
}

// --- programs ------------------------------------------------------------------

// vprogram is one schedulable thing: a movie (all its parts) or an episode.
type vprogram struct {
	key   string // "m<item>" or "e<item>:<season>:<episode>"
	unit  string // what sequential order cycles through: a show, or "movies"
	files []db.VirtualFile
	ms    int64
}

func (p vprogram) first() db.VirtualFile { return p.files[0] }

// programsFor groups playable files into programs that pass the filters,
// keeping the best copy of each (and every part of a split movie).
func programsFor(files []db.VirtualFile, c VirtualConfig) []vprogram {
	type key struct {
		item    int64
		season  int
		episode int
		part    int
		isMovie bool
	}
	best := map[key]db.VirtualFile{}
	for _, f := range files {
		if !c.matches(f) {
			continue
		}
		k := key{item: f.ItemID, season: f.Season, episode: f.Episode, isMovie: f.Kind == "movie"}
		if f.Role == "part" {
			k.part = max(1, f.PartNo)
		}
		if cur, ok := best[k]; !ok || f.Height > cur.Height || (f.Height == cur.Height && f.FileID < cur.FileID) {
			best[k] = f
		}
	}
	progs := map[string]*vprogram{}
	for k, f := range best {
		pk := fmt.Sprintf("e%d:%d:%d", f.ItemID, f.Season, f.Episode)
		unit := fmt.Sprintf("s%d", f.ItemID)
		if k.isMovie {
			pk, unit = fmt.Sprintf("m%d", f.ItemID), "movies"
		}
		p := progs[pk]
		if p == nil {
			p = &vprogram{key: pk, unit: unit}
			progs[pk] = p
		}
		p.files = append(p.files, f)
	}
	out := make([]vprogram, 0, len(progs))
	for _, p := range progs {
		// A split movie plays its parts in order; ignore a stray whole copy beside them.
		if len(p.files) > 1 {
			parts := p.files[:0]
			for _, f := range p.files {
				if f.Role == "part" {
					parts = append(parts, f)
				}
			}
			if len(parts) > 0 {
				p.files = parts
			}
			sort.Slice(p.files, func(i, j int) bool { return p.files[i].PartNo < p.files[j].PartNo })
		}
		for _, f := range p.files {
			p.ms += int64(f.DurationSec * 1000)
		}
		if p.ms >= virtualMinFileMs {
			out = append(out, *p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return lessProgram(out[i].first(), out[j].first()) })
	return out
}

// lessProgram is sequential order: shows by title then episode, movies by year.
func lessProgram(a, b db.VirtualFile) bool {
	if a.Kind != b.Kind {
		return a.Kind == "series"
	}
	if a.Kind == "movie" {
		if a.Year != b.Year {
			return a.Year < b.Year
		}
		return a.SortTitle < b.SortTitle
	}
	if a.ItemID != b.ItemID {
		return a.SortTitle < b.SortTitle || (a.SortTitle == b.SortTitle && a.ItemID < b.ItemID)
	}
	if a.Season != b.Season {
		return a.Season < b.Season
	}
	return a.Episode < b.Episode
}

func (c VirtualConfig) matches(f db.VirtualFile) bool {
	if len(c.Kinds) > 0 && !slices.Contains(c.Kinds, f.Kind) {
		return false
	}
	if len(c.Items) > 0 && !slices.Contains(c.Items, f.ItemID) {
		return false
	}
	if len(c.Genres) > 0 && !anyFold(f.Genres, c.Genres) {
		return false
	}
	if anyFold(f.Genres, c.ExcludeGenres) {
		return false
	}
	if c.YearFrom > 0 && (f.Year == 0 || f.Year < c.YearFrom) {
		return false
	}
	if c.YearTo > 0 && (f.Year == 0 || f.Year > c.YearTo) {
		return false
	}
	return c.MinRating <= 0 || f.Rating >= c.MinRating
}

func anyFold(have, want []string) bool {
	for _, h := range have {
		for _, w := range want {
			if strings.EqualFold(h, w) {
				return true
			}
		}
	}
	return false
}

// --- picking the next program ---------------------------------------------------

// virtualState is where the builder left off, stored with the channel.
type virtualState struct {
	Deck []string       `json:"deck,omitempty"` // shuffle: what's left of this round
	Pos  map[string]int `json:"pos,omitempty"`  // sequential: next index into each unit
	Turn int            `json:"turn,omitempty"` // sequential: which unit is next
	Last string         `json:"last,omitempty"` // never the same program twice in a row
}

type picker struct {
	order string
	progs []vprogram
	byKey map[string]int
	units []string
	unitP map[string][]int // unit → indexes into progs, in order
	st    *virtualState
	rng   *rand.Rand
}

func newPicker(order string, progs []vprogram, st *virtualState, rng *rand.Rand) *picker {
	p := &picker{order: order, progs: progs, byKey: map[string]int{}, unitP: map[string][]int{}, st: st, rng: rng}
	for i, pr := range progs {
		p.byKey[pr.key] = i
		if _, ok := p.unitP[pr.unit]; !ok {
			p.units = append(p.units, pr.unit)
		}
		p.unitP[pr.unit] = append(p.unitP[pr.unit], i)
	}
	if st.Pos == nil {
		st.Pos = map[string]int{}
	}
	return p
}

func (p *picker) next() vprogram {
	if p.order == "sequential" {
		u := p.units[p.st.Turn%len(p.units)]
		p.st.Turn = (p.st.Turn + 1) % len(p.units)
		list := p.unitP[u]
		i := p.st.Pos[u] % len(list)
		p.st.Pos[u] = (i + 1) % len(list)
		pr := p.progs[list[i]]
		p.st.Last = pr.key
		return pr
	}
	for range 2 * len(p.progs) {
		if len(p.st.Deck) == 0 {
			p.st.Deck = make([]string, len(p.progs))
			for i, pr := range p.progs {
				p.st.Deck[i] = pr.key
			}
			p.rng.Shuffle(len(p.st.Deck), func(i, j int) { p.st.Deck[i], p.st.Deck[j] = p.st.Deck[j], p.st.Deck[i] })
			if len(p.st.Deck) > 1 && p.st.Deck[0] == p.st.Last {
				p.st.Deck[0], p.st.Deck[1] = p.st.Deck[1], p.st.Deck[0]
			}
		}
		k := p.st.Deck[0]
		p.st.Deck = p.st.Deck[1:]
		if i, ok := p.byKey[k]; ok { // the library may have changed since the deck was dealt
			p.st.Last = k
			return p.progs[i]
		}
	}
	pr := p.progs[p.rng.Intn(len(p.progs))]
	p.st.Last = pr.key
	return pr
}

// --- filler --------------------------------------------------------------------

// fillerClips lists the clips in a folder (and below), probing any it hasn't seen.
func (s *Service) fillerClips(ctx context.Context, folder string) ([]db.FillerClip, error) {
	known, err := s.db.FillerClips(ctx)
	if err != nil {
		return nil, err
	}
	var out []db.FillerClip
	err = filepath.WalkDir(folder, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !parse.IsVideo(p) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		c, ok := known[p]
		if !ok || c.Mtime != info.ModTime().Unix() {
			pi, err := probe.Probe(ctx, s.cfg.FFprobe, p)
			if err != nil || pi.DurationSec == nil || *pi.DurationSec < 1 || pi.VideoCodec == "" {
				return nil // not something we can play
			}
			c = db.FillerClip{Path: p, Mtime: info.ModTime().Unix(), DurationMs: int64(*pi.DurationSec * 1000), HasAudio: pi.AudioCodec != ""}
			_ = s.db.SaveFillerClip(ctx, c)
		}
		out = append(out, c)
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	// Forget clips that have left the folder. Not when the walk found none:
	// that's an unmounted share, and probing them all again is slow.
	if err == nil && len(out) > 0 {
		here := map[string]bool{}
		for _, c := range out {
			here[c.Path] = true
		}
		prefix := strings.TrimRight(folder, `/\`) + string(filepath.Separator)
		var gone []string
		for p := range known {
			if strings.HasPrefix(p, prefix) && !here[p] {
				gone = append(gone, p)
			}
		}
		if len(gone) > 0 {
			_ = s.db.DeleteFillerClips(ctx, gone)
		}
	}
	return out, err
}

type fillerPool struct {
	clips []db.FillerClip
	rng   *rand.Rand
	last  string
}

// take returns clips adding up to about ms: whole clips until it's reached,
// or, when exact, cut so they add up to ms exactly.
func (f *fillerPool) take(ms int64, exact bool) []db.FillerClip {
	var out []db.FillerClip
	for total := int64(0); total < ms && len(f.clips) > 0; {
		if exact && ms-total < 1000 {
			break // not worth a clip; the next program starts a moment early
		}
		c := f.clips[f.rng.Intn(len(f.clips))]
		if len(f.clips) > 1 && c.Path == f.last {
			continue
		}
		f.last = c.Path
		if exact && total+c.DurationMs > ms {
			c.DurationMs = ms - total
		}
		out = append(out, c)
		total += c.DurationMs
	}
	return out
}

// --- building the schedule -----------------------------------------------------

// ErrNothingToPlay means no file in the library matches a channel's filters.
var ErrNothingToPlay = usererr.New("nothing in the library matches this channel")

// extendVirtual builds a channel's schedule ahead when it's running low.
func (s *Service) extendVirtual(ctx context.Context, vc db.VirtualChannel, now time.Time) error {
	s.virtualMu.Lock()
	defer s.virtualMu.Unlock()
	return s.extendVirtualLocked(ctx, vc, now)
}

// extendVirtualLocked is extendVirtual with virtualMu held.
func (s *Service) extendVirtualLocked(ctx context.Context, vc db.VirtualChannel, now time.Time) error {
	// Re-read: the channel's state may have moved on while waiting for the lock.
	if cur, err := s.db.VirtualChannel(ctx, vc.ID); err == nil {
		vc = cur
	} else {
		return err
	}
	end, err := s.db.PlayoutEnd(ctx, vc.ID)
	if err != nil {
		return err
	}
	nowMs := now.UnixMilli()
	if end-nowMs > virtualMinAhead.Milliseconds() {
		return nil
	}
	cfg, err := ParseVirtualConfig(vc.Config)
	if err != nil {
		return err
	}
	if cfg.Stream != nil {
		// Nothing to schedule: the guide just says the channel is on.
		return s.db.ExtendPlayout(ctx, vc.ID, nil, streamGuide(vc.Number, vc.Name, now, virtualAhead), vc.State)
	}
	files, err := s.db.VirtualFiles(ctx, cfg.Libraries)
	if err != nil {
		return err
	}
	progs := programsFor(files, cfg)
	if len(progs) == 0 {
		return ErrNothingToPlay
	}
	var st virtualState
	_ = json.Unmarshal([]byte(vc.State), &st)
	rng := rand.New(rand.NewSource(now.UnixNano()))
	pool := &fillerPool{rng: rng}
	if cfg.Filler.Folder != "" {
		if pool.clips, err = s.fillerClips(ctx, cfg.Filler.Folder); err != nil {
			slog.Warn("virtual channel: can't read the commercials folder", "channel", vc.Number, "folder", cfg.Filler.Folder, "err", err)
		}
	}
	// A new (or long-stopped) schedule starts at the last boundary, so the
	// first tune lands part way into a program like real TV.
	t := end
	if t < nowMs {
		step := int64(max(cfg.Filler.Align, 30)) * 60_000
		t = nowMs - nowMs%step
	}
	b := scheduleBuilder{cfg: cfg, pool: pool, number: vc.Number}
	pick := newPicker(cfg.Order, progs, &st, rng)
	for t < nowMs+virtualAhead.Milliseconds() {
		t = b.add(t, pick.next())
	}
	state, _ := json.Marshal(st)
	if err := s.db.ExtendPlayout(ctx, vc.ID, b.pieces, b.guide, string(state)); err != nil {
		return err
	}
	slog.Info("virtual channel scheduled", "channel", vc.Number, "programs", len(b.guide), "through", time.UnixMilli(t).Format(time.RFC3339))
	return nil
}

type scheduleBuilder struct {
	cfg    VirtualConfig
	pool   *fillerPool
	number string
	pieces []db.PlayoutPiece
	guide  []db.Program
}

// add schedules one program starting at t (ms) and returns when the next starts.
func (b *scheduleBuilder) add(t int64, p vprogram) int64 {
	start := t
	at := start / 1000
	breakEvery := int64(b.cfg.Filler.BreakEvery) * 60_000
	withBreaks := len(b.pool.clips) > 0 && breakEvery > 0
	for _, f := range p.files {
		fileMs := int64(f.DurationSec * 1000)
		// Breaks fall every breakEvery into the file, but not in its last few minutes.
		for in := int64(0); in < fileMs; {
			seg := fileMs - in
			if withBreaks && seg > breakEvery+3*60_000 {
				seg = breakEvery
			}
			b.pieces = append(b.pieces, db.PlayoutPiece{StartMs: t, EndMs: t + seg, Path: f.Path, InMs: in, HasAudio: f.HasAudio, ProgramAt: at})
			t += seg
			in += seg
			if in < fileMs {
				t = b.filler(t, b.pool.take(int64(b.cfg.Filler.BreakLength)*1000, false), at)
			}
		}
	}
	// Pad to the next boundary with commercials, unless that's a long way off.
	if align := int64(b.cfg.Filler.Align) * 60_000; align > 0 && len(b.pool.clips) > 0 {
		if gap := (align - t%align) % align; gap > 0 && gap <= maxAlignFillMs {
			t = b.filler(t, b.pool.take(gap, true), at)
		}
	}
	b.guide = append(b.guide, guideProgram(b.number, p, at, t/1000))
	return t
}

func (b *scheduleBuilder) filler(t int64, clips []db.FillerClip, at int64) int64 {
	for _, c := range clips {
		b.pieces = append(b.pieces, db.PlayoutPiece{StartMs: t, EndMs: t + c.DurationMs, Path: c.Path, Filler: true, HasAudio: c.HasAudio, ProgramAt: at})
		t += c.DurationMs
	}
	return t
}

// guideProgram describes a program for the guide, from the library's metadata.
func guideProgram(channel string, p vprogram, start, end int64) db.Program {
	f := p.first()
	g := db.Program{Channel: channel, StartAt: start, EndAt: end, Title: f.Title, Synopsis: f.Plot, Categories: f.Genres}
	img := fmt.Sprintf("/api/artwork/items/%d/poster?v=%d", f.ItemID, f.UpdatedAt)
	if f.HasBackdrop {
		img = fmt.Sprintf("/api/artwork/items/%d/backdrop?v=%d", f.ItemID, f.UpdatedAt)
	}
	if f.Kind == "movie" {
		if f.Year > 0 {
			g.EpisodeTitle = fmt.Sprint(f.Year)
		}
		g.Categories = append([]string{"Movie"}, f.Genres...)
	} else {
		g.EpisodeTitle = f.EpisodeTitle
		g.EpisodeNum = fmt.Sprintf("S%02dE%02d", f.Season, f.Episode)
		if f.HasStill {
			img = fmt.Sprintf("/api/artwork/files/%d/still", f.FileID)
		}
		if t, err := time.Parse("2006-01-02", f.AirDate); err == nil {
			oa := t.Unix()
			g.OriginalAirdate = &oa
		}
	}
	g.ImageURL = db.ImageURL(img)
	return g
}

// streamGuide is a stream channel's listings: Couchside doesn't know what the
// stream shows, so each hour from the current one is an entry with the
// channel's name. Writing them again replaces the same entries.
func streamGuide(number, name string, now time.Time, ahead time.Duration) []db.Program {
	var out []db.Program
	for t := now.Truncate(time.Hour); t.Before(now.Add(ahead)); t = t.Add(time.Hour) {
		out = append(out, db.Program{Channel: number, StartAt: t.Unix(), EndAt: t.Add(time.Hour).Unix(), Title: name,
			Synopsis: "A live stream.", Categories: []string{"Live"}})
	}
	return out
}

// --- running -----------------------------------------------------------------

// virtualLoop keeps every virtual channel's schedule built ahead.
func (s *Service) virtualLoop(ctx context.Context) {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for {
		s.extendAllVirtual(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-s.wakeVirtual:
		}
	}
}

func (s *Service) extendAllVirtual(ctx context.Context) {
	chans, err := s.db.VirtualChannels(ctx)
	if err != nil {
		slog.Error("virtual channels", "err", err)
		return
	}
	s.mu.Lock()
	s.virtualCount = len(chans)
	s.mu.Unlock()
	now := time.Now()
	for _, vc := range chans {
		err := s.extendVirtual(ctx, vc, now)
		s.setVirtualErr(vc.ID, err)
		if err != nil && !errors.Is(err, ErrNothingToPlay) && ctx.Err() == nil {
			slog.Warn("virtual channel", "channel", vc.Number, "err", err)
		}
	}
	_ = s.db.PrunePlayout(ctx, now.Add(-6*time.Hour).UnixMilli())
	// The guide refresh prunes listings too, but it only runs with a tuner.
	_ = s.db.PrunePrograms(ctx, now.Add(-6*time.Hour).Unix())
}

func (s *Service) setVirtualErr(id int64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		delete(s.virtualErr, id)
	} else {
		s.virtualErr[id] = err.Error()
	}
}

// VirtualError is why a channel has no schedule ("" when it's fine).
func (s *Service) VirtualError(id int64) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.virtualErr[id]
}

// RebuildVirtual (re)builds schedules now, after a channel is added or changed.
func (s *Service) RebuildVirtual() {
	select {
	case s.wakeVirtual <- struct{}{}:
	default:
	}
}

// PreviewVirtual lays out the next hours of a config without saving it, for
// the setup wizard.
func (s *Service) PreviewVirtual(ctx context.Context, cfg VirtualConfig, hours int) ([]db.Program, int, error) {
	if cfg.Stream != nil {
		return streamGuide("preview", "Live stream", time.Now(), time.Duration(hours)*time.Hour), 0, nil
	}
	files, err := s.db.VirtualFiles(ctx, cfg.Libraries)
	if err != nil {
		return nil, 0, err
	}
	progs := programsFor(files, cfg)
	if len(progs) == 0 {
		return []db.Program{}, 0, nil
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	pool := &fillerPool{rng: rng}
	if cfg.Filler.Folder != "" {
		pool.clips, _ = s.fillerClips(ctx, cfg.Filler.Folder)
	}
	now := time.Now().UnixMilli()
	t := now - now%(int64(max(cfg.Filler.Align, 30))*60_000)
	b := scheduleBuilder{cfg: cfg, pool: pool, number: "preview"}
	pick := newPicker(cfg.Order, progs, &virtualState{}, rng)
	for t < now+int64(hours)*3_600_000 && len(b.guide) < 200 {
		t = b.add(t, pick.next())
	}
	return b.guide, len(progs), nil
}

// --- managing channels -----------------------------------------------------------

// SaveVirtual creates a channel (id 0) or replaces one's settings, then
// rebuilds its schedule. Anyone watching it is cut off so they re-tune.
func (s *Service) SaveVirtual(ctx context.Context, id int64, number, name string, cfg VirtualConfig) (int64, error) {
	if err := cfg.Validate(); err != nil {
		return 0, err
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return 0, err
	}
	if id == 0 {
		if id, err = s.db.CreateVirtualChannel(ctx, number, name, raw, channelSortKey(number)); err != nil {
			return 0, err
		}
	} else {
		// Under virtualMu, with the rebuild: a schedule extension already
		// running has read the old settings, and would otherwise put 36 hours
		// of the old schedule back after the update cleared it.
		s.virtualMu.Lock()
		defer s.virtualMu.Unlock()
		old, err := s.db.VirtualChannel(ctx, id)
		if err != nil {
			return 0, err
		}
		if err := s.db.UpdateVirtualChannel(ctx, id, number, name, raw, channelSortKey(number)); err != nil {
			return 0, err
		}
		s.live.stopKey("vc:" + old.Number)
		vc, err := s.db.VirtualChannel(ctx, id)
		if err == nil {
			err = s.extendVirtualLocked(ctx, vc, time.Now())
			s.setVirtualErr(id, err)
		}
		s.RebuildVirtual() // recounts channels too
		return id, nil
	}
	vc, err := s.db.VirtualChannel(ctx, id)
	if err == nil {
		err = s.extendVirtual(ctx, vc, time.Now())
		s.setVirtualErr(id, err)
	}
	s.RebuildVirtual() // recounts channels too
	return id, nil
}

// DeleteVirtual removes a channel, its schedule and its guide.
func (s *Service) DeleteVirtual(ctx context.Context, id int64) error {
	vc, err := s.db.VirtualChannel(ctx, id)
	if err != nil {
		return err
	}
	s.virtualMu.Lock() // not while its schedule is being extended
	err = s.db.DeleteVirtualChannel(ctx, id)
	s.virtualMu.Unlock()
	if err != nil {
		return err
	}
	s.live.stopKey("vc:" + vc.Number)
	s.setVirtualErr(id, nil)
	s.RebuildVirtual()
	return nil
}
