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
;
; Detection is tuned for US broadcast recordings, with settings similar to
; Plex's. On sitcoms that fade to black between scenes, comskip's defaults took
; scenes for ads; these leave out silence cut points (detect_method 43 instead
; of 107) and score anything over 250s as show.
output_edl=1
output_default=0
verbose=0

detect_method=43
validate_silence=1
validate_uniform=1
validate_scenechange=1
max_brightness=60
test_brightness=40
max_avg_brightness=25
max_commercialbreak=600
min_commercialbreak=25
max_commercial_size=125
min_commercial_size=4
min_show_segment_length=250
non_uniformity=500
max_volume=500
min_silence=12
ticker_tape=0
logo_at_bottom=0
punish=0
punish_threshold=1.3
punish_modifier=2
intelligent_brightness=0
logo_percentile=0.92
logo_threshold=0.75
punish_no_logo=1
aggressive_logo_rejection=0
connect_blocks_with_logo=1
logo_filter=0
cut_on_ar_change=1
delete_show_after_last_commercial=0
delete_show_before_or_after_current=0
delete_block_after_commercial=0
remove_before=0
remove_after=0
shrink_logo=5
after_logo=0
padding=0
ms_audio_delay=5
volume_slip=40
skip_b_frames=0
max_repair_size=200
disable_heuristics=4
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
	// comskipINI only makes this folder when it writes the default ini.
	work := filepath.Join(w.cfg.CacheDir, "comskip")
	if err := os.MkdirAll(work, 0o755); err != nil {
		return err
	}
	out, err := os.MkdirTemp(work, fmt.Sprintf("%d-", fileID))
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
