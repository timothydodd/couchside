package livetv

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/timothydodd/couchside/internal/probe"
)

// A stream channel's source is looked at once before it's played. One that
// is already 8-bit H.264 with AAC or MP3 (what every browser and TV plays)
// is passed through untouched: no decoding, no second encode, and it doesn't
// count as a transcode. Anything else is converted like a tuner channel.
const streamProbeFor = 10 * time.Minute

type streamProbe struct {
	info *probe.Info // nil when the probe failed
	at   time.Time
}

var streamProbes sync.Map // url → streamProbe

// streamInfo probes a stream's codecs, remembering the answer for a while.
func (s *Service) streamInfo(ctx context.Context, url string) *probe.Info {
	if s.cfg.FFprobe == "" {
		return nil
	}
	if v, ok := streamProbes.Load(url); ok {
		if p := v.(streamProbe); time.Since(p.at) < streamProbeFor {
			return p.info
		}
	}
	pctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	info, err := probe.Probe(pctx, s.cfg.FFprobe, url)
	if err != nil {
		slog.Warn("stream channel: couldn't read the source's codecs; converting it", "err", err)
		info = nil
	}
	streamProbes.Store(url, streamProbe{info: info, at: time.Now()})
	return info
}

// streamSpec decides how to play a stream from what the probe found: copy
// what everything plays, convert the rest.
func streamSpec(info *probe.Info, o WatchOpts) Spec {
	sp := o.spec("", "")
	sp.CopyVideo, sp.CopyAudio = false, false
	if info == nil {
		return sp
	}
	if info.Height != nil {
		sp.SrcHeight = *info.Height
	}
	if info.VideoCodec == "h264" && info.EightBit420() && !info.HDR() && info.DVProfile == 0 {
		sp.CopyVideo, sp.VideoCodec = true, "h264"
	}
	if (info.AudioCodec == "aac" || info.AudioCodec == "mp3") && info.AudioChannels <= 2 {
		sp.CopyAudio = true
	}
	return sp
}
