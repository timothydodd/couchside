// Package transcode turns files browsers can't play (or can't stream fast
// enough) into H.264/AAC: live HLS sessions for the player, and whole-file
// encodes for background "optimize" jobs.
package transcode

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Encoder builds ffmpeg video arguments for the configured hardware.
type Encoder struct {
	FFmpeg      string
	HW          string // none | vaapi | qsv | nvenc, after detection
	VAAPIDevice string
	Tonemap     bool // zscale available for HDR → SDR
	// VAAPI only: decode and scale on the GPU too (verified at start-up), and
	// tone map there (the iHD driver's tonemap_vaapi). Without HWDecode the
	// CPU decodes and scales and the GPU only encodes, which for 4K HEVC is
	// most of the work on the CPU.
	HWDecode  bool
	HWTonemap bool
}

// HWDecodable lists codecs worth handing to the GPU's decoder. Anything it
// turns out not to support falls back per session (see Session.hwDecode).
var HWDecodable = map[string]bool{"h264": true, "hevc": true, "mpeg2video": true, "vp9": true, "av1": true, "vc1": true}

var hwEncoders = map[string]string{"nvenc": "h264_nvenc", "vaapi": "h264_vaapi", "qsv": "h264_qsv"}

// Detect checks which encoder actually works. A requested hardware encoder
// that's missing from ffmpeg or fails a one-frame test encode falls back to
// software, with a warning, rather than breaking playback.
func Detect(ctx context.Context, ffmpeg, want, vaapiDevice string) Encoder {
	e := Encoder{FFmpeg: ffmpeg, HW: "none", VAAPIDevice: vaapiDevice}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	filters, _ := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-filters").Output()
	e.Tonemap = strings.Contains(string(filters), " zscale ") && strings.Contains(string(filters), " tonemap ")

	want = strings.ToLower(strings.TrimSpace(want))
	if want == "" || want == "none" || want == "software" {
		return e
	}
	enc, ok := hwEncoders[want]
	if !ok {
		slog.Warn("unknown COUCHSIDE_HWACCEL, using software encoding", "value", want)
		return e
	}
	encoders, _ := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-encoders").Output()
	if !strings.Contains(string(encoders), " "+enc+" ") {
		slog.Warn("ffmpeg has no hardware encoder for COUCHSIDE_HWACCEL, using software", "hwaccel", want, "encoder", enc)
		return e
	}
	test := Encoder{FFmpeg: ffmpeg, HW: want, VAAPIDevice: vaapiDevice}
	in, out := test.Video(VideoOpts{MaxHeight: 240, SrcHeight: 240, BitrateK: 500})
	args := append([]string{"-hide_banner", "-loglevel", "error"}, in...)
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25:duration=0.4")
	args = append(args, out...)
	args = append(args, "-f", "null", "-")
	if msg, err := exec.CommandContext(ctx, ffmpeg, args...).CombinedOutput(); err != nil {
		slog.Warn("hardware encoder test failed, using software", "hwaccel", want, "err", err, "ffmpeg", Tail(string(msg), 300))
		return e
	}
	e.HW = want
	if want == "vaapi" {
		e.HWDecode = testHWDecode(ctx, test, string(filters))
		e.HWTonemap = e.HWDecode && strings.Contains(string(filters), " tonemap_vaapi ")
		slog.Info("vaapi pipeline", "gpuDecode", e.HWDecode, "gpuTonemap", e.HWTonemap)
	}
	return e
}

// testHWDecode checks the whole GPU pipeline: it encodes a short clip on the
// GPU, then decodes, scales and re-encodes it there.
func testHWDecode(ctx context.Context, e Encoder, filters string) bool {
	if !strings.Contains(filters, " scale_vaapi ") {
		return false
	}
	dir, err := os.MkdirTemp("", "couchside-hwtest-")
	if err != nil {
		return false
	}
	defer os.RemoveAll(dir)
	clip := filepath.Join(dir, "clip.mp4")
	in, out := e.Video(VideoOpts{MaxHeight: 240, SrcHeight: 240, BitrateK: 500})
	args := append([]string{"-hide_banner", "-loglevel", "error"}, in...)
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25:duration=0.4")
	args = append(args, out...)
	args = append(args, "-y", clip)
	if msg, err := exec.CommandContext(ctx, e.FFmpeg, args...).CombinedOutput(); err != nil {
		slog.Warn("vaapi decode test: couldn't make a test clip", "err", err, "ffmpeg", Tail(string(msg), 300))
		return false
	}
	full := e
	full.HWDecode = true
	in, out = full.Video(VideoOpts{MaxHeight: 120, SrcHeight: 240, BitrateK: 300, HWDecode: true})
	args = append([]string{"-hide_banner", "-loglevel", "error"}, in...)
	args = append(args, "-i", clip)
	args = append(args, out...)
	args = append(args, "-f", "null", "-")
	if msg, err := exec.CommandContext(ctx, e.FFmpeg, args...).CombinedOutput(); err != nil {
		slog.Warn("vaapi decode test failed: the GPU will only encode", "err", err, "ffmpeg", Tail(string(msg), 300))
		return false
	}
	return true
}

// VideoOpts describes one H.264 encode.
type VideoOpts struct {
	MaxHeight   int  // output height cap; 0 keeps the source height. With SrcHeight unknown the cap is a filter expression, so a smaller picture is never scaled up
	SrcHeight   int  // 0 when unknown
	BitrateK    int  // target/peak bitrate in kbit/s
	HDR         bool // source is PQ/HLG: tone map to SDR
	File        bool // whole-file encode: slower preset, better compression
	Deinterlace bool // broadcast TV: deinterlace frames flagged interlaced
	Live        bool // live TV: steady frame-by-frame output over compression
	HWDecode    bool // VAAPI: decode, scale and tone map on the GPU (needs Encoder.HWDecode)
	Exact       bool // scale to MaxHeight exactly, up as well as down: pieces joined into one stream need one size
}

// Video returns ffmpeg arguments that go before -i (device setup) and after
// it (a -vf filter chain plus encoder settings).
func (e Encoder) Video(o VideoOpts) (in, out []string) {
	in, chain, codec := e.VideoParts(o)
	return in, append([]string{"-vf", chain}, codec...)
}

// VideoParts is Video split up, for callers that need the filter chain inside
// a -filter_complex (e.g. to burn in subtitles first).
func (e Encoder) VideoParts(o VideoOpts) (in []string, chain string, codec []string) {
	if e.HW == "vaapi" && o.HWDecode && e.HWDecode {
		return e.vaapiFull(o)
	}
	var f []string
	if o.Deinterlace {
		// Only touches frames flagged interlaced, so progressive channels pass through.
		f = append(f, "yadif=mode=send_frame:parity=auto:deint=interlaced")
	}
	switch {
	case o.MaxHeight > 0 && o.Exact:
		f = append(f, fmt.Sprintf("scale=-2:%d", o.MaxHeight))
	case o.MaxHeight > 0 && o.SrcHeight == 0:
		// Size unknown (a live stream): cap the height, never scale up.
		f = append(f, fmt.Sprintf("scale=-2:'min(ih,%d)'", o.MaxHeight))
	case o.MaxHeight > 0 && o.SrcHeight > o.MaxHeight:
		f = append(f, fmt.Sprintf("scale=-2:%d", o.MaxHeight))
	}
	if o.HDR && e.Tonemap {
		// Scale first (cheaper), then linearise, tone map with Hable and
		// convert to BT.709 so HDR sources don't look washed out.
		f = append(f, "zscale=t=linear:npl=100", "format=gbrpf32le", "zscale=p=bt709",
			"tonemap=tonemap=hable:desat=0", "zscale=t=bt709:m=bt709:r=tv")
	}
	br := fmt.Sprintf("%dk", o.BitrateK)
	buf := fmt.Sprintf("%dk", o.BitrateK*2)

	switch e.HW {
	case "vaapi":
		f = append(f, "format=nv12", "hwupload")
		in = []string{"-vaapi_device", e.VAAPIDevice}
		codec = []string{"-c:v", "h264_vaapi", "-b:v", br, "-maxrate", br, "-bufsize", buf}
		if o.Live {
			codec = append(codec, "-bf", "0")
		}
	case "qsv":
		f = append(f, "format=nv12")
		preset := "veryfast"
		if o.File {
			preset = "medium"
		}
		codec = []string{"-c:v", "h264_qsv", "-preset", preset, "-b:v", br, "-maxrate", br, "-bufsize", buf}
	case "nvenc":
		f = append(f, "format=yuv420p")
		preset := "p4"
		if o.File {
			preset = "p6"
		}
		codec = []string{"-c:v", "h264_nvenc", "-preset", preset, "-rc", "vbr", "-cq", "23",
			"-b:v", br, "-maxrate", br, "-bufsize", buf, "-profile:v", "high"}
	default:
		f = append(f, "format=yuv420p")
		preset, crf := "veryfast", "22"
		if o.File {
			preset, crf = "medium", "20"
		}
		codec = []string{"-c:v", "libx264", "-preset", preset, "-crf", crf,
			"-maxrate", br, "-bufsize", buf, "-profile:v", "high"}
		if o.Live {
			// No lookahead or B-frames: frames leave the encoder as they come in,
			// so segments arrive evenly instead of in bursts the player stalls between.
			codec = append(codec, "-tune", "zerolatency")
		}
	}
	return in, strings.Join(f, ","), codec
}

// vaapiFull keeps frames on the GPU from decode to encode, as Plex and
// Jellyfin do: VAAPI decodes, scale_vaapi resizes (and converts 10-bit to
// 8-bit), tonemap_vaapi maps HDR to SDR, h264_vaapi encodes. When the driver
// can't tone map, only the already-scaled frames visit the CPU for it.
func (e Encoder) vaapiFull(o VideoOpts) (in []string, chain string, codec []string) {
	in = []string{"-init_hw_device", "vaapi=va:" + e.VAAPIDevice, "-filter_hw_device", "va",
		"-hwaccel", "vaapi", "-hwaccel_device", "va", "-hwaccel_output_format", "vaapi"}
	var f []string
	if o.Deinterlace {
		// auto=1: only frames flagged interlaced, like yadif's deint=interlaced.
		f = append(f, "deinterlace_vaapi=auto=1")
	}
	size := ""
	switch {
	case o.MaxHeight > 0 && o.Exact:
		size = fmt.Sprintf("w=-2:h=%d:", o.MaxHeight)
	case o.MaxHeight > 0 && o.SrcHeight == 0:
		size = fmt.Sprintf(`w=-2:h=min(ih\,%d):`, o.MaxHeight) // size unknown: cap it, never scale up
	case o.MaxHeight > 0 && o.SrcHeight > o.MaxHeight:
		size = fmt.Sprintf("w=-2:h=%d:", o.MaxHeight)
	}
	switch {
	case o.HDR && e.HWTonemap:
		f = append(f, "scale_vaapi="+size+"format=p010", "tonemap_vaapi=format=nv12:p=bt709:t=bt709:m=bt709")
	case o.HDR && e.Tonemap:
		f = append(f, "scale_vaapi="+size+"format=p010", "hwdownload", "format=p010le",
			"zscale=t=linear:npl=100", "format=gbrpf32le", "zscale=p=bt709",
			"tonemap=tonemap=hable:desat=0", "zscale=t=bt709:m=bt709:r=tv", "format=nv12", "hwupload")
	default:
		f = append(f, "scale_vaapi="+size+"format=nv12")
	}
	br := fmt.Sprintf("%dk", o.BitrateK)
	codec = []string{"-c:v", "h264_vaapi", "-b:v", br, "-maxrate", br, "-bufsize", fmt.Sprintf("%dk", o.BitrateK*2)}
	if o.Live {
		codec = append(codec, "-bf", "0") // no B-frames: segments arrive as steadily as the broadcast
	}
	return in, strings.Join(f, ","), codec
}

// AudioArgs encodes to stereo AAC, which every browser plays.
func AudioArgs() []string { return []string{"-c:a", "aac", "-ac", "2", "-b:a", "192k"} }

// BitrateFor picks a streaming bitrate for an output height.
func BitrateFor(height int) int {
	switch {
	case height >= 1080:
		return 8000
	case height >= 720:
		return 4000
	case height >= 480:
		return 1500
	default:
		return 1000
	}
}

// OutputHeight clamps a requested height to the source, capped at 1080p
// (H.264 4K is heavy to encode and few browsers need it).
func OutputHeight(requested, src int) int {
	h := requested
	if h <= 0 || h > 1080 {
		h = 1080
	}
	if src > 0 && src < h {
		h = src
	}
	return h
}

// Tail is the last n bytes of s, trimmed: the end of ffmpeg's output, where
// the error is.
func Tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}
