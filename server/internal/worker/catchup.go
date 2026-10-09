package worker

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/timothydodd/couchside/internal/db"
)

// catchUp queues the per-file work a library's files are missing. The scan
// only queues it for new and changed files, so a file indexed before a
// feature existed (or was switched on, or while comskip wasn't installed, or
// whose cache was wiped) would otherwise never get it. It runs after every
// completed scan. A file whose last try failed waits until that job is
// pruned from Activity (or retried there), so every scan doesn't try again.
func (w *Worker) catchUp(ctx context.Context, lib db.Library) error {
	stills, err := w.db.FilesNeedingStills(ctx, lib.ID)
	if err != nil {
		return err
	}
	for _, f := range stills {
		if err := w.db.Enqueue(ctx, KindStill, f.ID, "Still "+filepath.Base(f.Path)); err != nil {
			return err
		}
	}
	commercials := 0
	if w.comskip != "" {
		recs, err := w.db.RecordingsNeedingCommercials(ctx, lib.ID)
		if err != nil {
			return err
		}
		for _, f := range recs {
			if err := w.db.Enqueue(ctx, KindCommercials, f.ID, "Find commercials "+filepath.Base(f.Path)); err != nil {
				return err
			}
		}
		commercials = len(recs)
	}
	previews := 0
	if lib.Trickplay {
		if previews, err = w.queueTrickplay(ctx, lib.ID, false); err != nil {
			return err
		}
	}
	intros := 0
	if lib.Intros && lib.Kind == "tv" {
		if intros, err = w.QueueIntros(ctx, lib.ID); err != nil {
			return err
		}
	}
	// Files indexed before the dynamic range was recorded: one job reads them all.
	ranges, err := w.db.FilesNeedingDynamicRange(ctx, lib.ID)
	if err != nil {
		return err
	}
	if len(ranges) > 0 {
		if err := w.db.Enqueue(ctx, KindDynamicRange, lib.ID, "Read HDR info "+lib.Name); err != nil {
			return err
		}
	}
	if len(stills)+commercials+previews+intros+len(ranges) > 0 {
		slog.Info("catching up existing files", "library", lib.Name, "stills", len(stills), "commercials", commercials,
			"previews", previews, "intros", intros, "dynamicRange", len(ranges))
		w.Wake()
	}
	return nil
}
