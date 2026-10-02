package livetv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/timothydodd/couchside/internal/metadata"
	"github.com/timothydodd/couchside/internal/parse"
)

const (
	settingRecordingsDir = "dvr.recordings_dir"
	settingPadBefore     = "dvr.pad_before" // seconds
	settingPadAfter      = "dvr.pad_after"
)

// Padding is how many seconds recordings start early and run late: the
// Settings choice, or COUCHSIDE_DVR_PAD_BEFORE/AFTER.
func (s *Service) Padding(ctx context.Context) (before, after int64) {
	get := func(key string, def time.Duration) int64 {
		if v, err := s.db.Setting(ctx, key); err == nil && v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
				return n
			}
		}
		return int64(def / time.Second)
	}
	return get(settingPadBefore, s.cfg.PadBefore), get(settingPadAfter, s.cfg.PadAfter)
}

// SetPadding saves the padding and applies it to recordings that haven't
// started yet. Ones already recording keep the times they started with.
func (s *Service) SetPadding(ctx context.Context, before, after int64) error {
	if err := s.db.SetSetting(ctx, settingPadBefore, strconv.FormatInt(before, 10)); err != nil {
		return err
	}
	if err := s.db.SetSetting(ctx, settingPadAfter, strconv.FormatInt(after, 10)); err != nil {
		return err
	}
	if err := s.db.SetScheduledPadding(ctx, before, after); err != nil {
		return err
	}
	s.wake()
	return nil
}

// RecordingsDir is where new recordings go: the folder chosen in Settings,
// or COUCHSIDE_RECORDINGS_DIR.
func (s *Service) RecordingsDir(ctx context.Context) string {
	if v, err := s.db.Setting(ctx, settingRecordingsDir); err == nil && v != "" {
		return v
	}
	return s.cfg.RecordingsDir
}

func (s *Service) DefaultRecordingsDir() string { return s.cfg.RecordingsDir }

// CheckWritable confirms Couchside can create files in dir (creating it if
// its parent exists).
func CheckWritable(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("can't create %s: %w", dir, err)
	}
	f, err := os.CreateTemp(dir, ".couchside-write-test-*")
	if err != nil {
		if errors.Is(err, syscall.EROFS) {
			return fmt.Errorf("%s is on a read-only mount; make the media share writable to record there", dir)
		}
		return fmt.Errorf("can't write to %s: %w", dir, err)
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

// CheckWritableNoCreate tests an existing folder without creating it.
func CheckWritableNoCreate(dir string) error {
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return fmt.Errorf("folder not found")
	}
	f, err := os.CreateTemp(dir, ".couchside-write-test-*")
	if err != nil {
		if errors.Is(err, syscall.EROFS) {
			return fmt.Errorf("read-only: the media share is mounted read-only")
		}
		return fmt.Errorf("not writable: %v", err)
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

// MoveResult reports what SetRecordingsDir moved.
type MoveResult struct {
	Moved   int      `json:"moved"`
	Failed  []string `json:"failed"`
	Library string   `json:"library"` // library the new folder feeds
}

// SetRecordingsDir switches where new recordings go, optionally moving
// finished recordings over. Recordings in progress finish where they started.
func (s *Service) SetRecordingsDir(ctx context.Context, dir string, move bool) (MoveResult, error) {
	var res MoveResult
	dir = filepath.Clean(dir)
	if err := CheckWritable(dir); err != nil {
		return res, err
	}
	old := s.RecordingsDir(ctx)
	oldLib, _ := s.db.LibraryByPath(ctx, old)
	if err := s.db.SetSetting(ctx, settingRecordingsDir, dir); err != nil {
		return res, err
	}
	s.ensureLibrary(ctx)
	if lib, _ := s.db.LibraryContaining(ctx, dir); lib != nil {
		res.Library = lib.Name
	}

	if move && old != dir {
		recs, err := s.db.RecordingsWithStatus(ctx, "completed")
		if err != nil {
			return res, err
		}
		for _, r := range recs {
			if r.Path == "" || !within(r.Path, old) {
				continue
			}
			dst := s.pathInDir(dir, recordingName(r, s.showYear(ctx, r, false))) // fresh, clean name in the new folder
			if err := moveFile(r.Path, dst); err != nil {
				res.Failed = append(res.Failed, fmt.Sprintf("%s: %v", r.Title, err))
				continue
			}
			removeEmptyParents(filepath.Dir(r.Path), old)
			_ = s.db.SetRecordingPath(ctx, r.ID, dst)
			res.Moved++
		}
		// Rescan the old library so moved files drop out of it.
		if oldLib != nil && s.work != nil {
			_ = s.work.Enqueue(ctx, "scan", oldLib.ID, "Scan "+oldLib.Name)
			// The auto-created DVR library is pointless once emptied into a real one.
			if oldLib.Name == "DVR Recordings" && oldLib.Path == old && res.Moved > 0 && len(res.Failed) == 0 && !hasFiles(old) {
				if lib, _ := s.db.LibraryContaining(ctx, dir); lib == nil || lib.ID != oldLib.ID {
					_ = s.db.DeleteLibrary(ctx, oldLib.ID)
				}
			}
		}
	}
	s.scanRecordings(ctx)
	slog.Info("recordings folder changed", "from", old, "to", dir, "moved", res.Moved, "failed", len(res.Failed))
	return res, nil
}

// pathInDir picks a file name for a recording inside dir, reusing a matching
// show folder ("The Simpsons (1989)") and season folder ("Season 07") so the
// recording joins the existing show instead of creating a duplicate. When
// the series year is known, a same-titled folder of another series
// ("MacGyver (1985)", or a plain "MacGyver" matched to 1985) isn't reused.
func (s *Service) pathInDir(dir string, n recName) string {
	show, season, file := n.show, n.season, n.file
	showDir := matchDir(dir, show, func(name string) bool {
		pn := parse.Name(name)
		if metadata.Normalize(pn.Title) != metadata.Normalize(n.title) {
			return false
		}
		if n.year == 0 || pn.Year == n.year {
			return true
		}
		if pn.Year != 0 {
			return false
		}
		fy, _ := s.db.FolderSeriesYear(context.Background(), filepath.Join(dir, name))
		return fy == 0 || (fy >= n.year-1 && fy <= n.year+1)
	})
	num := seasonNumber(season)
	seasonDir := matchDir(filepath.Join(dir, showDir), season, func(name string) bool {
		return num >= 0 && seasonNumber(name) == num
	})
	base := filepath.Join(dir, showDir, seasonDir)
	p := filepath.Join(base, file+".ts")
	for i := 2; s.pathTaken(p); i++ {
		p = filepath.Join(base, fmt.Sprintf("%s (%d).ts", file, i))
	}
	return p
}

// pathTaken reports whether a new recording can't use p: the file exists,
// or another recording is still writing its parts there (the final file
// only appears when it finishes).
func (s *Service) pathTaken(p string) bool {
	if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
		return true
	}
	if len(partFiles(p)) > 0 {
		return true
	}
	taken, err := s.db.RecordingPathActive(context.Background(), p)
	if err != nil {
		slog.Warn("dvr: check recording path", "path", p, "err", err)
	}
	return taken
}

// matchDir returns the name of an existing subfolder of parent that matches,
// or fallback when none does.
func matchDir(parent, fallback string, match func(string) bool) string {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return fallback
	}
	for _, e := range entries {
		if e.IsDir() && e.Name() == fallback {
			return fallback
		}
	}
	for _, e := range entries {
		if e.IsDir() && match(e.Name()) {
			return e.Name()
		}
	}
	return fallback
}

func seasonNumber(name string) int {
	l := strings.ToLower(strings.TrimSpace(name))
	for _, p := range []string{"season ", "season", "s"} {
		if strings.HasPrefix(l, p) {
			if n, err := strconv.Atoi(strings.TrimSpace(l[len(p):])); err == nil {
				return n
			}
		}
	}
	return -1
}

func within(p, dir string) bool {
	return strings.HasPrefix(filepath.Clean(p), filepath.Clean(dir)+string(os.PathSeparator))
}

func hasFiles(dir string) bool {
	found := false
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// moveFile renames, falling back to copy+delete across filesystems (Docker
// volume → SMB share).
func moveFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".moving"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Remove(src)
}
