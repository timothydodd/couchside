// Package worker runs background jobs from the SQLite queue: library scans,
// metadata matching, artwork and episode stills.
package worker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/imaging"
	"github.com/timothydodd/couchside/internal/metadata"
	"github.com/timothydodd/couchside/internal/transcode"
)

const (
	KindScan    = "scan"    // ref: library id
	KindMatch   = "match"   // ref: media item id
	KindArtwork = "artwork" // ref: media item id
	KindStill   = "still"   // ref: file id
)

type Worker struct {
	db        *db.DB
	cfg       config.Config
	providers *metadata.Chain
	ff        imaging.FFmpeg
	enc       transcode.Encoder
	wake      chan struct{}
	wakeEnc   chan struct{}

	mu      sync.Mutex
	cancels map[int64]context.CancelFunc // running job id → cancel
}

func New(d *db.DB, cfg config.Config, providers *metadata.Chain, enc transcode.Encoder) *Worker {
	return &Worker{db: d, cfg: cfg, providers: providers, ff: imaging.FFmpeg{Bin: cfg.FFmpeg}, enc: enc,
		wake: make(chan struct{}, 1), wakeEnc: make(chan struct{}, 1), cancels: map[int64]context.CancelFunc{}}
}

// Cancel stops a running job or drops a queued one. Reports whether anything was cancelled.
func (w *Worker) Cancel(ctx context.Context, jobID int64) (bool, error) {
	w.mu.Lock()
	cancel, ok := w.cancels[jobID]
	w.mu.Unlock()
	if ok {
		cancel()
		return true, nil
	}
	return w.db.CancelQueuedJob(ctx, jobID)
}

// Enqueue queues a job and wakes an idle worker.
func (w *Worker) Enqueue(ctx context.Context, kind string, ref int64, label string) error {
	if err := w.db.Enqueue(ctx, kind, ref, label); err != nil {
		return err
	}
	w.Wake()
	return nil
}

func (w *Worker) Wake() {
	for _, c := range []chan struct{}{w.wake, w.wakeEnc} {
		select {
		case c <- struct{}{}:
		default:
		}
	}
}

// Run starts the worker pool and the periodic scanner, blocking until ctx ends.
func (w *Worker) Run(ctx context.Context) {
	if err := w.db.RequeueRunning(ctx); err != nil {
		slog.Error("requeue running jobs", "err", err)
	}
	w.cleanupOptimized(ctx)
	var wg sync.WaitGroup
	for i := 0; i < w.cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.loop(ctx, false)
		}()
	}
	for i := 0; i < w.cfg.EncodeWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.loop(ctx, true)
		}()
	}
	if w.cfg.ScanInterval > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.periodicScan(ctx)
		}()
	}
	wg.Wait()
}

func (w *Worker) loop(ctx context.Context, encode bool) {
	idle := time.NewTicker(5 * time.Second)
	defer idle.Stop()
	wake := w.wake
	if encode {
		wake = w.wakeEnc
	}
	for {
		job, err := w.db.ClaimJob(ctx, encode)
		if err != nil && ctx.Err() == nil {
			slog.Error("claim job", "err", err)
		}
		if job == nil {
			select {
			case <-ctx.Done():
				return
			case <-wake:
			case <-idle.C:
			}
			continue
		}
		// More work may be waiting; let another idle worker pick it up.
		w.Wake()
		start := time.Now()
		jobCtx, cancel := context.WithCancel(ctx)
		w.mu.Lock()
		w.cancels[job.ID] = cancel
		w.mu.Unlock()
		err = w.handle(jobCtx, job)
		w.mu.Lock()
		delete(w.cancels, job.ID)
		w.mu.Unlock()
		cancelled := jobCtx.Err() != nil
		cancel()
		if ctx.Err() != nil {
			return // shutting down: leave it running, RequeueRunning picks it up
		}
		if cancelled && err != nil {
			err = errCancelled
		}
		if err != nil {
			slog.Warn("job failed", "kind", job.Kind, "ref", job.RefID, "label", job.Label, "err", err)
		} else {
			slog.Debug("job done", "kind", job.Kind, "ref", job.RefID, "took", time.Since(start))
		}
		if err == errCancelled {
			if derr := w.db.DeleteJob(context.WithoutCancel(ctx), job.ID); derr != nil {
				slog.Error("delete cancelled job", "err", derr)
			}
			continue
		}
		if ferr := w.db.FinishJob(context.WithoutCancel(ctx), job.ID, err); ferr != nil {
			slog.Error("finish job", "err", ferr)
		}
	}
}

func (w *Worker) handle(ctx context.Context, j *db.Job) error {
	switch j.Kind {
	case KindScan:
		return w.scan(ctx, j.RefID)
	case KindMatch:
		return w.match(ctx, j.RefID)
	case KindArtwork:
		return w.artwork(ctx, j.RefID)
	case KindStill:
		return w.still(ctx, j.RefID)
	case KindOptimize:
		return w.optimize(ctx, j.ID, j.RefID)
	}
	return fmt.Errorf("unknown job kind %q", j.Kind)
}

func (w *Worker) periodicScan(ctx context.Context) {
	t := time.NewTicker(w.cfg.ScanInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.ScanAll(ctx)
		}
	}
}

func (w *Worker) ScanAll(ctx context.Context) {
	libs, err := w.db.Libraries(ctx)
	if err != nil {
		slog.Error("list libraries", "err", err)
		return
	}
	for _, l := range libs {
		if err := w.Enqueue(ctx, KindScan, l.ID, "Scan "+l.Name); err != nil {
			slog.Error("enqueue scan", "err", err)
		}
	}
}
