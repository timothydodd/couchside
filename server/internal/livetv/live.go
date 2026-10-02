package livetv

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/timothydodd/couchside/internal/transcode"
)

const (
	liveSegDur    = 2                // seconds; short segments keep the delay behind live low
	liveIdleKill  = 20 * time.Second // free the tuner soon after the last viewer leaves
	liveStartWait = 20 * time.Second
)

// LiveSession is one channel being tuned and transcoded to HLS. Viewers of
// the same channel at the same quality share it.
type LiveSession struct {
	ID      string    `json:"id"`
	Channel string    `json:"channel"`
	Name    string    `json:"name"`
	Height  int       `json:"height"`
	HW      string    `json:"hw"`
	Started time.Time `json:"started"`
	// Passthrough: the client can decode the broadcast's own video and/or
	// audio, so it's repackaged into HLS instead of re-encoded.
	CopyVideo bool `json:"copyVideo"`
	CopyAudio bool `json:"copyAudio"`
	HWDecode  bool `json:"hwDecode"` // VAAPI decodes and deinterlaces too
	Virtual   bool `json:"virtual"`  // one of Couchside's own channels

	key        string // what's being streamed: "ch:2.1", "rec:42" or "vc:900"
	dir        string
	cmd        *exec.Cmd          // tuner and recording streams: one ffmpeg
	cancel     context.CancelFunc // virtual channels: stops their run loop
	exited     chan struct{}
	stderr     *syncBuffer
	mu         sync.Mutex
	lastAccess time.Time
}

func (l *LiveSession) touch() {
	l.mu.Lock()
	l.lastAccess = time.Now()
	l.mu.Unlock()
}

func (l *LiveSession) idle() time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	return time.Since(l.lastAccess)
}

func (l *LiveSession) running() bool {
	select {
	case <-l.exited:
		return false
	default:
		return true
	}
}

type liveManager struct {
	enc        transcode.Encoder
	root       string
	maxEncodes int // live streams that encode video at once (COUCHSIDE_MAX_TRANSCODES)

	mu       sync.Mutex
	sessions map[string]*LiveSession
	starting map[string]*liveStart // streams being started, by startKey
}

// liveStart is a stream being started; others asking for the same one wait
// for it rather than take a second tuner.
type liveStart struct {
	done   chan struct{}
	s      *LiveSession
	err    error
	encode bool
}

// ErrBusyEncoding means the live encode limit is reached.
var ErrBusyEncoding = errors.New("too many streams are being converted right now; try again shortly, or a lower quality on another stream")

func newLiveManager(enc transcode.Encoder, root string, maxEncodes int) (*liveManager, error) {
	_ = os.RemoveAll(root)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	if maxEncodes <= 0 {
		maxEncodes = 2
	}
	return &liveManager{enc: enc, root: root, maxEncodes: maxEncodes, sessions: map[string]*LiveSession{},
		starting: map[string]*liveStart{}}, nil
}

// LiveHeights are the heights a live stream can be converted to (the player's
// presets). Anything else snaps to the nearest one below, so heights can't be
// used to start an encode each, or to upscale.
var LiveHeights = []int{360, 480, 720, 1080}

func snapHeight(h int) int {
	if h <= 0 {
		return 720
	}
	out := LiveHeights[0]
	for _, p := range LiveHeights {
		if p <= h {
			out = p
		}
	}
	return out
}

// claim joins a running or starting stream for k, or registers k as starting
// and returns a nil session with its liveStart, which the caller must finish.
// It refuses a new video encode past the limit. Caller holds m.mu.
func (m *liveManager) claimLocked(ctx context.Context, k string, match func(*LiveSession) bool, encode bool) (*LiveSession, *liveStart, error) {
	for {
		for _, s := range m.sessions {
			if match(s) && s.running() {
				s.touch()
				return s, nil, nil
			}
		}
		f := m.starting[k]
		if f == nil {
			break
		}
		m.mu.Unlock()
		select {
		case <-f.done:
		case <-ctx.Done():
			m.mu.Lock()
			return nil, nil, ctx.Err()
		}
		m.mu.Lock()
		// The first caller gave up (its viewer left): try again ourselves.
		if errors.Is(f.err, context.Canceled) {
			continue
		}
		if f.err != nil {
			return nil, nil, f.err
		}
		f.s.touch()
		return f.s, nil, nil
	}
	if encode && m.encodesLocked() >= m.maxEncodes {
		return nil, nil, ErrBusyEncoding
	}
	f := &liveStart{done: make(chan struct{}), encode: encode}
	m.starting[k] = f
	return nil, f, nil
}

// encodesLocked counts streams converting video, running or starting.
func (m *liveManager) encodesLocked() int {
	n := 0
	for _, s := range m.sessions {
		if !s.CopyVideo && s.running() {
			n++
		}
	}
	for _, f := range m.starting {
		if f.encode {
			n++
		}
	}
	return n
}

// finish records how a start went and wakes anyone waiting on it.
func (m *liveManager) finish(k string, f *liveStart, s *LiveSession, err error) {
	m.mu.Lock()
	if err == nil {
		m.sessions[s.ID] = s
	}
	delete(m.starting, k)
	f.s, f.err = s, err
	m.mu.Unlock()
	close(f.done)
}

func (m *liveManager) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

// Spec says how to stream a channel or recording.
type Spec struct {
	Height     int    // output height when transcoding
	CopyVideo  bool   // repackage the broadcast's video as is (height is then the source's)
	CopyAudio  bool   // repackage the broadcast's audio as is (e.g. AC-3 for a Roku)
	VideoCodec string // the broadcast's video codec, normalized ("mpeg2", "h264", "hevc"); "" if unknown

	window int // segments kept in the playlist; 0 keeps all (a recording watched from its start)
}

// liveWindow keeps 3 hours of a tuner stream (the rewind the live player
// offers); older segments are deleted, so a TV left on overnight doesn't
// fill the disk.
const liveWindow = 3 * 3600 / liveSegDur

// start tunes a channel, or joins a running stream of it at the same quality.
// It returns once the first playlist exists, or with an error if the tuner
// refused (all tuners busy) or ffmpeg failed.
func (m *liveManager) start(ctx context.Context, channel, name, streamURL string, spec Spec) (*LiveSession, error) {
	spec.window = liveWindow
	return m.startInput(ctx, "ch:"+channel, channel, name, []string{
		"-rw_timeout", "15000000", // 15s without data from the tuner → give up
		"-fflags", "+genpts+discardcorrupt", "-err_detect", "ignore_err", "-i", streamURL,
	}, spec)
}

// startRecordingPlayback streams a recording that's still being written, from
// its beginning: ffmpeg follows the growing file instead of stopping at EOF.
func (m *liveManager) startRecordingPlayback(ctx context.Context, recID int64, channel, name, file string, spec Spec) (*LiveSession, error) {
	return m.startInput(ctx, fmt.Sprintf("rec:%d", recID), channel, name, []string{
		"-follow", "1", "-rw_timeout", "30000000",
		"-fflags", "+genpts+discardcorrupt", "-err_detect", "ignore_err", "-i", "file:" + file,
	}, spec)
}

// gpuCodecs maps broadcast codecs to the ffprobe names transcode.HWDecodable uses.
var gpuCodecs = map[string]string{"mpeg2": "mpeg2video", "h264": "h264", "hevc": "hevc"}

func (m *liveManager) startInput(ctx context.Context, key, channel, name string, input []string, spec Spec) (*LiveSession, error) {
	if spec.CopyVideo {
		spec.Height = 0 // the broadcast's own size
	} else {
		spec.Height = snapHeight(spec.Height)
	}
	k := fmt.Sprintf("%s|%d|%t|%t", key, spec.Height, spec.CopyVideo, spec.CopyAudio)
	m.mu.Lock()
	joined, f, err := m.claimLocked(ctx, k, func(s *LiveSession) bool {
		return s.key == key && s.Height == spec.Height && s.CopyVideo == spec.CopyVideo && s.CopyAudio == spec.CopyAudio
	}, !spec.CopyVideo)
	m.mu.Unlock()
	if err != nil || joined != nil {
		return joined, err
	}
	s, err := m.launchInput(ctx, key, channel, name, input, spec)
	m.finish(k, f, s, err)
	if err != nil {
		return nil, err
	}
	slog.Info("live tv", "channel", channel, "name", name, "height", spec.Height, "hw", s.HW, "gpuDecode", s.HWDecode,
		"copyVideo", spec.CopyVideo, "copyAudio", spec.CopyAudio, "session", s.ID)
	return s, nil
}

// launchInput starts ffmpeg for a new stream, falling back to CPU decoding.
func (m *liveManager) launchInput(ctx context.Context, key, channel, name string, input []string, spec Spec) (*LiveSession, error) {

	// Decode on the GPU when it passed the start-up test and knows the codec;
	// if that run fails (not a busy tuner), tune again with the CPU decoding.
	hwDecode := !spec.CopyVideo && m.enc.HWDecode && transcode.HWDecodable[gpuCodecs[spec.VideoCodec]]
	s, err := m.launch(ctx, key, channel, name, input, spec, hwDecode)
	if err != nil && hwDecode && !errors.Is(err, ErrNoTuner) && ctx.Err() == nil {
		slog.Warn("live tv: GPU decoding failed, decoding on the CPU instead", "channel", channel, "err", err)
		s, err = m.launch(ctx, key, channel, name, input, spec, false)
	}
	return s, err
}

// liveArgs builds the ffmpeg command for one live stream.
func (m *liveManager) liveArgs(input []string, spec Spec, hwDecode bool, dir string) []string {
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error"}
	var vIn, vOut []string
	if !spec.CopyVideo {
		vIn, vOut = m.enc.Video(transcode.VideoOpts{MaxHeight: spec.Height, BitrateK: transcode.BitrateFor(spec.Height),
			Deinterlace: true, Live: true, HWDecode: hwDecode})
	}
	args = append(args, vIn...)
	args = append(args, input...)
	args = append(args, "-map", "0:v:0", "-map", "0:a:0", "-sn", "-dn")
	if spec.CopyVideo {
		// Segments are cut at the broadcast's own keyframes (MPEG-2 sends one about every half second).
		args = append(args, "-c:v", "copy")
	} else {
		args = append(args, vOut...)
		args = append(args, "-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", liveSegDur))
	}
	if spec.CopyAudio {
		args = append(args, "-c:a", "copy")
	} else {
		args = append(args, transcode.AudioArgs()...)
	}
	args = append(args, "-f", "hls", "-hls_time", strconv.Itoa(liveSegDur))
	if spec.window > 0 {
		args = append(args, "-hls_list_size", strconv.Itoa(spec.window), "-hls_delete_threshold", "1",
			"-hls_flags", "temp_file+independent_segments+delete_segments")
	} else {
		args = append(args, "-hls_list_size", "0", "-hls_playlist_type", "event", "-hls_flags", "temp_file+independent_segments")
	}
	return append(args, "-hls_segment_filename", filepath.Join(dir, "seg%d.ts"), filepath.Join(dir, "index.m3u8"))
}

// launch starts ffmpeg and waits for its first playlist.
func (m *liveManager) launch(ctx context.Context, key, channel, name string, input []string, spec Spec, hwDecode bool) (*LiveSession, error) {
	id := randomID()
	dir := filepath.Join(m.root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	cmd := exec.Command(m.enc.FFmpeg, m.liveArgs(input, spec, hwDecode, dir)...)
	stderr := &syncBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start ffmpeg: %w", err)
	}
	hw := m.enc.HW
	if spec.CopyVideo {
		hw = "" // nothing encoded
	}
	s := &LiveSession{ID: id, key: key, Channel: channel, Name: name, Height: spec.Height, HW: hw, Started: time.Now(),
		CopyVideo: spec.CopyVideo, CopyAudio: spec.CopyAudio, HWDecode: hwDecode,
		dir: dir, cmd: cmd, exited: make(chan struct{}), stderr: stderr, lastAccess: time.Now()}
	go func() {
		_ = cmd.Wait()
		close(s.exited)
	}()

	// Wait for the first playlist so the player never sees a 404.
	deadline := time.Now().Add(liveStartWait)
	playlist := filepath.Join(dir, "index.m3u8")
	for {
		if _, err := os.Stat(playlist); err == nil {
			break
		}
		if !s.running() {
			msg := strings.TrimSpace(stderr.String())
			_ = os.RemoveAll(dir)
			if strings.Contains(msg, "503") || strings.Contains(strings.ToLower(msg), "service unavailable") {
				return nil, ErrNoTuner
			}
			if msg == "" {
				msg = "ffmpeg exited without output"
			}
			return nil, fmt.Errorf("couldn't tune %s: %s", channel, lastLine(msg))
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			_ = cmd.Process.Kill()
			<-s.exited
			_ = os.RemoveAll(dir)
			return nil, errors.New("the tuner didn't deliver video in time; weak signal?")
		}
		time.Sleep(150 * time.Millisecond)
	}
	return s, nil
}

func (m *liveManager) get(id string) *LiveSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[id]
}

// file returns a playlist or segment path inside a session, touching it.
func (m *liveManager) file(id, name string) (string, error) {
	s := m.get(id)
	if s == nil {
		return "", os.ErrNotExist
	}
	if name != "index.m3u8" && !(strings.HasPrefix(name, "seg") && strings.HasSuffix(name, ".ts") && !strings.ContainsAny(name, `/\`)) {
		return "", os.ErrNotExist
	}
	s.touch()
	p := filepath.Join(s.dir, name)
	if _, err := os.Stat(p); err != nil {
		return "", err
	}
	return p, nil
}

// leave marks a viewer gone. The stream stops at the next reap unless
// another viewer of the same channel keeps requesting it.
func (m *liveManager) leave(id string) {
	if s := m.get(id); s != nil {
		s.mu.Lock()
		s.lastAccess = time.Now().Add(-liveIdleKill + 3*time.Second)
		s.mu.Unlock()
	}
}

func (m *liveManager) stop(s *LiveSession) {
	m.mu.Lock()
	delete(m.sessions, s.ID)
	m.mu.Unlock()
	if s.running() {
		if s.cancel != nil {
			s.cancel()
		} else {
			_ = s.cmd.Process.Kill()
		}
		<-s.exited
	}
	_ = os.RemoveAll(s.dir)
	slog.Info("live tv stopped", "channel", s.Channel, "session", s.ID)
}

func (m *liveManager) sessionsList() []*LiveSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*LiveSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s)
	}
	return out
}

func (m *liveManager) run(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			for _, s := range m.sessionsList() {
				m.stop(s)
			}
			return
		case <-t.C:
		}
		for _, s := range m.sessionsList() {
			if s.idle() > liveIdleKill || !s.running() {
				m.stop(s)
			}
		}
	}
}

// syncBuffer collects ffmpeg stderr from its own goroutine.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.buf.Len() > 8192 {
		b.buf.Reset()
	}
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func randomID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		s = s[i+1:]
	}
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

// stopKey ends every session streaming key (a channel whose settings changed).
func (m *liveManager) stopKey(key string) {
	for _, s := range m.sessionsList() {
		if s.key == key {
			m.stop(s)
		}
	}
}
