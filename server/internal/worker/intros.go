package worker

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"

	"github.com/timothydodd/couchside/internal/audiofp"
	"github.com/timothydodd/couchside/internal/db"
)

// Intro detection: a season's episodes open with the same theme, so the
// stretch of audio two neighbouring episodes share near their start is the
// intro. A job per series (encode pool, after encodes) fingerprints the
// first part of each episode's sound and compares neighbours (audiofp). What
// it finds is stored as a 'detected' intro, which a file's own chapter marks
// and an admin's marks both take precedence over.
const (
	KindIntros = "intros" // ref: series item id

	introScanSec  = 600.0 // how far into an episode an intro is looked for…
	introScanPart = 0.4   // …and never past this share of it
	introMinSec   = 15.0  // shorter than this is a sting or a logo, not an intro
	introMaxSec   = 180.0 // longer is two copies of the same episode, or a recap of one
)

// QueueIntros queues intro detection for a library's series that have
// episodes it hasn't looked at.
func (w *Worker) QueueIntros(ctx context.Context, libraryID int64) (int, error) {
	series, err := w.db.SeriesNeedingIntros(ctx, libraryID)
	if err != nil {
		return 0, err
	}
	for _, s := range series {
		if err := w.db.Enqueue(ctx, KindIntros, s.ID, "Find intros "+s.Title); err != nil {
			return 0, err
		}
	}
	if len(series) > 0 {
		w.Wake()
	}
	return len(series), nil
}

func (w *Worker) intros(ctx context.Context, jobID, itemID int64) error {
	files, err := w.db.EpisodeFiles(ctx, itemID)
	if err != nil {
		return err
	}
	// Season by season: themes change between seasons.
	found, looked := 0, 0
	for lo := 0; lo < len(files); {
		hi := lo
		for hi < len(files) && files[hi].Season == files[lo].Season {
			hi++
		}
		f, l, err := w.seasonIntros(ctx, files[lo:hi])
		if err != nil {
			return err
		}
		found, looked = found+f, looked+l
		lo = hi
	}
	if looked > 0 {
		_ = w.db.SetJobResult(context.WithoutCancel(ctx), jobID, fmt.Sprintf("Found an intro in %d of %d episodes", found, looked))
	}
	return nil
}

// seasonIntros looks for the intro in a season's episodes that haven't been
// looked at, and reports how many it found and looked at.
func (w *Worker) seasonIntros(ctx context.Context, eps []db.EpisodeFile) (found, looked int, err error) {
	todo := false
	for _, e := range eps {
		todo = todo || !e.Checked
	}
	if !todo || len(eps) < 2 {
		return 0, 0, nil // nothing new, or nothing to compare a lone episode with
	}
	prints := make([][]uint16, len(eps))
	print := func(i int) []uint16 {
		if prints[i] == nil {
			fp, err := w.fingerprint(ctx, eps[i])
			if err != nil && ctx.Err() == nil {
				slog.Warn("intros: couldn't read an episode's sound", "file", eps[i].Path, "err", err)
			}
			if fp == nil {
				fp = []uint16{} // tried
			}
			prints[i] = fp
		}
		return prints[i]
	}
	for i, e := range eps {
		if e.Checked {
			continue
		}
		if ctx.Err() != nil {
			return found, looked, ctx.Err()
		}
		looked++
		// The next episode first, then the one before: one of them may be a
		// special, or the season's first episode may have a longer opening.
		for _, j := range []int{i + 1, i - 1, i + 2} {
			if j < 0 || j >= len(eps) || e.HasIntro {
				continue
			}
			s, ok := audiofp.Common(print(i), print(j), introMinSec)
			if !ok || s.Length > introMaxSec {
				continue
			}
			if err := w.db.SetSegment(ctx, e.FileID, db.MarkedSegment{Kind: "intro", Start: s.StartA, End: s.StartA + s.Length, Source: "detected"}); err != nil {
				return found, looked, err
			}
			found++
			break
		}
		if ctx.Err() != nil {
			return found, looked, ctx.Err() // cut short: not a verdict on this episode
		}
		if err := w.db.MarkIntroChecked(ctx, e); err != nil {
			return found, looked, err
		}
	}
	return found, looked, nil
}

// fingerprint reads the start of an episode's first audio track and reduces
// it to an audiofp fingerprint.
func (w *Worker) fingerprint(ctx context.Context, e db.EpisodeFile) ([]uint16, error) {
	limit := introScanSec
	if e.DurationSec > 0 {
		limit = min(limit, e.DurationSec*introScanPart)
	}
	out, err := exec.CommandContext(ctx, w.cfg.FFmpeg, "-hide_banner", "-nostdin", "-loglevel", "error",
		"-t", strconv.FormatFloat(limit, 'f', 1, 64), "-i", e.Path, "-map", "0:a:0", "-vn", "-sn", "-dn",
		"-ac", "1", "-ar", strconv.Itoa(audiofp.Rate), "-f", "s16le", "-").Output()
	if err != nil {
		return nil, err
	}
	pcm := make([]int16, len(out)/2)
	for i := range pcm {
		pcm[i] = int16(binary.LittleEndian.Uint16(out[2*i:]))
	}
	return audiofp.Fingerprint(pcm), nil
}
