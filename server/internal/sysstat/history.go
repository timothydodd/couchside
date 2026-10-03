package sysstat

import (
	"context"
	"sync"
	"time"
)

// Point is one reading in the history: the whole scope, Couchside itself and
// its ffmpeg/comskip children, plus how many streams the server was producing.
type Point struct {
	T         int64   `json:"t"` // unix seconds
	CPU       float64 `json:"cpu"`
	ServerCPU float64 `json:"serverCpu"`
	EncCPU    float64 `json:"encCpu"`
	Mem       uint64  `json:"mem"`
	ServerMem uint64  `json:"serverMem"`
	EncMem    uint64  `json:"encMem"`
	Encoders  int     `json:"encoders"` // ffmpeg/comskip processes
	Streams   int     `json:"streams"`  // HLS and live TV sessions
}

const (
	Every      = 5 * time.Second
	fineKeep   = time.Hour      // 5s points
	coarseKeep = 24 * time.Hour // 1-minute averages
)

// History keeps recent readings in memory for the System page's charts.
// It starts empty at every restart.
type History struct {
	streams func() int

	mu      sync.Mutex
	fine    []Point
	coarse  []Point
	pending []Point // this minute's fine points, averaged into coarse when it ends
	cores   float64
	memMax  uint64
	scope   string
}

// NewHistory records with its own Sampler, so it doesn't change what Read
// averages over for anyone else. streams may be nil.
func NewHistory(streams func() int) *History {
	return &History{streams: streams}
}

// Run samples every 5 seconds until ctx ends.
func (h *History) Run(ctx context.Context) {
	var s Sampler
	s.Read() // the first reading measures over a short pause; start the clock
	t := time.NewTicker(Every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			st := s.Read()
			if !st.Available {
				continue
			}
			p := Point{
				T: now.Unix(), CPU: st.CPUPercent, ServerCPU: st.Server.CPUPercent, EncCPU: st.Encoders.CPUPercent,
				Mem: st.MemUsed, ServerMem: st.Server.RSS, EncMem: st.Encoders.RSS, Encoders: st.Encoders.Count,
			}
			if h.streams != nil {
				p.Streams = h.streams()
			}
			h.add(p, st)
		}
	}
}

func (h *History) add(p Point, st Stats) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cores, h.memMax, h.scope = st.Cores, st.MemTotal, st.Scope
	if n := len(h.pending); n > 0 && h.pending[n-1].T/60 != p.T/60 {
		h.coarse = keepSince(append(h.coarse, average(h.pending)), p.T-int64(coarseKeep/time.Second))
		h.pending = h.pending[:0]
	}
	h.pending = append(h.pending, p)
	h.fine = keepSince(append(h.fine, p), p.T-int64(fineKeep/time.Second))
}

// Window is what Points returns: the readings plus the scale to draw them on.
type Window struct {
	Every    int     `json:"every"` // seconds between points
	Cores    float64 `json:"cores"`
	MemTotal uint64  `json:"memTotal"`
	Scope    string  `json:"scope"`
	Points   []Point `json:"points"`
}

// Points returns the last d of history: 5s points up to an hour, 1-minute
// averages beyond that (with the current minute's points on the end).
func (h *History) Points(d time.Duration, now time.Time) Window {
	h.mu.Lock()
	defer h.mu.Unlock()
	since := now.Add(-d).Unix()
	w := Window{Cores: h.cores, MemTotal: h.memMax, Scope: h.scope, Points: []Point{}}
	if d <= fineKeep {
		w.Every = int(Every / time.Second)
		w.Points = append(w.Points, keepSince(h.fine, since)...)
		return w
	}
	w.Every = 60
	w.Points = append(w.Points, keepSince(h.coarse, since)...)
	if len(h.pending) > 0 {
		w.Points = append(w.Points, average(h.pending))
	}
	return w
}

// keepSince drops points before since. ps is in time order.
func keepSince(ps []Point, since int64) []Point {
	i := 0
	for i < len(ps) && ps[i].T < since {
		i++
	}
	return ps[i:]
}

// average folds a minute of points into one: means for use, peaks for counts.
func average(ps []Point) Point {
	var a Point
	var mem, smem, emem float64
	for _, p := range ps {
		a.CPU += p.CPU
		a.ServerCPU += p.ServerCPU
		a.EncCPU += p.EncCPU
		mem += float64(p.Mem)
		smem += float64(p.ServerMem)
		emem += float64(p.EncMem)
		a.Encoders = max(a.Encoders, p.Encoders)
		a.Streams = max(a.Streams, p.Streams)
	}
	n := float64(len(ps))
	a.T = ps[0].T / 60 * 60
	a.CPU, a.ServerCPU, a.EncCPU = a.CPU/n, a.ServerCPU/n, a.EncCPU/n
	a.Mem, a.ServerMem, a.EncMem = uint64(mem/n), uint64(smem/n), uint64(emem/n)
	return a
}
