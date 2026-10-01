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

	key        string // what's being streamed: "ch:2.1" or "rec:42"
	dir        string
	cmd        *exec.Cmd
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
	enc  transcode.Encoder
	root string

	mu       sync.Mutex
	sessions map[string]*LiveSession
}

func newLiveManager(enc transcode.Encoder, root string) (*liveManager, error) {
	_ = os.RemoveAll(root)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &liveManager{enc: enc, root: root, sessions: map[string]*LiveSession{}}, nil
}

func (m *liveManager) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

// start tunes a channel, or joins a running stream of it at the same quality.
// It returns once the first playlist exists, or with an error if the tuner
// refused (all tuners busy) or ffmpeg failed.
func (m *liveManager) start(ctx context.Context, channel, name, streamURL string, height int) (*LiveSession, error) {
	return m.startInput(ctx, "ch:"+channel, channel, name, []string{
		"-rw_timeout", "15000000", // 15s without data from the tuner → give up
		"-fflags", "+genpts+discardcorrupt", "-err_detect", "ignore_err", "-i", streamURL,
	}, height)
}

// startRecordingPlayback streams a recording that's still being written, from
// its beginning: ffmpeg follows the growing file instead of stopping at EOF.
func (m *liveManager) startRecordingPlayback(ctx context.Context, recID int64, channel, name, file string, height int) (*LiveSession, error) {
	return m.startInput(ctx, fmt.Sprintf("rec:%d", recID), channel, name, []string{
		"-follow", "1", "-rw_timeout", "30000000",
		"-fflags", "+genpts+discardcorrupt", "-err_detect", "ignore_err", "-i", "file:" + file,
	}, height)
}

func (m *liveManager) startInput(ctx context.Context, key, channel, name string, input []string, height int) (*LiveSession, error) {
	m.mu.Lock()
	for _, s := range m.sessions {
		if s.key == key && s.Height == height && s.running() {
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
	vIn, vOut := m.enc.Video(transcode.VideoOpts{MaxHeight: height, BitrateK: transcode.BitrateFor(height), Deinterlace: true, Live: true})
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error"}
	args = append(args, vIn...)
	args = append(args, input...)
	args = append(args, "-map", "0:v:0", "-map", "0:a:0", "-sn", "-dn")
	args = append(args, vOut...)
	args = append(args, "-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", liveSegDur))
	args = append(args, transcode.AudioArgs()...)
	args = append(args, "-f", "hls", "-hls_time", strconv.Itoa(liveSegDur), "-hls_list_size", "0",
		"-hls_playlist_type", "event", "-hls_flags", "temp_file+independent_segments",
		"-hls_segment_filename", filepath.Join(dir, "seg%d.ts"), filepath.Join(dir, "index.m3u8"))

	cmd := exec.Command(m.enc.FFmpeg, args...)
	stderr := &syncBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start ffmpeg: %w", err)
	}
	hw := m.enc.HW
	s := &LiveSession{ID: id, key: key, Channel: channel, Name: name, Height: height, HW: hw, Started: time.Now(),
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
	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()
	slog.Info("live tv", "channel", channel, "name", name, "height", height, "hw", hw, "session", id)
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
		_ = s.cmd.Process.Kill()
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
