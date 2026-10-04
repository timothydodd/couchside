package worker

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/timothydodd/couchside/internal/audiofp"
	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
)

// writeWAV saves 8 kHz mono samples as a WAV file ffmpeg can read.
func writeWAV(t *testing.T, path string, pcm []int16) {
	t.Helper()
	data := make([]byte, 44+2*len(pcm))
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(36+2*len(pcm)))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1) // PCM
	binary.LittleEndian.PutUint16(data[22:], 1) // mono
	binary.LittleEndian.PutUint32(data[24:], audiofp.Rate)
	binary.LittleEndian.PutUint32(data[28:], audiofp.Rate*2)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(2*len(pcm)))
	for i, v := range pcm {
		binary.LittleEndian.PutUint16(data[44+2*i:], uint16(v))
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// Three episodes whose sound differs except for a 30-second theme at a
// different place in each, and one episode without it. The job marks the
// intro where it is in each, leaves the odd one out unmarked, and doesn't
// look at any of them again. Uses the real ffmpeg.
func TestIntrosWithFFmpeg(t *testing.T) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	lib, _ := d.CreateLibrary(ctx, "TV", dir, "tv")
	show, _, _ := d.EnsureItem(ctx, lib, "series", "Show", 2020)

	const sec = audiofp.Rate
	theme := make([]int16, 30*sec)
	r := rand.New(rand.NewSource(1))
	note, next := 440.0, 0
	for i := range theme {
		if i >= next {
			note = 220 * math.Pow(2, float64(r.Intn(24))/12)
			next = i + sec/8 + r.Intn(sec/4)
		}
		x := float64(i) / sec
		theme[i] = int16(5000 * (math.Sin(2*math.Pi*note*x) + 0.5*math.Sin(3*math.Pi*note*x)))
	}
	themeAt := []int{40, 15, 70, -1} // seconds in; -1 = this episode has no theme
	var files []int64
	for n, at := range themeAt {
		pcm := make([]int16, 400*sec)
		rr := rand.New(rand.NewSource(int64(10 + n)))
		lp := 0.0
		for i := range pcm {
			lp = 0.9*lp + 0.1*(rr.Float64()*2-1)
			pcm[i] = int16(lp * 6000)
		}
		if at >= 0 {
			for i, v := range theme {
				pcm[at*sec+i] = v
			}
		}
		path := filepath.Join(dir, fmt.Sprintf("Show S01E%02d.wav", n+1))
		writeWAV(t, path, pcm)
		st, _ := os.Stat(path)
		ep, _ := d.EnsureEpisode(ctx, show, 1, n+1, "", "")
		dur := 400.0
		fid, err := d.UpsertFile(ctx, db.File{LibraryID: lib, MediaItemID: show, EpisodeID: &ep, Path: path, Size: st.Size(), Mtime: st.ModTime().Unix(), DurationSec: &dur, AudioCodec: "pcm_s16le"}, 1)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, fid)
	}
	w := &Worker{db: d, cfg: config.Config{FFmpeg: ff}, wake: make(chan struct{}, 1), wakeEnc: make(chan struct{}, 1)}
	if n, err := w.QueueIntros(ctx, lib); err != nil || n != 1 {
		t.Fatalf("queued %d series, %v", n, err)
	}
	if err := w.intros(ctx, 0, show); err != nil {
		t.Fatal(err)
	}
	for n, at := range themeAt {
		segs, err := d.FileSegments(ctx, files[n])
		if err != nil {
			t.Fatal(err)
		}
		if at < 0 {
			if len(segs) != 0 {
				t.Errorf("episode %d has no theme but got %+v", n+1, segs)
			}
			continue
		}
		if len(segs) != 1 || segs[0].Kind != "intro" || segs[0].Source != "detected" {
			t.Errorf("episode %d: %+v", n+1, segs)
			continue
		}
		if s := segs[0]; math.Abs(s.Start-float64(at)) > 2 || math.Abs(s.End-float64(at+30)) > 2 {
			t.Errorf("episode %d: intro %.1f–%.1f, want about %d–%d", n+1, s.Start, s.End, at, at+30)
		}
	}
	// All looked at: nothing more to queue, and an admin's mark isn't replaced.
	if n, _ := w.QueueIntros(ctx, lib); n != 0 {
		t.Errorf("queued %d series again", n)
	}
	if err := d.SetSegment(ctx, files[0], db.MarkedSegment{Kind: "intro", Start: 1, End: 31, Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	if err := d.SetSegment(ctx, files[0], db.MarkedSegment{Kind: "intro", Start: 40, End: 70, Source: "detected"}); err != nil {
		t.Fatal(err)
	}
	if segs, _ := d.FileSegments(ctx, files[0]); len(segs) != 1 || segs[0].Source != "manual" || segs[0].Start != 1 {
		t.Errorf("a detected intro replaced a manual one: %+v", segs)
	}
}
