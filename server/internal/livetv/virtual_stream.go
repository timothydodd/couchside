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
	"github.com/timothydodd/couchside/internal/usererr"
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

	virtualRetryMs    = 30_000 // a piece that failed is tried again this often while it's scheduled
	virtualShortMs    = 1500   // a run this much shorter than asked means the file ended early
	virtualSkipWaitMs = 12_000 // at tune-in, wait this long for the next piece when the current one won't play
)

// playoutSource returns schedule pieces playing at, or after, ms.
type playoutSource func(ctx context.Context, ms int64) ([]db.PlayoutPiece, error)

// startVirtual starts (or joins) a virtual channel's stream.
func (m *liveManager) startVirtual(ctx context.Context, channel, name string, spec Spec, src playoutSource) (*LiveSession, error) {
	spec.CopyVideo, spec.CopyAudio = false, false
	spec.Height = snapHeight(spec.Height)
	key := "vc:" + channel
	k := fmt.Sprintf("%s|%d", key, spec.Height)
	m.mu.Lock()
	joined, f, err := m.claimLocked(ctx, k, func(s *LiveSession) bool { return s.key == key && s.Height == spec.Height }, true)
	m.mu.Unlock()
	if err != nil || joined != nil {
		return joined, err
	}
	s, err := m.launchVirtual(ctx, key, channel, name, spec, src)
	m.finish(k, f, s, err)
	return s, err
}

// launchVirtual starts a virtual channel's stream and waits for its first playlist.
func (m *liveManager) launchVirtual(ctx context.Context, key, channel, name string, spec Spec, src playoutSource) (*LiveSession, error) {
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
			return nil, usererr.New("the channel didn't start in time: " + lastLine(s.stderr.String()))
		case <-ctx.Done():
			cancel()
			<-s.exited
			_ = os.RemoveAll(dir)
			return nil, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	slog.Info("virtual channel playing", "channel", channel, "name", name, "height", spec.Height, "hw", s.HW, "session", s.ID)
	return s, nil
}

// playVirtual runs until ctx ends. The first error before any output goes to
// failed; later ones are logged and the piece skipped.
func (m *liveManager) playVirtual(ctx context.Context, s *LiveSession, pl *mergedPlaylist, spec Spec, src playoutSource, failed chan<- error) {
	info := map[string]pieceVideo{} // probed once per file
	gpuOff := false                 // a GPU run failed: decode on the CPU from here on
	play := func(ctx context.Context, p db.PlayoutPiece, inMs, durMs, streamMs int64, realtime bool, run int) (int64, error) {
		v, ok := info[p.Path]
		if !ok && m.pieceInfo != nil {
			v = m.pieceInfo(ctx, p.Path)
			info[p.Path] = v
		}
		hw := !gpuOff && m.enc.HWDecode && transcode.HWDecodable[v.Codec]
		err := m.runPiece(ctx, m.virtualArgs(p, v, hw, inMs, durMs, streamMs, realtime, spec, s.dir, run), s, pl, run)
		if err != nil && hw && ctx.Err() == nil && pl.runMs(run) == 0 {
			slog.Warn("virtual channel: GPU decoding failed, decoding on the CPU instead", "channel", s.Channel, "file", p.Path, "err", err)
			gpuOff = true
			err = m.runPiece(ctx, m.virtualArgs(p, v, false, inMs, durMs, streamMs, realtime, spec, s.dir, run), s, pl, run)
		}
		return pl.runMs(run), err
	}
	virtualLoop(ctx, s.Channel, src, play, func() int64 { return time.Now().UnixMilli() }, sleepCtx, failed)
}

// pieceRunner plays durMs of a piece from inMs and returns how much it
// actually produced, which is less when the file is shorter than the
// schedule believes or the run failed part way.
type pieceRunner func(ctx context.Context, p db.PlayoutPiece, inMs, durMs, streamMs int64, realtime bool, run int) (producedMs int64, err error)

// virtualLoop walks the schedule, keeping the stream at the schedule's
// position: pos never runs ahead of the clock. A piece that can't be played
// (its file moved or the share is gone) leaves a hole in the stream for as
// long as it's scheduled, and is tried again now and then in case the file
// is back, rather than starting the next program early: everyone on the
// channel shares this stream, and the guide says what's on.
func virtualLoop(ctx context.Context, channel string, src playoutSource, play pieceRunner, now func() int64, sleep func(context.Context, time.Duration), failed chan<- error) {
	pos := now() - virtualLeadMs
	var streamMs int64 // stream time so far: each run's timestamps carry on from the last
	started := false   // something has reached the playlist
	lastFailed := ""
	for run := 0; ctx.Err() == nil; run++ {
		t := now()
		if pos > t+1000 {
			sleep(ctx, time.Duration(pos-t)*time.Millisecond) // nothing to play yet
			continue
		}
		if pos < t-virtualResyncMs {
			pos = t - virtualLeadMs // fell far behind (a slow encode): jump to the schedule
		}
		pieces, err := src(ctx, pos)
		if err == nil && len(pieces) == 0 {
			err = usererr.New("nothing is scheduled on this channel; check that its filters match something in your library")
		}
		if err != nil {
			if !started {
				failed <- err
				return
			}
			slog.Warn("virtual channel", "channel", channel, "err", err)
			sleep(ctx, 2*time.Second)
			continue
		}
		p := pieces[0]
		if p.StartMs > pos {
			pos = p.StartMs // a gap in the schedule
			continue
		}
		end := p.EndMs
		// Catch up fast at the start (the player's buffer), and later only if
		// the small delays between runs have added up.
		lag := t - pos
		realtime := lag <= virtualMaxLagMs && (started || lag < 1000)
		if !realtime {
			end = min(end, t) // catch up only as far as the schedule is
		}
		if end-pos < 300 {
			pos = end
			continue
		}
		produced, err := play(ctx, p, p.InMs+(pos-p.StartMs), end-pos, streamMs, realtime, run)
		if ctx.Err() != nil {
			return
		}
		streamMs += produced
		if produced > 0 {
			started = true
		}
		switch {
		case err != nil && !started:
			// Tuning in while a file that won't play is on. Wait for the next
			// piece if that's soon; otherwise say when the channel is back.
			if p.EndMs-now() > virtualSkipWaitMs {
				failed <- usererr.Errorf("%s can't be played right now (the file is missing or unreadable); this channel carries on at %s",
					filepath.Base(p.Path), time.UnixMilli(p.EndMs).Format("3:04 PM"))
				return
			}
			slog.Warn("virtual channel: a piece won't play; waiting for the next", "channel", channel, "file", p.Path, "err", err)
			pos = p.EndMs
		case err != nil:
			if p.Path != lastFailed {
				slog.Warn("virtual channel: a piece won't play; the stream pauses until it does or the next is due", "channel", channel, "file", p.Path, "err", err)
				lastFailed = p.Path
			}
			pos = min(p.EndMs, max(pos+produced, now()+virtualRetryMs))
		case produced+virtualShortMs < end-pos:
			// The file ended before the schedule expected (replaced by a
			// shorter one since the schedule was built).
			slog.Warn("virtual channel: a file is shorter than scheduled", "channel", channel, "file", p.Path, "missing_ms", end-pos-produced)
			pos = p.EndMs
		default:
			pos = end
		}
	}
}

// pieceVideo is what converting a library file needs to know about it.
type pieceVideo struct {
	HDR   bool   // PQ or HLG: tone map to SDR
	Codec string // ffprobe's name, for transcode.HWDecodable
}

// virtualArgs encodes durMs of a file from inMs to HLS segments for run,
// with timestamps starting offsetMs into the stream. Every piece comes out
// at the session's height, smaller sources included: the pieces are joined
// into one stream, and players cope badly with the picture size changing at
// each join.
func (m *liveManager) virtualArgs(p db.PlayoutPiece, v pieceVideo, hwDecode bool, inMs, durMs, offsetMs int64, realtime bool, spec Spec, dir string, run int) []string {
	secs := func(ms int64) string { return strconv.FormatFloat(float64(ms)/1000, 'f', 3, 64) }
	vIn, vOut := m.enc.Video(transcode.VideoOpts{MaxHeight: spec.Height, BitrateK: transcode.BitrateFor(spec.Height),
		Deinterlace: true, Live: true, HDR: v.HDR, HWDecode: hwDecode})
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
	args = append(args, transcode.ForceKeyFrames(liveSegDur)...)
	args = append(args, transcode.AudioArgs()...)
	args = append(args, "-ar", "48000", "-output_ts_offset", secs(offsetMs))
	return append(args, transcode.HLSOutput{SegDur: liveSegDur, Start: -1, Event: true, Independent: true,
		Segments: filepath.Join(dir, fmt.Sprintf("seg%d_%%d.ts", run)),
		Playlist: filepath.Join(dir, fmt.Sprintf("run%d.m3u8", run))}.Args()...)
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
	discSeq int64         // discontinuities that have slid out of the window
	seen    map[int]int   // segments merged so far, per run
	ms      map[int]int64 // their total length, per run
}

// runMs is how much a run has put in the playlist so far.
func (p *mergedPlaylist) runMs(run int) int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ms[run]
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
		p.seen, p.ms = map[int]int{}, map[int]int64{}
	}
	added := false
	for i := p.seen[run]; i < len(uris); i++ {
		p.ms[run] += int64(math.Round(durs[i] * 1000))
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
	if vc, err := s.db.VirtualChannel(ctx, id); err != nil {
		return nil, err
	} else if cfg, err := ParseVirtualConfig(vc.Config); err == nil && cfg.Stream != nil {
		return s.live.startStream(ctx, ch.Number, ch.Name, cfg.Stream.URL, o.spec("", ""))
	}
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
