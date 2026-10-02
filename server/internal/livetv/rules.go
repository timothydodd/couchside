package livetv

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/metadata"
)

// RuleSummary explains what a rule evaluation did, for the UI.
type RuleSummary struct {
	Airings         int `json:"airings"`         // upcoming airings of the series in the guide
	Scheduled       int `json:"scheduled"`       // airings this rule will record
	Added           int `json:"added"`           // of those, newly scheduled in this run
	AlreadyHave     int `json:"alreadyHave"`     // episode is in the library
	AlreadyRecorded int `json:"alreadyRecorded"` // recorded, scheduled elsewhere, or recording now
	Duplicates      int `json:"duplicates"`      // same episode airs again later
	NotNew          int `json:"notNew"`          // reruns skipped in "new" mode (or when there's no episode info)
	NoEpisodeInfo   int `json:"noEpisodeInfo"`   // guide had no episode number or title
	OtherChannel    int `json:"otherChannel"`
	Conflicts       int `json:"conflicts"` // skipped because every tuner is booked
	Cancelled       int `json:"cancelled"` // airings you cancelled by hand
	Locked          int `json:"locked"`    // DRM channels
}

var reSxxExx = regexp.MustCompile(`^S(\d+)E(\d+)$`)

// programKey identifies an episode: "S11E4" (numbers normalized so "S11E04"
// in the guide matches season 11 episode 4 in the library), or the
// title plus episode title, or "" when the guide doesn't say.
func programKey(p db.Program) string {
	if m := reSxxExx.FindStringSubmatch(strings.ToUpper(p.EpisodeNum)); m != nil {
		s, _ := strconv.Atoi(m[1])
		e, _ := strconv.Atoi(m[2])
		return "S" + strconv.Itoa(s) + "E" + strconv.Itoa(e)
	}
	if p.EpisodeTitle != "" {
		return titleKey(p.Title, p.EpisodeTitle)
	}
	return ""
}

// titleKey compares show and episode titles ignoring case and punctuation.
func titleKey(show, episode string) string {
	return "t:" + metadata.Normalize(show) + "|" + metadata.Normalize(episode)
}

// normalizeKeys converts keys from the database to programKey form.
func normalizeKeys(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in))
	for k := range in {
		if rest, ok := strings.CutPrefix(k, "t:"); ok {
			show, ep, _ := strings.Cut(rest, "|")
			out[titleKey(show, ep)] = true
		} else if m := reSxxExx.FindStringSubmatch(k); m != nil {
			s, _ := strconv.Atoi(m[1])
			e, _ := strconv.Atoi(m[2])
			out["S"+strconv.Itoa(s)+"E"+strconv.Itoa(e)] = true
		} else {
			out[k] = true
		}
	}
	return out
}

// CreateRule makes (or replaces) the series rule for a guide program's show
// and evaluates it right away.
func (s *Service) CreateRule(ctx context.Context, programID int64, mode, channel string, mediaItemID *int64, keepLast int) (db.SeriesRule, RuleSummary, error) {
	p, err := s.db.Program(ctx, programID)
	if err != nil {
		return db.SeriesRule{}, RuleSummary{}, err
	}
	if p.SeriesID == "" {
		return db.SeriesRule{}, RuleSummary{}, errors.New("the guide doesn't identify this as a series, so it can only be recorded once")
	}
	if err := validMode(mode); err != nil {
		return db.SeriesRule{}, RuleSummary{}, err
	}
	if mediaItemID == nil {
		if c := s.LibraryMatchesFor(ctx, p); len(c) > 0 {
			mediaItemID = &c[0].ID
		}
	}
	id, err := s.db.UpsertRule(ctx, db.SeriesRule{SeriesID: p.SeriesID, Title: p.Title, ImageURL: p.ImageURL, Mode: mode,
		Channel: channel, MediaItemID: mediaItemID, KeepLast: max(0, keepLast)})
	if err != nil {
		return db.SeriesRule{}, RuleSummary{}, err
	}
	return s.reevaluate(ctx, id)
}

// UpdateRule changes a rule's options and re-evaluates it.
func (s *Service) UpdateRule(ctx context.Context, r db.SeriesRule) (db.SeriesRule, RuleSummary, error) {
	if err := validMode(r.Mode); err != nil {
		return db.SeriesRule{}, RuleSummary{}, err
	}
	if err := s.db.UpdateRule(ctx, r); err != nil {
		return db.SeriesRule{}, RuleSummary{}, err
	}
	if !r.Enabled {
		_ = s.db.UnscheduleStale(ctx, r.ID, nil)
		rule, err := s.db.Rule(ctx, r.ID)
		return rule, RuleSummary{}, err
	}
	return s.reevaluate(ctx, r.ID)
}

// DeleteRule removes a rule and its upcoming recordings; finished ones stay.
func (s *Service) DeleteRule(ctx context.Context, id int64) error {
	if err := s.db.UnscheduleStale(ctx, id, nil); err != nil {
		return err
	}
	return s.db.DeleteRule(ctx, id)
}

func (s *Service) reevaluate(ctx context.Context, id int64) (db.SeriesRule, RuleSummary, error) {
	rule, err := s.db.Rule(ctx, id)
	if err != nil {
		return rule, RuleSummary{}, err
	}
	sum, err := s.evaluateRule(ctx, rule)
	if err != nil {
		return rule, sum, err
	}
	rule, err = s.db.Rule(ctx, id)
	s.wake()
	return rule, sum, err
}

func validMode(m string) error {
	switch m {
	case "new", "missing", "all":
		return nil
	}
	return errors.New("mode must be new, missing or all")
}

// LibraryMatch is a library show a rule can be compared against.
type LibraryMatch struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Year      *int   `json:"year"`
	FileCount int    `json:"fileCount"`
	DVR       bool   `json:"dvr"`      // lives in the DVR Recordings library
	Matching  int    `json:"matching"` // upcoming airings whose episode title this show has
}

// LibraryMatchesFor ranks same-titled library shows by how many of the
// series' upcoming episode titles they contain, so a rule for the 2016
// "MacGyver" compares against the 2016 show, not the 1985 one.
func (s *Service) LibraryMatchesFor(ctx context.Context, p db.Program) []LibraryMatch {
	cands := s.LibraryMatches(ctx, p.Title)
	if len(cands) < 2 || p.SeriesID == "" {
		return cands
	}
	progs, err := s.db.UpcomingSeriesPrograms(ctx, p.SeriesID, time.Now().Add(-6*time.Hour).Unix())
	if err != nil {
		return cands
	}
	for i := range cands {
		keys, err := s.db.LibraryEpisodeKeys(ctx, cands[i].ID)
		if err != nil {
			continue
		}
		keys = normalizeKeys(keys)
		for _, up := range progs {
			if up.EpisodeTitle != "" && keys[titleKey(up.Title, up.EpisodeTitle)] {
				cands[i].Matching++
			}
		}
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].Matching > cands[j].Matching })
	return cands
}

// LibraryMatches finds library shows with the same (normalized) title, best
// guess first: shows outside the DVR library, then newest.
func (s *Service) LibraryMatches(ctx context.Context, title string) []LibraryMatch {
	// Never nil: the "Record series" form reads candidates.length.
	out := []LibraryMatch{}
	all, err := s.db.SeriesByTitle(ctx)
	if err != nil {
		return out
	}
	s.mu.Lock()
	dvrLib := s.libraryID
	s.mu.Unlock()
	want := metadata.Normalize(title)
	for _, it := range all {
		if metadata.Normalize(it.Title) != want {
			continue
		}
		full, err := s.db.Item(ctx, it.ID)
		if err != nil {
			continue
		}
		out = append(out, LibraryMatch{ID: it.ID, Title: it.Title, Year: it.Year, FileCount: it.FileCount, DVR: full.LibraryID == dvrLib})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DVR != out[j].DVR {
			return !out[i].DVR
		}
		yi, yj := 0, 0
		if out[i].Year != nil {
			yi = *out[i].Year
		}
		if out[j].Year != nil {
			yj = *out[j].Year
		}
		return yi > yj
	})
	return out
}

// evaluateRule schedules the airings a rule wants and drops ones it no longer does.
func (s *Service) evaluateRule(ctx context.Context, rule db.SeriesRule) (RuleSummary, error) {
	var sum RuleSummary
	if !rule.Enabled {
		return sum, nil
	}
	now := time.Now().Unix()
	progs, err := s.db.UpcomingSeriesPrograms(ctx, rule.SeriesID, now)
	if err != nil {
		return sum, err
	}
	sum.Airings = len(progs)
	chans, err := s.db.Channels(ctx)
	if err != nil {
		return sum, err
	}
	byNum := map[string]db.Channel{}
	for _, c := range chans {
		byNum[c.Number] = c
	}
	have := map[string]bool{}
	if rule.MediaItemID != nil && rule.Mode == "missing" {
		if have, err = s.db.LibraryEpisodeKeys(ctx, *rule.MediaItemID); err != nil {
			return sum, err
		}
		have = normalizeKeys(have)
	}
	recorded, err := s.db.RecordedEpisodes(ctx, rule.SeriesID)
	if err != nil {
		return sum, err
	}
	recorded = normalizeKeys(recorded)
	tuners := 0
	if d := s.currentDevice(); d != nil {
		tuners = d.TunerCount
	}

	seen := map[string]bool{}
	var keep []int64
	for _, p := range progs {
		ch, ok := byNum[p.Channel]
		if !ok {
			continue
		}
		if ch.DRM {
			sum.Locked++
			continue
		}
		if rule.Channel != "" && p.Channel != rule.Channel {
			sum.OtherChannel++
			continue
		}
		key := programKey(p)
		existing, err := s.db.RecordingAt(ctx, p.Channel, p.StartAt)
		if err != nil {
			return sum, err
		}
		own := existing != nil && existing.RuleID != nil && *existing.RuleID == rule.ID && existing.Status == "scheduled"
		if existing != nil && !own {
			switch existing.Status {
			case "cancelled":
				sum.Cancelled++
			case "failed":
				// a failed attempt at this airing: let the rule try again below
				existing = nil
			default:
				sum.AlreadyRecorded++
				if key != "" {
					seen[key] = true
				}
			}
			if existing != nil {
				continue
			}
		}

		want := true
		switch rule.Mode {
		case "new":
			switch {
			case !p.IsNew:
				sum.NotNew++
				want = false
			case key != "" && !own && recorded[key]:
				// Still flagged new on a later airing, but already recorded.
				sum.AlreadyRecorded++
				want = false
			}
		case "missing":
			switch {
			case key == "":
				// No episode info: only take first airings rather than every rerun.
				if !p.IsNew {
					sum.NoEpisodeInfo++
					want = false
				}
			case have[key]:
				sum.AlreadyHave++
				want = false
			case !own && recorded[key]:
				sum.AlreadyRecorded++
				want = false
			}
		}
		if want && rule.Mode != "all" && key != "" && seen[key] {
			sum.Duplicates++
			want = false
		}
		if !want {
			continue
		}
		if own {
			keep = append(keep, existing.ID)
			seen[key] = true
			sum.Scheduled++
			continue
		}
		padB, padA := s.Padding(ctx)
		if tuners > 0 && s.peakBusy(ctx, p.StartAt-padB, p.EndAt+padA) >= tuners {
			sum.Conflicts++ // a later airing of the same episode may still fit
			continue
		}
		added, err := s.db.ScheduleRuleRecording(ctx, db.Recording{Channel: p.Channel, ChannelName: ch.Name, Title: p.Title,
			EpisodeTitle: p.EpisodeTitle, EpisodeNum: p.EpisodeNum, Synopsis: p.Synopsis, ImageURL: p.ImageURL,
			SeriesID: p.SeriesID, Categories: p.Categories, StartAt: p.StartAt, EndAt: p.EndAt, PadBefore: padB, PadAfter: padA}, rule.ID)
		if err != nil {
			return sum, err
		}
		if r, _ := s.db.RecordingAt(ctx, p.Channel, p.StartAt); r != nil {
			keep = append(keep, r.ID)
			s.prefetchShow(*r)
		}
		if key != "" {
			seen[key] = true
		}
		sum.Scheduled++
		if added {
			sum.Added++
		}
	}
	if err := s.db.UnscheduleStale(ctx, rule.ID, keep); err != nil {
		return sum, err
	}
	b, _ := json.Marshal(sum)
	_ = s.db.SetRuleSummary(ctx, rule.ID, string(b))
	if sum.Added > 0 {
		slog.Info("series rule scheduled recordings", "title", rule.Title, "mode", rule.Mode, "added", sum.Added, "scheduled", sum.Scheduled)
	}
	return sum, nil
}

// applyRules evaluates every enabled rule (after guide refreshes and hourly).
func (s *Service) applyRules(ctx context.Context) {
	rules, err := s.db.Rules(ctx)
	if err != nil {
		slog.Error("series rules", "err", err)
		return
	}
	for _, r := range rules {
		if _, err := s.evaluateRule(ctx, r); err != nil {
			slog.Warn("series rule failed", "title", r.Title, "err", err)
		}
	}
	s.wake()
}

// afterRecording enforces keep-last-N for the rule that made a recording.
// newestEpisodesFirst orders recordings by season and episode, newest first,
// when every one has them, so a re-recorded rerun of an old episode doesn't
// push out a newer one. Otherwise they stay in recording order.
func newestEpisodesFirst(recs []db.Recording) {
	type se struct{ s, e int }
	keys := make([]se, len(recs))
	for i, r := range recs {
		m := reEpisode.FindStringSubmatch(r.EpisodeNum)
		if m == nil {
			return
		}
		keys[i].s, _ = strconv.Atoi(m[1])
		keys[i].e, _ = strconv.Atoi(m[2])
	}
	idx := make([]int, len(recs))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		ka, kb := keys[idx[a]], keys[idx[b]]
		if ka.s != kb.s {
			return ka.s > kb.s
		}
		return ka.e > kb.e
	})
	sorted := make([]db.Recording, len(recs))
	for i, j := range idx {
		sorted[i] = recs[j]
	}
	copy(recs, sorted)
}

// peakBusy is the most recordings in progress at any one moment of [from,
// to): two that don't overlap each other share a tuner.
func (s *Service) peakBusy(ctx context.Context, from, to int64) int {
	wins, err := s.db.ActiveWindows(ctx, from, to)
	if err != nil {
		return 0
	}
	return peakOverlap(wins, from, to)
}

func peakOverlap(wins [][2]int64, from, to int64) int {
	type ev struct {
		at    int64
		delta int
	}
	var evs []ev
	for _, w := range wins {
		evs = append(evs, ev{max(w[0], from), 1}, ev{min(w[1], to), -1})
	}
	// Ends before starts at the same instant: back-to-back recordings share a tuner.
	sort.Slice(evs, func(i, j int) bool {
		if evs[i].at != evs[j].at {
			return evs[i].at < evs[j].at
		}
		return evs[i].delta < evs[j].delta
	})
	n, peak := 0, 0
	for _, e := range evs {
		n += e.delta
		peak = max(peak, n)
	}
	return peak
}

func (s *Service) afterRecording(ctx context.Context, rec db.Recording) {
	if rec.RuleID == nil {
		return
	}
	rule, err := s.db.Rule(ctx, *rec.RuleID)
	if err != nil || rule.KeepLast <= 0 {
		return
	}
	done, err := s.db.RuleRecordingsNewestFirst(ctx, rule.ID)
	if err != nil {
		return
	}
	newestEpisodesFirst(done)
	for i, r := range done {
		if i < rule.KeepLast {
			continue
		}
		if err := s.Delete(ctx, r.ID); err != nil {
			slog.Warn("keep-last cleanup failed", "title", r.Title, "err", err)
		} else {
			slog.Info("removed old recording (keep last N)", "title", r.Title, "episode", r.EpisodeNum, "keep", rule.KeepLast)
		}
	}
}
