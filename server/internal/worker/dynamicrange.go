package worker

import (
	"context"
	"errors"
	"log/slog"

	"github.com/timothydodd/couchside/internal/probe"
)

// KindDynamicRange reads the dynamic range (HDR10, HLG, Dolby Vision) of a
// library's files indexed before it was recorded. ref: library id.
const KindDynamicRange = "dynamicrange"

// dynamicRange probes each of the library's files that has no dynamic range
// yet. A file that can't be read now is recorded as SDR rather than left
// unknown, so it doesn't bring the job back after every scan; a re-scan of
// the file (or a change to it) reads it again.
func (w *Worker) dynamicRange(ctx context.Context, libraryID int64) error {
	files, err := w.db.FilesNeedingDynamicRange(ctx, libraryID)
	if err != nil {
		return err
	}
	found := 0
	for _, f := range files {
		p, err := probe.Probe(ctx, w.cfg.FFprobe, f.Path)
		if errors.Is(err, context.Canceled) {
			return err
		}
		if errors.Is(err, context.DeadlineExceeded) {
			// Slow share: leave it for the next catch-up.
			continue
		}
		dr, dv := "", 0
		if err != nil {
			slog.Warn("dynamic range: couldn't read file", "path", f.Path, "err", err)
		} else {
			dr, dv = p.DynamicRange(), p.DVProfile
		}
		if dr != "" {
			found++
		}
		if err := w.db.SetDynamicRange(ctx, f.ID, dr, dv); err != nil {
			return err
		}
	}
	slog.Info("dynamic range read", "library", libraryID, "files", len(files), "hdr", found)
	return nil
}
