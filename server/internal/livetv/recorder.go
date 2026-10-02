package livetv

import (
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
	"time"

	"github.com/timothydodd/couchside/internal/db"
)

// WatchOpts is what a client asks for: a height for transcoding, and the
// codecs it can decode itself (names as NormalizeCodec gives them), so a
// broadcast it can play is passed through instead of re-encoded.
type WatchOpts struct {
	Height      int
	VideoCodecs []string
	AudioCodecs []string
}

// NormalizeCodec maps the HDHomeRun's codec names ("MPEG2", "H264", "AC3")
// and clients' names to one spelling: mpeg2, h264, hevc, ac3, eac3, ac4, aac, mp3.
func NormalizeCodec(c string) string {
	c = strings.ToLower(strings.TrimSpace(c))
	switch c {
	case "mpeg2", "mpeg2video", "mpeg-2":
		return "mpeg2"
	case "h264", "h.264", "avc", "mpeg4 avc":
		return "h264"
	case "hevc", "h265", "h.265":
		return "hevc"
	case "eac3", "e-ac3", "ec-3":
		return "eac3"
	case "ac-3", "ac3":
		return "ac3"
	}
	return c
}

func hasCodec(list []string, codec string) bool {
	if codec == "" {
		return false
	}
	for _, c := range list {
		if NormalizeCodec(c) == codec {
			return true
		}
	}
	return false
}

// spec decides, for a channel's codecs, what to pass through.
func (o WatchOpts) spec(videoCodec, audioCodec string) Spec {
	v, a := NormalizeCodec(videoCodec), NormalizeCodec(audioCodec)
	sp := Spec{Height: o.Height, VideoCodec: v, CopyVideo: hasCodec(o.VideoCodecs, v), CopyAudio: hasCodec(o.AudioCodecs, a)}
	if sp.Height <= 0 {
		sp.Height = 720
	}
	return sp
}

// Watch starts (or joins) a live stream of a channel.
func (s *Service) Watch(ctx context.Context, channel string, o WatchOpts) (*LiveSession, error) {
	ch, err := s.db.Channel(ctx, channel)
	if err != nil {
		return nil, err
	}
	if ch.DRM {
		return nil, errors.New("this channel is copy-protected (ATSC 3.0 DRM) and can only be watched in SiliconDust's own apps")
	}
	return s.live.start(ctx, ch.Number, ch.Name, ch.URL, o.spec(ch.VideoCodec, ch.AudioCodec))
}

// WatchRecording plays a recording that's still in progress from its start.
func (s *Service) WatchRecording(ctx context.Context, id int64, o WatchOpts) (*LiveSession, error) {
	r, err := s.db.Recording(ctx, id)
	if err != nil {
		return nil, err
	}
	if r.Status != "recording" {
		return nil, errors.New("this recording isn't in progress; play it from the library")
	}
	parts := existingParts(r.Path)
	if len(parts) == 0 {
		return nil, errors.New("nothing has been recorded yet; try again in a few seconds")
	}
	// A recording holds the channel's broadcast as is, so the same codecs apply.
	var vc, ac string
	if ch, err := s.db.Channel(ctx, r.Channel); err == nil {
		vc, ac = ch.VideoCodec, ch.AudioCodec
	}
	// Follow the part being written. After a dropped signal that's only the
	// latest piece; the rest joins up once the recording finishes.
	return s.live.startRecordingPlayback(ctx, r.ID, r.Channel, r.Title, parts[len(parts)-1], o.spec(vc, ac))
}

func (s *Service) LiveFile(id, name string) (string, error) { return s.live.file(id, name) }
func (s *Service) LeaveLive(id string)                      { s.live.leave(id) }
func (s *Service) LiveSessions() []*LiveSession             { return s.live.sessionsList() }

// Record schedules a guide program. Returns the recording id and how many
// other recordings overlap it (more than the tuner count means a conflict).
func (s *Service) Record(ctx context.Context, programID int64) (int64, int, error) {
	p, err := s.db.Program(ctx, programID)
	if err != nil {
		return 0, 0, err
	}
	if p.EndAt <= time.Now().Unix() {
		return 0, 0, errors.New("that program has already ended")
	}
	ch, err := s.db.Channel(ctx, p.Channel)
	if err != nil {
		return 0, 0, err
	}
	if ch.DRM {
		return 0, 0, errors.New("this channel is copy-protected (ATSC 3.0 DRM) and can't be recorded")
	}
	r := db.Recording{Channel: p.Channel, ChannelName: ch.Name, Title: p.Title, EpisodeTitle: p.EpisodeTitle,
		EpisodeNum: p.EpisodeNum, Synopsis: p.Synopsis, ImageURL: p.ImageURL, SeriesID: p.SeriesID,
		Categories: p.Categories, StartAt: p.StartAt, EndAt: p.EndAt}
	r.PadBefore, r.PadAfter = s.Padding(ctx)
	id, err := s.db.ScheduleRecording(ctx, r)
	if err != nil {
		return 0, 0, err
	}
	s.prefetchShow(r)
	overlap, _ := s.db.Overlapping(ctx, r.StartAt-r.PadBefore, r.EndAt+r.PadAfter, id)
	s.wake()
	return id, overlap, nil
}

// Cancel stops a running recording (keeping what was recorded) or drops a scheduled one.
func (s *Service) Cancel(ctx context.Context, id int64) error {
	s.mu.Lock()
	cancel, running := s.recCancel[id]
	s.mu.Unlock()
	if running {
		cancel()
		return nil
	}
	return s.db.CancelRecording(ctx, id)
}

// Delete removes a finished recording and its file.
func (s *Service) Delete(ctx context.Context, id int64) error {
	r, err := s.db.Recording(ctx, id)
	if err != nil {
		return err
	}
	if r.Status == "recording" || r.Status == "scheduled" {
		return errors.New("cancel the recording first")
	}
	// Only ever delete files Couchside recorded: the row's own path, and only
	// if it's a .ts file (recording folders may be shared with a TV library).
	if r.Path != "" && strings.HasSuffix(r.Path, ".ts") {
		if err := os.Remove(r.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if lib, _ := s.db.LibraryContaining(ctx, r.Path); lib != nil {
			removeEmptyParents(filepath.Dir(r.Path), lib.Path)
			if s.work != nil {
				_ = s.work.Enqueue(ctx, "scan", lib.ID, "Scan "+lib.Name)
			}
		}
	}
	return s.db.DeleteRecording(ctx, id)
}

func (s *Service) wake() {
	select {
	case s.wakeSched <- struct{}{}:
	default:
	}
}

// scheduler starts due recordings.
func (s *Service) scheduler(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		due, err := s.db.DueRecordings(ctx, time.Now().Unix())
		if err != nil && ctx.Err() == nil {
			slog.Error("dvr: list due recordings", "err", err)
		}
		for _, r := range due {
			if r.EndAt+r.PadAfter <= time.Now().Unix() {
				_ = s.db.FinishRecording(ctx, r.ID, "failed", "", 0, "Missed: the server wasn't running when this aired")
				continue
			}
			s.startRecording(ctx, r, "", nil)
		}
		select {
		case <-ctx.Done():
			return
		case <-s.wakeSched:
		case <-t.C:
		}
	}
}

// startRecording begins a recording, or resumes one after a restart when
// path and the parts captured so far are given.
func (s *Service) startRecording(parent context.Context, r db.Recording, path string, parts []string) {
	if path == "" {
		path = s.recordingPath(parent, r)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			_ = s.db.FinishRecording(parent, r.ID, "failed", "", 0, "Recordings folder isn't writable: "+err.Error())
			return
		}
		if err := s.db.MarkRecording(parent, r.ID, path); err != nil {
			slog.Error("dvr: mark recording", "err", err)
			return
		}
	}
	ctx, cancel := context.WithCancel(parent)
	s.mu.Lock()
	s.recCancel[r.ID] = cancel
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.recCancel, r.ID)
			s.mu.Unlock()
			cancel()
		}()
		s.record(ctx, parent, r, path, parts)
	}()
}

// record captures the tuner stream (stream copy, no transcoding) until the
// padded end time. A dropped stream or busy tuner is retried in new parts,
// which are joined afterwards.
func (s *Service) record(ctx, parent context.Context, r db.Recording, path string, parts []string) {
	end := time.Unix(r.EndAt+r.PadAfter, 0)
	slog.Info("dvr: recording", "title", r.Title, "channel", r.Channel, "until", end.Format(time.Kitchen), "path", path, "resumedParts", len(parts))
	var lastErr string
	for attempt := len(parts); time.Until(end) > 5*time.Second && ctx.Err() == nil; attempt++ {
		ch, err := s.db.Channel(parent, r.Channel)
		if err != nil {
			lastErr = "channel is no longer in the lineup"
			break
		}
		part := fmt.Sprintf("%s.part%d.ts", strings.TrimSuffix(path, ".ts"), attempt)
		dur := time.Until(end).Seconds()
		cmd := exec.CommandContext(ctx, s.cfg.FFmpeg, "-hide_banner", "-nostdin", "-loglevel", "error", "-y",
			"-rw_timeout", "15000000", "-i", ch.URL, "-map", "0:v", "-map", "0:a", "-dn", "-sn", "-c", "copy",
			"-t", fmt.Sprintf("%.0f", dur), "-f", "mpegts", part)
		out, err := cmd.CombinedOutput()
		if st, e := os.Stat(part); e == nil && st.Size() > 0 {
			parts = append(parts, part)
		} else {
			_ = os.Remove(part)
		}
		if err == nil || ctx.Err() != nil {
			continue
		}
		msg := lastLine(string(out))
		if strings.Contains(msg, "503") {
			msg = "all tuners were busy"
		}
		lastErr = msg
		_ = s.db.SetRecordingError(parent, r.ID, "Retrying: "+msg)
		slog.Warn("dvr: recording interrupted, retrying", "title", r.Title, "err", msg)
		select {
		case <-ctx.Done():
		case <-time.After(10 * time.Second):
		}
	}

	if parent.Err() != nil {
		// The server is shutting down mid-recording. Leave the parts and the
		// "recording" status alone: recoverInterrupted resumes into the same
		// file on the next start and joins everything at the end.
		slog.Info("dvr: server stopping, recording will resume on restart", "title", r.Title, "parts", len(parts))
		return
	}
	stoppedEarly := ctx.Err() != nil
	if len(parts) == 0 {
		msg := "Nothing was recorded"
		if lastErr != "" {
			msg += ": " + lastErr
		}
		_ = s.db.FinishRecording(parent, r.ID, "failed", "", 0, msg)
		return
	}
	if err := joinParts(parent, s.cfg.FFmpeg, parts, path); err != nil {
		_ = s.db.FinishRecording(parent, r.ID, "failed", parts[0], 0, "Couldn't finish the file: "+err.Error())
		return
	}
	size := int64(0)
	if st, err := os.Stat(path); err == nil {
		size = st.Size()
	}
	note := ""
	switch {
	case stoppedEarly:
		note = "Stopped early"
	case len(parts) > 1:
		note = fmt.Sprintf("The signal dropped %d time(s); there may be gaps", len(parts)-1)
	}
	_ = s.db.FinishRecording(parent, r.ID, "completed", path, size, note)
	slog.Info("dvr: recording finished", "title", r.Title, "size", size, "parts", len(parts))
	s.scanRecordings(parent)
	s.afterRecording(parent, r)
}

// joinParts renames a single part into place, or concatenates several
// (MPEG-TS concatenates cleanly with the concat protocol).
func joinParts(ctx context.Context, ffmpeg string, parts []string, dst string) error {
	if len(parts) == 1 {
		return os.Rename(parts[0], dst)
	}
	tmp := strings.TrimSuffix(dst, ".ts") + ".joining.ts"
	out, err := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-nostdin", "-loglevel", "error", "-y",
		"-i", "concat:"+strings.Join(parts, "|"), "-map", "0", "-c", "copy", "-f", "mpegts", tmp).CombinedOutput()
	if err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("%v: %s", err, lastLine(string(out)))
	}
	for _, p := range parts {
		_ = os.Remove(p)
	}
	return os.Rename(tmp, dst)
}

// recoverInterrupted handles recordings cut off by a restart. Parts captured
// so far are kept; if the program is still airing, recording resumes and all
// parts are joined into the one file at the end, otherwise they're joined now.
func (s *Service) recoverInterrupted(ctx context.Context) {
	recs, err := s.db.RecordingsWithStatus(ctx, "recording")
	if err != nil {
		return
	}
	for _, r := range recs {
		parts := existingParts(r.Path)
		if r.EndAt+r.PadAfter > time.Now().Unix()+5 {
			slog.Info("dvr: resuming recording after restart", "title", r.Title, "parts", len(parts))
			s.startRecording(ctx, r, r.Path, parts)
			continue
		}
		if len(parts) == 0 {
			_ = s.db.FinishRecording(ctx, r.ID, "failed", "", 0, "Interrupted by a server restart")
			continue
		}
		if err := joinParts(ctx, s.cfg.FFmpeg, parts, r.Path); err != nil {
			_ = s.db.FinishRecording(ctx, r.ID, "failed", "", 0, "Interrupted by a server restart")
			continue
		}
		size := int64(0)
		if st, err := os.Stat(r.Path); err == nil {
			size = st.Size()
		}
		_ = s.db.FinishRecording(ctx, r.ID, "completed", r.Path, size, "Interrupted by a server restart; the end is missing")
	}
	s.scanRecordings(ctx)
}

// existingParts lists what a recording has captured so far, in order: a
// finished file at the final path (older builds finalized on shutdown), then
// "<name>.partN.ts" pieces sorted by N.
func existingParts(path string) []string {
	if path == "" {
		return nil
	}
	var out []string
	if st, err := os.Stat(path); err == nil && st.Size() > 0 {
		out = append(out, path)
	}
	base := strings.TrimSuffix(path, ".ts")
	matches, _ := filepath.Glob(base + ".part*.ts")
	sort.Slice(matches, func(i, j int) bool { return partIndex(matches[i], base) < partIndex(matches[j], base) })
	return append(out, matches...)
}

func partIndex(p, base string) int {
	n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(p, base+".part"), ".ts"))
	if err != nil {
		return 1 << 30
	}
	return n
}

var (
	reUnsafe  = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)
	reEpisode = regexp.MustCompile(`^S(\d+)E(\d+)$`)
)

// recordingName names a recording the way Plex does, so the scanner files
// it under the right show: "Show/Season 25/Show - S25E12 - Title", or by air
// date when the guide has no episode number. year (0 = unknown) is the
// series' premiere year, added when another series shares the title:
// "MacGyver (2016)/Season 2/MacGyver (2016) - S02E05 - Title".
type recName struct {
	show, season, file string
	title              string // series title without the year
	year               int
}

func recordingName(r db.Recording, year int) recName {
	title := safeName(r.Title)
	show := title
	if year > 0 {
		show = fmt.Sprintf("%s (%d)", title, year)
	}
	start := time.Unix(r.StartAt, 0).Local()
	var season, file string
	if m := reEpisode.FindStringSubmatch(r.EpisodeNum); m != nil {
		season = "Season " + strings.TrimLeft(m[1], "0")
		file = fmt.Sprintf("%s - %s", show, r.EpisodeNum)
	} else {
		season = fmt.Sprintf("Season %d", start.Year())
		file = fmt.Sprintf("%s - %s", show, start.Format("2006-01-02 15 04 05"))
	}
	if r.EpisodeTitle != "" {
		file += " - " + safeName(r.EpisodeTitle)
	}
	if len(file) > 180 {
		file = file[:180]
	}
	return recName{show: show, season: season, file: file, title: title, year: year}
}

// recordingPath is where a new recording goes: the current recordings folder,
// inside an existing matching show/season folder when there is one.
func (s *Service) recordingPath(ctx context.Context, r db.Recording) string {
	return s.pathInDir(s.RecordingsDir(ctx), recordingName(r, s.showYear(ctx, r, true)))
}

func safeName(s string) string {
	s = reUnsafe.ReplaceAllString(s, " ")
	s = strings.Join(strings.Fields(s), " ")
	s = strings.Trim(s, ". ")
	if s == "" {
		return "Untitled"
	}
	return s
}

func removeEmptyParents(dir, root string) {
	root = filepath.Clean(root)
	for d := filepath.Clean(dir); d != root && strings.HasPrefix(d, root); d = filepath.Dir(d) {
		if err := os.Remove(d); err != nil {
			return
		}
	}
}
