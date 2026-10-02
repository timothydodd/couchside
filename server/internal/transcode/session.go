package transcode

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/timothydodd/couchside/internal/probe"
	"github.com/timothydodd/couchside/internal/usererr"
)

// SegDur is the HLS segment length in seconds. Keyframes are forced on this
// grid, so the playlist can list every segment up front and any segment can
// be (re)generated independently: that's what makes seeking instant.
const SegDur = 4

const (
	aheadLimit  = 30               // segments (2 min) ffmpeg may run ahead before it's paused
	resumeAt    = 15               // resume once the player is within this many segments
	restartGap  = 5                // a request further ahead than this restarts ffmpeg at that segment
	idleKill    = 90 * time.Second // stop ffmpeg when nobody has asked for a segment
	idleExpire  = 3 * time.Hour    // forget the session entirely
	segmentWait = 90 * time.Second
	evictAfter  = 20 * time.Second // at the limit, sessions idle this long give up their slot
	stderrKeep  = 4096
	seekPreroll = 10.0 // seconds decoded before a restart point and trimmed (see start)
)

var (
	ErrNoSession  = errors.New("transcode session not found")
	ErrBusy       = usererr.New("too many streams are transcoding right now")
	ErrBadSegment = errors.New("segment out of range")
	ErrPastEnd    = errors.New("past the end of the stream")
)

// Request describes what the player wants.
type Request struct {
	FileID         int64
	Title          string
	Path           string
	Duration       float64
	Height         int  // 0 = best available (source height, capped at 1080p)
	BitrateK       int  // video bitrate in kbit/s; 0 = the default for the height
	AllowCopyVideo bool // client can play the source's H.264 as is
	AllowCopyAudio bool // client can play the source's AAC/MP3 as is
	AudioIndex     int  // which audio track (0:a:N)
	BurnSubtitle   int  // image subtitle track to burn into the video, -1 for none
}

// Session is one live stream of one file at one quality.
type Session struct {
	ID        string    `json:"id"`
	FileID    int64     `json:"fileId"`
	Title     string    `json:"title"`
	Mode      string    `json:"mode"` // remux (video copied) | transcode
	Height    int       `json:"height"`
	BitrateK  int       `json:"bitrateK"`
	CopyVideo bool      `json:"copyVideo"`
	CopyAudio bool      `json:"copyAudio"`
	Audio     int       `json:"audio"`
	BurnSub   int       `json:"burnSubtitle"`
	HDR       bool      `json:"hdr"`
	HW        string    `json:"hw"`
	HWDecode  bool      `json:"hwDecode"` // VAAPI: decoding and scaling on the GPU too
	Created   time.Time `json:"created"`

	src       string
	dir       string
	duration  float64
	srcHeight int
	enc       Encoder

	// restartMu serialises stopping and starting ffmpeg, which releases mu
	// while it waits for a killed run to exit. Take it before mu, never after.
	restartMu sync.Mutex

	mu         sync.Mutex
	closed     bool // removed from the manager: never start ffmpeg again
	cmd        *exec.Cmd
	exited     chan struct{}
	exitErr    error
	stderr     *limitedWriter
	startSeg   int
	hi         int // highest finished segment of the current run
	lastReq    int
	lastAccess time.Time
	paused     bool
}

// SessionInfo is the Activity page view of a session.
type SessionInfo struct {
	*Session
	PositionSec float64 `json:"positionSec"`
	Running     bool    `json:"running"`
	Paused      bool    `json:"paused"`
	AheadSec    float64 `json:"aheadSec"`
}

type Manager struct {
	enc     Encoder
	ffprobe string
	root    string
	max     int

	mu       sync.Mutex
	sessions map[string]*Session
}

// NewManager wipes leftovers from a previous run: sessions live in memory only.
func NewManager(enc Encoder, ffprobe, root string, max int) (*Manager, error) {
	_ = os.RemoveAll(root)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &Manager{enc: enc, ffprobe: ffprobe, root: root, max: max, sessions: map[string]*Session{}}, nil
}

func (m *Manager) Encoder() Encoder { return m.enc }
func (m *Manager) Max() int         { return m.max }

// Create decides copy-vs-encode for each stream and registers a session.
// ffmpeg itself starts lazily, on the first segment request. Probing (up to
// a minute on a slow share) happens before the manager lock is taken.
func (m *Manager) Create(ctx context.Context, r Request) (*Session, error) {
	if r.Duration <= 0 {
		return nil, usererr.New("file duration is unknown, so it can't be streamed; try rescanning")
	}
	info, err := probe.Probe(ctx, m.ffprobe, r.Path)
	if err != nil {
		return nil, err
	}

	srcH := 0
	if info.Height != nil {
		srcH = *info.Height
	}
	// The chosen audio track decides whether audio can be copied.
	audioCodec, audioCh := info.AudioCodec, info.AudioChannels
	if r.AudioIndex > 0 {
		if st, err := probe.ListStreams(ctx, m.ffprobe, r.Path); err == nil && r.AudioIndex < len(st.Audio) {
			audioCodec, audioCh = st.Audio[r.AudioIndex].Codec, st.Audio[r.AudioIndex].Channels
		} else {
			r.AudioIndex = 0
		}
	}
	burn := r.BurnSubtitle >= 0
	copyVideo := r.AllowCopyVideo && !burn && info.VideoCodec == "h264" && info.EightBit420() && !info.HDR() &&
		(r.Height == 0 || (srcH > 0 && srcH <= r.Height))
	copyAudio := r.AllowCopyAudio && (audioCodec == "aac" || audioCodec == "mp3") && audioCh <= 6

	s := &Session{
		ID: newID(), FileID: r.FileID, Title: r.Title, CopyVideo: copyVideo, CopyAudio: copyAudio,
		Audio: r.AudioIndex, BurnSub: r.BurnSubtitle,
		HDR: info.HDR() && !copyVideo, HW: m.enc.HW, Created: time.Now(),
		src: r.Path, duration: r.Duration, srcHeight: srcH, enc: m.enc,
		lastAccess: time.Now(), hi: -1,
	}
	if copyVideo {
		s.Mode, s.Height, s.HW = "remux", srcH, ""
	} else {
		s.Mode = "transcode"
		s.Height = OutputHeight(r.Height, srcH)
		s.BitrateK = BitrateFor(s.Height)
		if r.BitrateK > 0 {
			s.BitrateK = min(max(r.BitrateK, 300), 40000)
		}
		// Burned-in subtitles are overlaid on the CPU, so those frames can't stay on the GPU.
		s.HWDecode = m.enc.HWDecode && !burn && HWDecodable[info.VideoCodec]
	}
	s.dir = filepath.Join(m.root, s.ID)
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return nil, err
	}

	// Check the limit and take the slot in one hold of the lock, so
	// concurrent requests can't all pass the check.
	m.mu.Lock()
	victim, err := m.admitLocked(nil)
	if err == nil {
		m.sessions[s.ID] = s
	}
	m.mu.Unlock()
	if err != nil {
		_ = os.RemoveAll(s.dir)
		return nil, err
	}
	if victim != nil {
		m.shutdown(victim)
	}
	slog.Info("transcode session", "id", s.ID, "file", r.FileID, "mode", s.Mode, "height", s.Height,
		"copyAudio", copyAudio, "hdr", s.HDR, "hw", m.enc.HW, "gpuDecode", s.HWDecode, "bitrateK", s.BitrateK, "src", info.VideoCodec+"/"+info.PixFmt+"/"+info.AudioCodec)
	return s, nil
}

// admitLocked enforces the live-session limit for one more stream: a new
// session (self nil) or self about to start ffmpeg again. A session is live
// while its ffmpeg runs or it was asked for in the last idleKill. Rather than
// refuse a stream because of abandoned ones (closed tab, reload, switched
// quality in another window), it evicts the least recently used session that
// has gone quiet, removing it from the map; the caller must shut it down.
// Caller holds m.mu.
func (m *Manager) admitLocked(self *Session) (*Session, error) {
	var live []*Session
	for _, s := range m.sessions {
		if s == self {
			continue
		}
		s.mu.Lock()
		if s.running() || time.Since(s.lastAccess) < idleKill {
			live = append(live, s)
		}
		s.mu.Unlock()
	}
	if len(live) < m.max {
		return nil, nil
	}
	var victim *Session
	var oldest time.Time
	for _, s := range live {
		s.mu.Lock()
		la := s.lastAccess
		s.mu.Unlock()
		if time.Since(la) >= evictAfter && (victim == nil || la.Before(oldest)) {
			victim, oldest = s, la
		}
	}
	if victim == nil {
		return nil, ErrBusy
	}
	slog.Info("evicting idle transcode session to make room", "id", victim.ID, "title", victim.Title)
	delete(m.sessions, victim.ID)
	return victim, nil
}

func (s *Session) segCount() int { return int(math.Ceil(s.duration / SegDur)) }

func (s *Session) segPath(n int) string { return filepath.Join(s.dir, "seg"+strconv.Itoa(n)+".ts") }

// Playlist lists every segment of the file up front (a VOD playlist), so the
// player knows the full duration and can seek anywhere immediately.
func (s *Session) Playlist() []byte {
	var b strings.Builder
	target := SegDur
	if s.CopyVideo {
		// Copied video can only be cut on its own keyframes, so segments run long.
		target = 12
	}
	fmt.Fprintf(&b, "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:0\n", target)
	n := s.segCount()
	for i := 0; i < n; i++ {
		d := float64(SegDur)
		if i == n-1 {
			d = s.duration - float64(SegDur*(n-1))
		}
		fmt.Fprintf(&b, "#EXTINF:%.3f,\nseg%d.ts\n", d, i)
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	return []byte(b.String())
}

func (m *Manager) Get(id string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[id]
}

// Close stops ffmpeg and deletes the session's segments.
func (m *Manager) Close(id string) {
	m.mu.Lock()
	s := m.sessions[id]
	delete(m.sessions, id)
	m.mu.Unlock()
	if s != nil {
		m.shutdown(s)
	}
}

// shutdown stops a session already removed from the map, for good.
func (m *Manager) shutdown(s *Session) {
	s.restartMu.Lock()
	s.mu.Lock()
	s.closed = true
	s.stop()
	s.mu.Unlock()
	s.restartMu.Unlock()
	_ = os.RemoveAll(s.dir)
}

// Segment returns the path of segment n, generating it if needed. It blocks
// until the segment is written, restarting ffmpeg at n when the player has
// jumped outside what the current run will produce soon.
func (m *Manager) Segment(ctx context.Context, id string, n int) (string, error) {
	s := m.Get(id)
	if s == nil {
		return "", ErrNoSession
	}
	if n < 0 || n >= s.segCount() {
		return "", ErrBadSegment
	}
	path := s.segPath(n)
	if exists(path) {
		s.mu.Lock()
		s.lastAccess, s.lastReq = time.Now(), n
		s.mu.Unlock()
		return path, nil
	}
	if err := m.ensureRun(s, n); err != nil {
		return "", err
	}

	deadline := time.NewTimer(segmentWait)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		if exists(path) {
			return path, nil
		}
		s.mu.Lock()
		exited := s.cmd == nil || !s.running()
		errMsg := ""
		cleanEOF := false
		if exited && s.stderr != nil {
			errMsg = tail(s.stderr.String(), 400)
			cleanEOF = s.exitErr == nil && s.cmd != nil
		}
		s.mu.Unlock()
		if exited && !exists(path) {
			if cleanEOF {
				// ffmpeg reached the end of the file without making this
				// segment: copied video is cut on its own keyframes, so a
				// remux can have fewer segments than the playlist lists.
				return "", ErrPastEnd
			}
			if s.gpuFallback(n, errMsg) {
				continue
			}
			if errMsg == "" {
				errMsg = "ffmpeg stopped before producing this segment"
			}
			return "", fmt.Errorf("transcode failed: %s", errMsg)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-deadline.C:
			return "", usererr.New("timed out waiting for the transcoder; the server may be too slow for this file")
		case <-tick.C:
		}
	}
}

// ensureRun makes sure an ffmpeg run will produce segment n: it resumes a
// paused run, or (re)starts ffmpeg at n when there's none or the player has
// jumped outside what the current run will produce soon. Restarts of one
// session are serialised, and each re-checks after waiting its turn, since
// the run another request just started may already cover n.
func (m *Manager) ensureRun(s *Session, n int) error {
	s.restartMu.Lock()
	defer s.restartMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	for admitted := false; ; {
		if s.closed {
			return ErrNoSession
		}
		s.lastAccess, s.lastReq = time.Now(), n
		if exists(s.segPath(n)) {
			return nil
		}
		s.refreshHi()
		if s.running() && n >= s.startSeg && n <= s.hi+restartGap {
			if s.paused {
				resume(s.cmd.Process)
				s.paused = false
			}
			return nil
		}
		if s.running() || admitted {
			return s.start(n)
		}
		// No ffmpeg running: starting one is a new stream as far as the limit
		// goes (a player back from a long pause, or one reviving a session
		// whose ffmpeg was reaped).
		s.mu.Unlock()
		m.mu.Lock()
		victim, err := m.admitLocked(s)
		m.mu.Unlock()
		if victim != nil {
			// Not here: we hold our own restartMu, and shutdown takes the victim's.
			go m.shutdown(victim)
		}
		s.mu.Lock()
		if err != nil {
			return err
		}
		admitted = true
	}
}

// gpuFallback handles a failed GPU-pipeline run: the GPU may not decode this
// codec or profile, or the driver can't tone map it. The session switches to
// CPU decoding (GPU encoding) for good and restarts at segment n. It returns
// false when there's nothing to fall back to.
func (s *Session) gpuFallback(n int, errMsg string) bool {
	s.restartMu.Lock()
	defer s.restartMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.HWDecode || s.closed {
		return false
	}
	slog.Warn("GPU decoding failed for this file; decoding on the CPU instead", "session", s.ID, "file", s.FileID, "ffmpeg", errMsg)
	s.HWDecode = false
	return s.start(n) == nil
}

// start (re)launches ffmpeg at segment n. Caller holds s.restartMu and s.mu.
func (s *Session) start(n int) error {
	s.stop()
	if s.closed {
		return ErrNoSession
	}
	startSec := float64(n * SegDur)
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error"}
	var vIn, vCodec []string
	var chain string
	if !s.CopyVideo {
		vIn, chain, vCodec = s.enc.VideoParts(VideoOpts{MaxHeight: s.Height, SrcHeight: s.srcHeight, BitrateK: s.BitrateK, HDR: s.HDR,
			HWDecode: s.HWDecode})
	}
	args = append(args, vIn...)
	// Seeking lands near startSec, and video starts at the next keyframe after
	// it; MPEG-TS has no index, so that can be seconds late, or (VAAPI on
	// MPEG-2) after the whole first segment, which then holds audio only and
	// hls.js stalls on it. So when encoding, seek a little early and trim
	// back to startSec in the filters: every run's first frame is then on
	// the grid. Copied streams can't be trimmed, so they seek as before.
	preroll := 0.0
	if startSec > 0 && !s.CopyVideo && !s.CopyAudio {
		preroll = min(startSec, seekPreroll)
	}
	if startSec > 0 {
		args = append(args, "-ss", strconv.FormatFloat(startSec-preroll, 'f', 3, 64))
	}
	if preroll > 0 {
		trim := "trim=start=" + strconv.FormatFloat(startSec, 'f', 3, 64)
		if chain == "" {
			chain = trim
		} else {
			chain = trim + "," + chain
		}
	}
	// copyts + start_at_zero keeps timestamps absolute (position in the file),
	// so segments from different ffmpeg runs line up on one timeline.
	args = append(args, "-copyts", "-start_at_zero", "-i", s.src)
	if s.BurnSub >= 0 {
		// Picture subtitles (PGS) can depend on data sent a few seconds
		// before the seek point, so read the subtitle track from a second,
		// earlier-seeked input; copyts keeps both on the same timeline.
		args = append(args, "-ss", strconv.FormatFloat(max(0, startSec-10), 'f', 3, 64),
			"-copyts", "-start_at_zero", "-i", s.src)
	}
	switch {
	case s.BurnSub >= 0:
		// Picture-based subtitles (Blu-ray PGS, DVD) can't be sent as text:
		// overlay them onto the video before scaling and encoding.
		// Subtitle bitmaps are often a different size than the video (1080p
		// PGS on a 4K film), so scale them to the video before overlaying.
		args = append(args, "-filter_complex",
			fmt.Sprintf("[1:s:%d][0:v:0]scale2ref=w=main_w:h=main_h[sub][vid];[vid][sub]overlay=eof_action=pass[sv];[sv]%s[vout]", s.BurnSub, chain),
			"-map", "[vout]")
	default:
		args = append(args, "-map", "0:v:0")
	}
	args = append(args, "-map", fmt.Sprintf("0:a:%d?", s.Audio), "-sn", "-dn", "-map_metadata", "-1", "-map_chapters", "-1")
	if s.CopyVideo {
		args = append(args, "-c:v", "copy")
	} else {
		if s.BurnSub < 0 {
			args = append(args, "-vf", chain)
		}
		args = append(args, vCodec...)
		// t is relative to this run's first frame, and runs always start on the
		// grid, so "every SegDur seconds" lands on the playlist's boundaries.
		args = append(args, "-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", SegDur))
	}
	if s.CopyAudio {
		args = append(args, "-c:a", "copy")
	} else {
		if preroll > 0 {
			args = append(args, "-af", "atrim=start="+strconv.FormatFloat(startSec, 'f', 3, 64))
		}
		args = append(args, AudioArgs()...)
	}
	args = append(args, "-max_muxing_queue_size", "4096", "-avoid_negative_ts", "disabled",
		"-f", "hls", "-hls_time", strconv.Itoa(SegDur), "-hls_segment_type", "mpegts",
		"-hls_flags", "temp_file", "-hls_list_size", "0", "-start_number", strconv.Itoa(n),
		"-hls_segment_filename", filepath.Join(s.dir, "seg%d.ts"), filepath.Join(s.dir, "ffmpeg.m3u8"))

	cmd := exec.Command(s.enc.FFmpeg, args...)
	stderr := &limitedWriter{max: stderrKeep}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	if startHook != nil {
		startHook(cmd.Process.Pid)
	}
	exited := make(chan struct{})
	s.cmd, s.exited, s.stderr, s.exitErr = cmd, exited, stderr, nil
	s.startSeg, s.hi, s.paused = n, n-1, false
	go func() {
		err := cmd.Wait()
		s.mu.Lock()
		if s.cmd == cmd {
			s.exitErr = err
		}
		s.mu.Unlock()
		close(exited)
	}()
	return nil
}

// stop kills the current ffmpeg, if any. Caller holds s.restartMu and s.mu;
// mu is released while the killed process exits.
func (s *Session) stop() {
	cmd := s.cmd
	if cmd == nil || cmd.Process == nil {
		return
	}
	if s.running() {
		if s.paused {
			resume(cmd.Process)
		}
		_ = cmd.Process.Kill()
		exited := s.exited
		s.mu.Unlock()
		<-exited
		s.mu.Lock()
	}
	if s.cmd != cmd {
		return // someone else replaced it meanwhile; that run is theirs
	}
	s.cmd, s.paused = nil, false
	// Drop half-written temp segments from the killed run.
	tmps, _ := filepath.Glob(filepath.Join(s.dir, "*.tmp"))
	for _, t := range tmps {
		_ = os.Remove(t)
	}
}

// startHook, when set (by tests), is told each ffmpeg's pid.
var startHook func(pid int)

func (s *Session) running() bool {
	if s.cmd == nil || s.exited == nil {
		return false
	}
	select {
	case <-s.exited:
		return false
	default:
		return true
	}
}

// refreshHi advances hi over segments the current run has finished.
func (s *Session) refreshHi() {
	if s.hi < s.startSeg-1 {
		s.hi = s.startSeg - 1
	}
	for exists(s.segPath(s.hi + 1)) {
		s.hi++
	}
}

// Run throttles runaway encodes and reaps idle sessions until ctx ends.
func (m *Manager) Run(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			m.mu.Lock()
			ids := make([]string, 0, len(m.sessions))
			for id := range m.sessions {
				ids = append(ids, id)
			}
			m.mu.Unlock()
			for _, id := range ids {
				m.Close(id)
			}
			return
		case <-t.C:
		}
		m.mu.Lock()
		all := make([]*Session, 0, len(m.sessions))
		for _, s := range m.sessions {
			all = append(all, s)
		}
		m.mu.Unlock()
		for _, s := range all {
			s.restartMu.Lock()
			s.mu.Lock()
			idle := time.Since(s.lastAccess)
			expired := idle > idleExpire
			if !expired && s.running() {
				s.refreshHi()
				switch {
				case idle > idleKill:
					s.stop()
				case !s.paused && s.hi-s.lastReq > aheadLimit:
					suspend(s.cmd.Process)
					s.paused = true
				case s.paused && s.hi-s.lastReq < resumeAt:
					resume(s.cmd.Process)
					s.paused = false
				}
			}
			s.mu.Unlock()
			s.restartMu.Unlock()
			if expired {
				m.Close(s.ID)
			}
		}
	}
}

// Sessions lists live sessions, newest first.
func (m *Manager) Sessions() []SessionInfo {
	m.mu.Lock()
	all := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		all = append(all, s)
	}
	m.mu.Unlock()
	out := make([]SessionInfo, 0, len(all))
	for _, s := range all {
		s.mu.Lock()
		if time.Since(s.lastAccess) < idleKill {
			s.refreshHi()
			out = append(out, SessionInfo{Session: s, PositionSec: float64(s.lastReq * SegDur), Running: s.running(),
				Paused: s.paused, AheadSec: math.Max(0, float64((s.hi-s.lastReq)*SegDur))})
		}
		s.mu.Unlock()
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// limitedWriter keeps only the tail of ffmpeg's stderr. exec writes to it
// from its own goroutine, hence the lock.
type limitedWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
	max int
}

func (w *limitedWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	if w.buf.Len() > w.max {
		rest := w.buf.Bytes()[w.buf.Len()-w.max:]
		nb := append([]byte(nil), rest...)
		w.buf.Reset()
		w.buf.Write(nb)
	}
	return len(p), nil
}
