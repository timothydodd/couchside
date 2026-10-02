package worker

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/timothydodd/couchside/internal/db"
)

// KindCommercials runs comskip over a file and stores the commercial breaks
// it finds. The player marks them on the timeline and skips them. Finished
// DVR recordings get one automatically; any file can be queued from the player.
const KindCommercials = "commercials" // ref: file id

// defaultComskipINI is used unless COUCHSIDE_COMSKIP_INI names a tuned one.
// Couchside reads the .edl file. Detection settings not listed keep comskip's
// defaults; each change here says why.
const defaultComskipINI = `; Written by Couchside. Set COUCHSIDE_COMSKIP_INI to use your own comskip.ini
; (keep output_edl=1, Couchside reads the .edl file).
output_edl=1
output_default=0
verbose=0

; A block counts as show (its commercial score is cut to 1%) when the channel
; logo is on screen for more of it than this. comskip's default, 0.25, keeps
; the network promos and station IDs at the end of a break, which show the
; logo briefly, so skips ended early. Half the block is still easily met by
; real show segments, where the logo is up throughout.
logo_percentage_threshold=0.5
`

// detectComskip resolves the comskip binary, or returns "" when it isn't installed.
func detectComskip(bin string) string {
	if bin == "" {
		return ""
	}
	p, err := exec.LookPath(bin)
	if err != nil {
		slog.Info("comskip not found: commercial detection is off", "comskip", bin)
		return ""
	}
	slog.Info("commercial detection enabled", "comskip", p)
	return p
}

// CommercialsAvailable reports whether comskip is installed.
func (w *Worker) CommercialsAvailable() bool { return w.comskip != "" }

// EnqueueCommercials queues commercial detection for a file.
func (w *Worker) EnqueueCommercials(ctx context.Context, fileID int64, path string) error {
	return w.Enqueue(ctx, KindCommercials, fileID, "Find commercials "+filepath.Base(path))
}

var rePercent = regexp.MustCompile(`(\d+)%\s*$`)

func (w *Worker) commercials(ctx context.Context, jobID, fileID int64) error {
	if w.comskip == "" {
		return errors.New("comskip isn't installed")
	}
	f, err := w.db.File(ctx, fileID)
	if err != nil {
		return err
	}
	if f.Problem != "" {
		return fmt.Errorf("the file can't be read (%s)", f.Problem)
	}
	ini, err := w.comskipINI()
	if err != nil {
		return err
	}
	out, err := os.MkdirTemp(filepath.Join(w.cfg.CacheDir, "comskip"), fmt.Sprintf("%d-", fileID))
	if err != nil {
		return err
	}
	defer os.RemoveAll(out)

	cmd := exec.CommandContext(ctx, w.comskip, "--ini="+ini, "--output="+out, "--output-filename=couchside", f.Path)
	cmd.Dir = out // comskip may write logs next to where it runs
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	tail := &tailBuffer{max: 4 << 10}
	cmd.Stdout = tail
	if err := cmd.Start(); err != nil {
		return err
	}
	// Progress lines end in "NN%\r"; everything is kept for the error message.
	last := time.Time{}
	sc := bufio.NewScanner(stderr)
	sc.Split(scanLinesCR)
	for sc.Scan() {
		line := sc.Text()
		tail.Write([]byte(line + "\n"))
		m := rePercent.FindStringSubmatch(line)
		if m == nil || time.Since(last) < 2*time.Second {
			continue
		}
		if pct, err := strconv.Atoi(m[1]); err == nil {
			last = time.Now()
			_ = w.db.SetJobProgress(ctx, jobID, min(0.99, float64(pct)/100))
		}
	}
	err = cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// comskip exits 1 when it found commercials and 0 when it found none, but
	// also exits 1 on some errors: the .edl file tells them apart.
	edl := filepath.Join(out, "couchside.edl")
	body, readErr := os.ReadFile(edl)
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit) && exit.ExitCode() == 1 && readErr == nil:
	default:
		return fmt.Errorf("comskip: %v: %s", err, lastLine(tail.String()))
	}
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	segs := parseEDL(string(body))
	_ = w.db.SetJobProgress(ctx, jobID, 1)
	slog.Info("commercials found", "file", f.Path, "breaks", len(segs))
	return w.db.SetCommercials(ctx, fileID, f.Size, f.Mtime, segs)
}

// comskipINI returns the configured comskip.ini, or writes Couchside's default to the cache.
func (w *Worker) comskipINI() (string, error) {
	if w.cfg.ComskipINI != "" {
		return w.cfg.ComskipINI, nil
	}
	p := filepath.Join(w.cfg.CacheDir, "comskip", "comskip.ini")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	if cur, err := os.ReadFile(p); err == nil && string(cur) == defaultComskipINI {
		return p, nil
	}
	return p, os.WriteFile(p, []byte(defaultComskipINI), 0o644)
}

// parseEDL reads MPlayer EDL lines ("start end action", in seconds), sorted
// and with overlapping or touching breaks merged.
func parseEDL(s string) []db.Segment {
	segs := []db.Segment{}
	for _, line := range strings.Split(s, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		start, err1 := strconv.ParseFloat(fields[0], 64)
		end, err2 := strconv.ParseFloat(fields[1], 64)
		if err1 != nil || err2 != nil || end <= start {
			continue
		}
		segs = append(segs, db.Segment{Start: max(0, start), End: end})
	}
	sort.Slice(segs, func(i, j int) bool { return segs[i].Start < segs[j].Start })
	out := segs[:0]
	for _, s := range segs {
		if n := len(out); n > 0 && s.Start <= out[n-1].End+1 {
			out[n-1].End = max(out[n-1].End, s.End)
			continue
		}
		out = append(out, s)
	}
	return out
}

// scanLinesCR splits on \n or \r, since progress output rewrites one line with \r.
func scanLinesCR(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// tailBuffer keeps the last max bytes written to it. comskip's stdout and
// stderr both feed it, from different goroutines.
type tailBuffer struct {
	max int
	mu  sync.Mutex
	buf []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if over := len(t.buf) - t.max; over > 0 {
		t.buf = t.buf[over:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

// cleanupComskip removes work folders left by comskip runs a restart cut off.
func (w *Worker) cleanupComskip() {
	dir := filepath.Join(w.cfg.CacheDir, "comskip")
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			_ = os.RemoveAll(filepath.Join(dir, e.Name()))
		}
	}
}
