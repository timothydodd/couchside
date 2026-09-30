package livetv

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/metadata"
	"github.com/timothydodd/couchside/internal/transcode"
)

// Enqueuer lets the service ask the worker to scan the recordings library.
type Enqueuer interface {
	Enqueue(ctx context.Context, kind string, ref int64, label string) error
}

type Config struct {
	Tuner         string // HDHomeRun address
	RecordingsDir string
	FFmpeg        string
	PadBefore     time.Duration
	PadAfter      time.Duration
	Metadata      *metadata.Chain // identifies which same-titled series a recording is; may be nil
}

// Service owns the tuner, guide, live sessions and the DVR scheduler.
type Service struct {
	cfg  Config
	db   *db.DB
	hdhr *HDHomeRun
	enc  transcode.Encoder
	work Enqueuer
	live *liveManager

	mu        sync.Mutex
	device    *Device
	deviceErr error
	guideErr  error
	guideAt   time.Time
	recCancel map[int64]context.CancelFunc
	libraryID int64
	wakeSched chan struct{}
	lookups   map[string]bool // guide series being identified in the background
}

func New(cfg Config, d *db.DB, enc transcode.Encoder, work Enqueuer, cacheDir string) (*Service, error) {
	lm, err := newLiveManager(enc, filepath.Join(cacheDir, "live"))
	if err != nil {
		return nil, err
	}
	return &Service{cfg: cfg, db: d, hdhr: NewHDHomeRun(cfg.Tuner), enc: enc, work: work, live: lm,
		recCancel: map[int64]context.CancelFunc{}, wakeSched: make(chan struct{}, 1), lookups: map[string]bool{}}, nil
}

// Status is the Live TV summary for the UI.
type Status struct {
	Configured    bool     `json:"configured"`
	Device        *Device  `json:"device"`
	Error         string   `json:"error"`
	GuideError    string   `json:"guideError"`
	GuideThrough  int64    `json:"guideThrough"`
	GuideUpdated  int64    `json:"guideUpdated"`
	TunersInUse   int      `json:"tunersInUse"`
	TunerUsers    []string `json:"tunerUsers"`
	Recording     int      `json:"recording"`
	LiveSessions  int      `json:"liveSessions"`
	RecordingsDir string   `json:"recordingsDir"`
	LibraryID     int64    `json:"libraryId"`
}

func (s *Service) Status(ctx context.Context) Status {
	s.mu.Lock()
	st := Status{Configured: true, LibraryID: s.libraryID}
	if s.device != nil {
		d := *s.device
		d.DeviceAuth = "" // never leaves the server
		st.Device = &d
	}
	if s.deviceErr != nil {
		st.Error = s.deviceErr.Error()
	}
	if s.guideErr != nil {
		st.GuideError = s.guideErr.Error()
	}
	if !s.guideAt.IsZero() {
		st.GuideUpdated = s.guideAt.Unix()
	}
	st.Recording = len(s.recCancel)
	s.mu.Unlock()
	st.RecordingsDir = s.RecordingsDir(ctx)
	st.GuideThrough, _ = s.db.GuideCoverage(ctx)
	st.LiveSessions = s.live.count()
	sctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if tuners, err := s.hdhr.Status(sctx); err == nil {
		for _, t := range tuners {
			if t.VctNumber != "" {
				st.TunersInUse++
				st.TunerUsers = append(st.TunerUsers, t.VctNumber+" "+t.VctName+" → "+t.TargetIP)
			}
		}
	}
	return st
}

// Summary is the cheap subset of Status for frequent polling (no tuner call).
func (s *Service) Summary() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]any{"configured": true, "recording": len(s.recCancel), "liveSessions": s.live.count(),
		"online": s.device != nil && s.deviceErr == nil}
}

// Run refreshes the lineup and guide, runs the DVR scheduler and reaps idle
// live streams until ctx ends.
func (s *Service) Run(ctx context.Context) {
	if err := CheckWritable(s.RecordingsDir(ctx)); err != nil {
		slog.Error("recordings folder is not writable; recording will fail", "err", err)
	}
	s.ensureLibrary(ctx)
	s.recoverInterrupted(ctx)

	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); s.refreshLoop(ctx) }()
	go func() { defer wg.Done(); s.scheduler(ctx) }()
	go func() { defer wg.Done(); s.live.run(ctx) }()
	wg.Wait()
}

func (s *Service) refreshLoop(ctx context.Context) {
	lineupEvery, guideEvery := time.Hour, 3*time.Hour
	var lastLineup, lastGuide, lastRules time.Time
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		if time.Since(lastLineup) > lineupEvery || s.currentDevice() == nil {
			if err := s.refreshLineup(ctx); err != nil {
				slog.Warn("hdhomerun lineup refresh failed", "err", err)
			} else {
				lastLineup = time.Now()
			}
		}
		if s.currentDevice() != nil && time.Since(lastGuide) > guideEvery {
			if err := s.refreshGuide(ctx); err != nil {
				slog.Warn("guide refresh failed", "err", err)
				lastGuide = time.Now().Add(-guideEvery + 10*time.Minute) // retry in 10 minutes
			} else {
				lastGuide = time.Now()
				s.applyRules(ctx)
				lastRules = time.Now()
			}
		}
		// Re-check hourly too: the library changes (new files, deletions) between guide refreshes.
		if s.currentDevice() != nil && time.Since(lastRules) > time.Hour {
			s.applyRules(ctx)
			lastRules = time.Now()
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Service) currentDevice() *Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.device
}

func (s *Service) refreshLineup(ctx context.Context) error {
	d, err := s.hdhr.Discover(ctx)
	s.mu.Lock()
	s.deviceErr = err
	if err == nil {
		s.device = d
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	lineup, err := s.hdhr.Lineup(ctx)
	if err != nil {
		return err
	}
	chans := make([]db.Channel, 0, len(lineup))
	for _, l := range lineup {
		chans = append(chans, db.Channel{Number: l.GuideNumber, Name: l.GuideName, URL: l.URL, HD: l.HD == 1,
			DRM: l.DRM == 1, VideoCodec: l.VideoCodec, AudioCodec: l.AudioCodec,
			SignalStrength: l.SignalStrength, SignalQuality: l.SignalQuality})
	}
	if err := s.db.ReplaceChannels(ctx, chans, channelSortKey); err != nil {
		return err
	}
	slog.Info("hdhomerun lineup", "device", d.FriendlyName, "tuners", d.TunerCount, "channels", len(chans))
	return nil
}

// refreshGuide pulls listings in ~4 hour pages until it has a day ahead or
// the service stops returning data.
func (s *Service) refreshGuide(ctx context.Context) error {
	d, err := s.hdhr.Discover(ctx) // DeviceAuth rotates
	if err != nil {
		return err
	}
	horizon := time.Now().Add(26 * time.Hour).Unix()
	var start, total int64
	for page := 0; page < 10; page++ {
		chans, err := fetchGuide(ctx, d.DeviceAuth, start)
		if err != nil {
			s.setGuideErr(err)
			return err
		}
		var progs []db.Program
		var maxEnd int64
		for _, c := range chans {
			if page == 0 {
				_ = s.db.SetChannelGuideInfo(ctx, c.GuideNumber, c.Affiliate, c.ImageURL)
			}
			for _, g := range c.Guide {
				p := db.Program{Channel: c.GuideNumber, StartAt: g.StartTime, EndAt: g.EndTime, Title: g.Title,
					EpisodeTitle: g.EpisodeTitle, EpisodeNum: g.EpisodeNumber, Synopsis: g.Synopsis, ImageURL: g.ImageURL,
					SeriesID: g.SeriesID, IsNew: g.First == 1, Categories: g.Filter}
				if g.OriginalAirdate > 0 {
					oa := g.OriginalAirdate
					p.OriginalAirdate = &oa
				}
				progs = append(progs, p)
				maxEnd = max(maxEnd, g.EndTime)
			}
		}
		if len(progs) == 0 {
			break
		}
		if err := s.db.UpsertPrograms(ctx, progs); err != nil {
			return err
		}
		total += int64(len(progs))
		if maxEnd >= horizon || maxEnd <= start {
			break
		}
		start = maxEnd
	}
	_ = s.db.PrunePrograms(ctx, time.Now().Add(-6*time.Hour).Unix())
	s.mu.Lock()
	s.guideErr, s.guideAt = nil, time.Now()
	s.mu.Unlock()
	through, _ := s.db.GuideCoverage(ctx)
	slog.Info("guide refreshed", "programs", total, "through", time.Unix(through, 0).Format(time.RFC3339))
	return nil
}

func (s *Service) setGuideErr(err error) {
	s.mu.Lock()
	s.guideErr = err
	s.mu.Unlock()
}

// RefreshNow re-reads lineup and guide on demand (Settings → Refresh).
func (s *Service) RefreshNow(ctx context.Context) error {
	if err := s.refreshLineup(ctx); err != nil {
		return err
	}
	if err := s.refreshGuide(ctx); err != nil {
		return err
	}
	s.applyRules(ctx)
	return nil
}

// ensureLibrary makes sure recordings show up in a TV library: the library
// that already contains the recordings folder (e.g. TV Shows), or a new
// "DVR Recordings" library for it.
func (s *Service) ensureLibrary(ctx context.Context) {
	dir := s.RecordingsDir(ctx)
	lib, err := s.db.LibraryContaining(ctx, dir)
	if err != nil {
		slog.Error("find recordings library", "err", err)
		return
	}
	id := int64(0)
	if lib != nil {
		id = lib.ID
	} else if id, err = s.db.CreateLibrary(ctx, "DVR Recordings", dir, "tv"); err != nil {
		slog.Error("create recordings library", "err", err)
		return
	}
	s.mu.Lock()
	s.libraryID = id
	s.mu.Unlock()
}

func (s *Service) scanRecordings(ctx context.Context) {
	s.mu.Lock()
	id := s.libraryID
	s.mu.Unlock()
	if id != 0 && s.work != nil {
		name := "recordings"
		if l, err := s.db.Library(ctx, id); err == nil {
			name = l.Name
		}
		_ = s.work.Enqueue(ctx, "scan", id, "Scan "+name)
	}
}

var ErrNoTuner = errors.New("all tuners are busy")
