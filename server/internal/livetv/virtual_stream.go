package livetv

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/transcode"
)

// A virtual channel's stream plays its schedule one piece at a time: one
// ffmpeg run per piece, each writing its own HLS segments, which are merged
// into the session's playlist (with a discontinuity between runs). The first
// run goes as fast as it can, so a viewer gets a few seconds of buffer
// straight away; the rest run at real time (-re), with a fast catch-up run
// when they've drifted too far behind the schedule.

const (
	virtualLeadMs   = 8000   // start this far behind the schedule, and catch up fast
	virtualMaxLagMs = 10_000 // real-time runs end a little late; catch up once this far behind
	virtualWindow   = 900    // segments kept in the playlist (30 minutes)
	virtualResyncMs = 30_000
)

// playoutSource returns schedule pieces playing at, or after, ms.
type playoutSource func(ctx context.Context, ms int64) ([]db.PlayoutPiece, error)

// startVirtual starts (or joins) a virtual channel's stream.
func (m *liveManager) startVirtual(ctx context.Context, channel, name string, spec Spec, src playoutSource) (*LiveSession, error) {
	spec.CopyVideo, spec.CopyAudio = false, false
	key := "vc:" + channel
	m.mu.Lock()
	for _, s := range m.sessions {
		if s.key == key && s.Height == spec.Height && s.running() {
			m.mu.Unlock()
			s.touch()
			return s, nil
		}
	}
	m.mu.Unlock()

	id := randomID()
	dir := filepath.Join(m.root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithCancel(context.Background())
	s := &LiveSession{ID: id, key: key, Channel: channel, Name: name, Height: spec.Height, HW: m.enc.HW, Started: time.Now(), Virtual: true,
		dir: dir, cancel: cancel, exited: make(chan struct{}), stderr: &syncBuffer{}, lastAccess: time.Now()}
	pl := &mergedPlaylist{dir: dir}
	failed := make(chan error, 1)
	go func() {
		defer close(s.exited)
		m.playVirtual(runCtx, s, pl, spec, src, failed)
	}()

	deadline := time.NewTimer(liveStartWait)
	defer deadline.Stop()
	playlist := filepath.Join(dir, "index.m3u8")
	for {
		if _, err := os.Stat(playlist); err == nil {
			break
		}
		select {
		case err := <-failed:
			cancel()
			<-s.exited
			_ = os.RemoveAll(dir)
			return nil, err
		case <-deadline.C:
			cancel()
			<-s.exited
			_ = os.RemoveAll(dir)
			return nil, errors.New("the channel didn't start in time: " + lastLine(s.stderr.String()))
		case <-ctx.Done():
			cancel()
			<-s.exited
			_ = os.RemoveAll(dir)
			return nil, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()
	slog.Info("virtual channel playing", "channel", channel, "name", name, "height", spec.Height, "hw", s.HW, "session", s.ID)
	return s, nil
}

// playVirtual runs until ctx ends. The first error before any output goes to
// failed; later ones are logged and the piece skipped.
func (m *liveManager) playVirtual(ctx context.Context, s *LiveSession, pl *mergedPlaylist, spec Spec, src playoutSource, failed chan<- error) {
	pos := time.Now().UnixMilli() - virtualLeadMs
	var streamMs int64 // stream time so far: each run's timestamps carry on from the last
	for run := 0; ctx.Err() == nil; run++ {
		now := time.Now().UnixMilli()
		if pos < now-virtualResyncMs {
			pos = now - virtualLeadMs // fell far behind (a slow encode): jump to the schedule
		}
		pieces, err := src(ctx, pos)
		if err == nil && len(pieces) == 0 {
			err = errors.New("nothing is scheduled on this channel; check that its filters match something in your library")
		}
		if err != nil {
			if pl.count() == 0 {
				failed <- err
				return
			}
			slog.Warn("virtual channel", "channel", s.Channel, "err", err)
			sleepCtx(ctx, 2*time.Second)
			continue
		}
		p := pieces[0]
		if p.StartMs > pos {
			pos = p.StartMs // a gap in the schedule
		}
		end := p.EndMs
		// Catch up fast at the start (the player's buffer), and later only if
		// the small delays between runs have added up.
		realtime := run > 0 && now-pos <= virtualMaxLagMs
		if !realtime {
			end = min(end, now) // catch up only as far as the schedule is
		}
		if end-pos < 300 {
			pos = end
			continue
		}
		args := m.virtualArgs(p, p.InMs+(pos-p.StartMs), end-pos, streamMs, realtime, spec, s.dir, run)
		if err := m.runPiece(ctx, args, s, pl, run); err != nil && ctx.Err() == nil {
			if pl.count() == 0 {
				failed <- fmt.Errorf("couldn't play %s: %w", filepath.Base(p.Path), err)
				return
			}
			slog.Warn("virtual channel: skipping a piece that won't play", "channel", s.Channel, "file", p.Path, "err", err)
			sleepCtx(ctx, time.Second)
		}
		streamMs += end - pos
		pos = end
	}
}

// virtualArgs encodes durMs of a file from inMs to HLS segments for run,
// with timestamps starting offsetMs into the stream.
func (m *liveManager) virtualArgs(p db.PlayoutPiece, inMs, durMs, offsetMs int64, realtime bool, spec Spec, dir string, run int) []string {
	secs := func(ms int64) string { return strconv.FormatFloat(float64(ms)/1000, 'f', 3, 64) }
	vIn, vOut := m.enc.Video(transcode.VideoOpts{MaxHeight: spec.Height, BitrateK: transcode.BitrateFor(spec.Height),
		Deinterlace: true, Live: true})
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error"}
	args = append(args, vIn...)
	if realtime {
		args = append(args, "-re")
	}
	args = append(args, "-ss", secs(inMs), "-t", secs(durMs), "-i", p.Path)
	audio := "0:a:0"
	if !p.HasAudio {
		args = append(args, "-f", "lavfi", "-t", secs(durMs), "-i", "anullsrc=r=48000:cl=stereo")
		audio = "1:a:0"
	}
	args = append(args, "-map", "0:v:0", "-map", audio, "-sn", "-dn")
	args = append(args, vOut...)
	args = append(args, "-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", liveSegDur))
	args = append(args, transcode.AudioArgs()...)
	return append(args, "-ar", "48000", "-output_ts_offset", secs(offsetMs), "-f", "hls", "-hls_time", strconv.Itoa(liveSegDur), "-hls_list_size", "0",
		"-hls_playlist_type", "event", "-hls_flags", "temp_file+independent_segments",
		"-hls_segment_filename", filepath.Join(dir, fmt.Sprintf("seg%d_%%d.ts", run)),
		filepath.Join(dir, fmt.Sprintf("run%d.m3u8", run)))
}

// runPiece runs one ffmpeg, merging its segments into the playlist as they appear.
func (m *liveManager) runPiece(ctx context.Context, args []string, s *LiveSession, pl *mergedPlaylist, run int) error {
	cmd := exec.CommandContext(ctx, m.enc.FFmpeg, args...)
	cmd.Stderr = s.stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	runList := filepath.Join(s.dir, fmt.Sprintf("run%d.m3u8", run))
	t := time.NewTicker(300 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case err := <-done:
			pl.follow(runList, run)
			_ = os.Remove(runList)
			if err != nil {
				return errors.New(lastLine(s.stderr.String()))
			}
			return nil
		case <-t.C:
			pl.follow(runList, run)
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// --- the merged playlist -------------------------------------------------------

type mergedSegment struct {
	seq  int64
	dur  float64
	uri  string
	disc bool // first segment of a later run
	run  int
}

// mergedPlaylist is the session's index.m3u8: every run's segments in order,
// a sliding window of the last virtualWindow.
type mergedPlaylist struct {
	dir string

	mu      sync.Mutex
	segs    []mergedSegment
	nextSeq int64
	discSeq int64       // discontinuities that have slid out of the window
	seen    map[int]int // segments merged so far, per run
}

func (p *mergedPlaylist) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return int(p.nextSeq)
}

// follow appends the segments of run's playlist not merged yet.
func (p *mergedPlaylist) follow(runList string, run int) {
	f, err := os.Open(runList)
	if err != nil {
		return
	}
	var durs []float64
	var uris []string
	sc := bufio.NewScanner(f)
	var dur float64
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "#EXTINF:"):
			v := strings.TrimSuffix(strings.TrimPrefix(line, "#EXTINF:"), ",")
			if i := strings.Index(v, ","); i >= 0 {
				v = v[:i]
			}
			dur, _ = strconv.ParseFloat(v, 64)
		case line != "" && !strings.HasPrefix(line, "#"):
			durs = append(durs, dur)
			uris = append(uris, filepath.Base(line))
		}
	}
	f.Close()

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.seen == nil {
		p.seen = map[int]int{}
	}
	added := false
	for i := p.seen[run]; i < len(uris); i++ {
		p.segs = append(p.segs, mergedSegment{seq: p.nextSeq, dur: durs[i], uri: uris[i], disc: i == 0 && p.nextSeq > 0, run: run})
		p.nextSeq++
		added = true
	}
	p.seen[run] = len(uris)
	if !added {
		return
	}
	for len(p.segs) > virtualWindow {
		if p.segs[0].disc {
			p.discSeq++
		}
		_ = os.Remove(filepath.Join(p.dir, p.segs[0].uri))
		p.segs = p.segs[1:]
	}
	p.write()
}

// write saves index.m3u8 atomically. Called with mu held.
func (p *mergedPlaylist) write() {
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:6\n")
	target := float64(liveSegDur + 1)
	for _, s := range p.segs {
		target = math.Max(target, math.Ceil(s.dur))
	}
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:%d\n#EXT-X-DISCONTINUITY-SEQUENCE:%d\n#EXT-X-INDEPENDENT-SEGMENTS\n",
		int(target), p.segs[0].seq, p.discSeq)
	for _, s := range p.segs {
		if s.disc {
			b.WriteString("#EXT-X-DISCONTINUITY\n")
		}
		fmt.Fprintf(&b, "#EXTINF:%.3f,\n%s\n", s.dur, s.uri)
	}
	tmp := filepath.Join(p.dir, "index.m3u8.tmp")
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err == nil {
		_ = os.Rename(tmp, filepath.Join(p.dir, "index.m3u8"))
	}
}

// watchVirtual starts a virtual channel's stream, building its schedule first
// if there isn't one yet (a channel that was just added).
func (s *Service) watchVirtual(ctx context.Context, id int64, ch db.Channel, o WatchOpts) (*LiveSession, error) {
	if end, err := s.db.PlayoutEnd(ctx, id); err != nil {
		return nil, err
	} else if end < time.Now().Add(5*time.Minute).UnixMilli() {
		vc, err := s.db.VirtualChannel(ctx, id)
		if err != nil {
			return nil, err
		}
		if err := s.extendVirtual(ctx, vc, time.Now()); err != nil {
			return nil, err
		}
	}
	spec := o.spec("", "")
	src := func(ctx context.Context, ms int64) ([]db.PlayoutPiece, error) {
		return s.db.PlayoutFrom(ctx, id, ms, 4)
	}
	return s.live.startVirtual(ctx, ch.Number, ch.Name, spec, src)
}
