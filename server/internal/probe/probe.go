// Package probe reads stream information from media files with ffprobe.
package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Info struct {
	DurationSec    *float64
	Container      string
	VideoCodec     string
	AudioCodec     string
	Width, Height  *int
	AudioTracks    int
	SubtitleTracks int
	PixFmt         string // e.g. yuv420p, yuv420p10le
	ColorTransfer  string // smpte2084 (HDR10) / arib-std-b67 (HLG) mean HDR
	AudioChannels  int
	// DVProfile is the Dolby Vision profile (5, 7, 8…), 0 when the video
	// isn't Dolby Vision. Profile 5 has no HDR10 or SDR layer underneath: a
	// player without Dolby Vision shows it green and purple.
	DVProfile int
}

// HDR reports whether the video uses a PQ or HLG transfer and needs tone mapping for SDR output.
func (i *Info) HDR() bool {
	return i.ColorTransfer == "smpte2084" || i.ColorTransfer == "arib-std-b67"
}

// DynamicRange names the video's range: "dv" (Dolby Vision, whatever is
// underneath), "hdr10" (PQ), "hlg", or "" for SDR.
func (i *Info) DynamicRange() string {
	switch {
	case i.DVProfile > 0:
		return "dv"
	case i.ColorTransfer == "smpte2084":
		return "hdr10"
	case i.ColorTransfer == "arib-std-b67":
		return "hlg"
	}
	return ""
}

// EightBit420 reports whether the video is plain 8-bit 4:2:0, the only H.264 flavour browsers decode.
func (i *Info) EightBit420() bool {
	return i.PixFmt == "yuv420p" || i.PixFmt == "yuvj420p"
}

type ffprobeOut struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
	Streams []struct {
		Duration      string            `json:"duration"`
		Tags          map[string]string `json:"tags"`
		CodecType     string            `json:"codec_type"`
		CodecName     string            `json:"codec_name"`
		Width         int               `json:"width"`
		Height        int               `json:"height"`
		PixFmt        string            `json:"pix_fmt"`
		ColorTransfer string            `json:"color_transfer"`
		Channels      int               `json:"channels"`
		SideData      []struct {
			DVProfile int `json:"dv_profile"`
		} `json:"side_data_list"`
		Disposition struct {
			AttachedPic int `json:"attached_pic"`
		} `json:"disposition"`
	} `json:"streams"`
}

// Timeout bounds one ffprobe run. Tests shorten it.
var Timeout = 60 * time.Second

func Probe(ctx context.Context, bin, path string) (*Info, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "-v", "error", "-print_format", "json",
		"-show_format", "-show_streams", path).Output()
	if err != nil {
		// Timed out (a slow share) or cancelled: say so, so callers can tell
		// that from a file ffprobe couldn't read.
		if ctx.Err() != nil {
			return nil, fmt.Errorf("ffprobe: %w", ctx.Err())
		}
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("ffprobe: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("ffprobe: %w", err)
	}
	var p ffprobeOut
	if err := json.Unmarshal(out, &p); err != nil {
		return nil, fmt.Errorf("ffprobe output: %w", err)
	}
	info := &Info{Container: strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")}
	if d, err := strconv.ParseFloat(p.Format.Duration, 64); err == nil && d > 0 {
		info.DurationSec = &d
	} else {
		// Some MKVs (streamed or piped muxes) have no container duration;
		// fall back to the longest stream duration or its DURATION tag.
		best := 0.0
		for _, st := range p.Streams {
			if d, err := strconv.ParseFloat(st.Duration, 64); err == nil && d > best {
				best = d
			}
			for k, v := range st.Tags {
				if strings.EqualFold(k, "DURATION") {
					if d := parseClock(v); d > best {
						best = d
					}
				}
			}
		}
		if best > 0 {
			info.DurationSec = &best
		}
	}
	for _, s := range p.Streams {
		switch s.CodecType {
		case "video":
			// Skip embedded cover art, which shows up as a video stream.
			if info.VideoCodec == "" && s.Disposition.AttachedPic == 0 && s.CodecName != "" && s.CodecName != "none" {
				w, h := s.Width, s.Height
				info.VideoCodec, info.Width, info.Height = s.CodecName, &w, &h
				info.PixFmt, info.ColorTransfer = s.PixFmt, s.ColorTransfer
				for _, sd := range s.SideData {
					if sd.DVProfile > 0 {
						info.DVProfile = sd.DVProfile
					}
				}
			}
		case "audio":
			if info.AudioCodec == "" {
				info.AudioCodec, info.AudioChannels = s.CodecName, s.Channels
			}
			info.AudioTracks++
		case "subtitle":
			info.SubtitleTracks++
		}
	}
	return info, nil
}

// parseClock reads "01:52:03.500000000" as seconds.
func parseClock(v string) float64 {
	parts := strings.Split(strings.TrimSpace(v), ":")
	if len(parts) != 3 {
		return 0
	}
	h, e1 := strconv.ParseFloat(parts[0], 64)
	m, e2 := strconv.ParseFloat(parts[1], 64)
	sec, e3 := strconv.ParseFloat(parts[2], 64)
	if e1 != nil || e2 != nil || e3 != nil {
		return 0
	}
	return h*3600 + m*60 + sec
}
