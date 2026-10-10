// Package transcode turns files browsers can't play (or can't stream fast
// enough) into H.264/AAC: live HLS sessions for the player, and whole-file
// encodes for background "optimize" jobs.
package transcode

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Encoder builds ffmpeg video arguments for the configured hardware.
type Encoder struct {
	FFmpeg      string
	HW          string // none | vaapi | qsv | nvenc, after detection
	VAAPIDevice string
	Tonemap     bool // zscale available for HDR → SDR
	// VAAPI and NVENC: decode and scale on the GPU too (verified at
	// start-up), and tone map there (TonemapFilter names the filter). Without
	// HWDecode the CPU decodes and scales and the GPU only encodes, which for
	// 4K HEVC is most of the work on the CPU.
	HWDecode      bool
	HWTonemap     bool
	TonemapFilter string // tonemap_vaapi, tonemap_cuda or tonemap_opencl when HWTonemap
	// Note says why no GPU is used, when one was asked for (or "auto" tried
	// some): each encoder tried and ffmpeg's reason it failed.
	Note string
	// Threads caps each playback or live TV ffmpeg that converts video
	// (config.LiveThreads); 0 lets ffmpeg take every core.
	Threads int
}

// ThreadArgs is ffmpeg's -threads for n, nothing for 0. As a global option
// (before -i) it caps the decoder as well as the encoder, which is what keeps
// a 4K software decode from starving a second stream.
func ThreadArgs(n int) []string {
	if n <= 0 {
		return nil
	}
	return []string{"-threads", strconv.Itoa(n)}
}

// HWDecodable lists codecs worth handing to the GPU's decoder. Anything it
// turns out not to support falls back per session (see Session.hwDecode).
var HWDecodable = map[string]bool{"h264": true, "hevc": true, "mpeg2video": true, "vp9": true, "av1": true, "vc1": true}

var hwEncoders = map[string]string{"nvenc": "h264_nvenc", "vaapi": "h264_vaapi", "qsv": "h264_qsv"}

// Detect checks which encoder actually works. A requested hardware encoder
// that's missing from ffmpeg or fails a one-frame test encode falls back to
// software, with a warning, rather than breaking playback. "auto" tries each
// encoder this platform might have (autoCandidates) and keeps the first that
// works.
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
	auto := want == "auto"
	candidates := []string{want}
	if auto {
		candidates = autoCandidates(vaapiDevice)
	} else if _, ok := hwEncoders[want]; !ok {
		slog.Warn("unknown COUCHSIDE_HWACCEL, using software encoding", "value", want)
		e.Note = "unknown COUCHSIDE_HWACCEL " + want
		return e
	}
	encoders, _ := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-encoders").Output()
	var notes []string
	for _, hw := range candidates {
		if err := testEncoder(ctx, Encoder{FFmpeg: ffmpeg, HW: hw, VAAPIDevice: vaapiDevice}, string(encoders)); err != nil {
			notes = append(notes, hw+": "+err.Error())
			if !auto {
				slog.Warn("hardware encoder test failed, using software", "hwaccel", hw, "err", err)
			}
			continue
		}
		e.HW = hw
		break
	}
	if e.HW == "none" {
		e.Note = strings.Join(notes, "; ")
		if auto && len(notes) > 0 {
			slog.Info("no usable GPU encoder, using software", "tried", e.Note)
		}
		return e
	}
	switch e.HW {
	case "vaapi":
		e.HWDecode = testHWDecode(ctx, e, string(filters))
		if e.HWDecode && strings.Contains(string(filters), " tonemap_vaapi ") {
			e.HWTonemap, e.TonemapFilter = true, "tonemap_vaapi"
		}
		slog.Info("vaapi pipeline", "gpuDecode", e.HWDecode, "gpuTonemap", e.TonemapFilter)
	case "nvenc":
		e.HWDecode = testHWDecode(ctx, e, string(filters))
		if e.HWDecode {
			// tonemap_cuda is jellyfin-ffmpeg's; tonemap_opencl is in stock
			// ffmpeg and runs on NVIDIA's OpenCL, with a round trip through
			// memory because OpenCL can't map CUDA frames.
			for _, f := range []string{"tonemap_cuda", "tonemap_opencl"} {
				if strings.Contains(string(filters), " "+f+" ") && testCUDATonemap(ctx, e, f) {
					e.HWTonemap, e.TonemapFilter = true, f
					break
				}
			}
		}
		slog.Info("nvenc pipeline", "gpuDecode", e.HWDecode, "gpuTonemap", e.TonemapFilter)
	}
	return e
}

// testCUDATonemap runs a synthetic 10-bit PQ clip through the CUDA pipeline
// with the given tone mapper, the way an HDR session would.
func testCUDATonemap(ctx context.Context, e Encoder, filter string) bool {
	e.HWTonemap, e.TonemapFilter = true, filter
	o := VideoOpts{MaxHeight: 120, SrcHeight: 240, BitrateK: 300, HDR: true, HWDecode: true}
	_, chain, codec := e.VideoParts(o)
	// Only the devices, no -hwaccel: the clip is lavfi, so it's uploaded by hand.
	args := append([]string{"-hide_banner", "-loglevel", "error"}, e.cudaDevices(o)...)
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25:duration=0.4")
	args = append(args, "-vf", "format=p010le,setparams=color_primaries=bt2020:color_trc=smpte2084:colorspace=bt2020nc,hwupload_cuda,"+chain)
	args = append(args, codec...)
	args = append(args, "-f", "null", "-")
	if msg, err := exec.CommandContext(ctx, e.FFmpeg, args...).CombinedOutput(); err != nil {
		slog.Warn("nvenc tone map test failed", "filter", filter, "err", err, "ffmpeg", Tail(string(msg), 300))
		return false
	}
	return true
}

// autoCandidates are the encoders "auto" tries, best first. A discrete
// NVIDIA card beats the Intel iGPU beside it; VAAPI is Linux only and needs
// its render node.
func autoCandidates(vaapiDevice string) []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"nvenc", "qsv"}
	case "linux":
		var c []string
		if _, err := os.Stat(vaapiDevice); err == nil {
			c = append(c, "vaapi")
		}
		return append(c, "nvenc", "qsv")
	}
	return nil
}

// testEncoder runs a one-frame test encode with e's hardware encoder. The
// error is ffmpeg's own reason, such as an NVIDIA driver too old for this
// ffmpeg, so Settings can show why the GPU isn't used.
func testEncoder(ctx context.Context, e Encoder, encoders string) error {
	if enc := hwEncoders[e.HW]; !strings.Contains(encoders, " "+enc+" ") {
		return fmt.Errorf("this ffmpeg has no %s", enc)
	}
	in, out := e.Video(VideoOpts{MaxHeight: 240, SrcHeight: 240, BitrateK: 500})
	args := append([]string{"-hide_banner", "-loglevel", "error"}, in...)
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25:duration=0.4")
	args = append(args, out...)
	args = append(args, "-f", "null", "-")
	if msg, err := exec.CommandContext(ctx, e.FFmpeg, args...).CombinedOutput(); err != nil {
		if r := ffmpegReason(string(msg)); r != "" {
			return errors.New(r)
		}
		return err
	}
	return nil
}

// ffmpegReason picks the line of ffmpeg's error output that says what's
// wrong, without its "[h264_nvenc @ 0x…]" prefix: the first two lines naming
// a driver or device (NVENC says which driver it needs), else the first line.
func ffmpegReason(out string) string {
	var lines []string
	for _, l := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		if i := strings.Index(l, "] "); strings.HasPrefix(l, "[") && i > 0 {
			l = l[i+2:]
		}
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	var picked []string
	for _, l := range lines {
		low := strings.ToLower(l)
		if strings.Contains(low, "driver") || strings.Contains(low, "device") || strings.Contains(low, "no capable") {
			picked = append(picked, l)
		}
	}
	if len(picked) == 0 {
		picked = lines[:1]
	}
	r := strings.Join(picked[:min(len(picked), 2)], " ")
	if len(r) > 240 {
		r = r[:240] + "…"
	}
	return r
}

// testHWDecode checks the whole GPU pipeline: it encodes a short clip on the
// GPU, then decodes, scales and re-encodes it there.
func testHWDecode(ctx context.Context, e Encoder, filters string) bool {
	scaler := map[string]string{"vaapi": "scale_vaapi", "nvenc": "scale_cuda"}[e.HW]
	if scaler == "" || !strings.Contains(filters, " "+scaler+" ") {
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
		slog.Warn("GPU decode test: couldn't make a test clip", "hw", e.HW, "err", err, "ffmpeg", Tail(string(msg), 300))
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
		slog.Warn("GPU decode test failed: the GPU will only encode", "hw", e.HW, "err", err, "ffmpeg", Tail(string(msg), 300))
		return false
	}
	return true
}

// VideoOpts describes one H.264 encode.
type VideoOpts struct {
	MaxHeight   int  // output height cap; 0 keeps the source height. With SrcHeight unknown the cap is a filter expression, so a smaller picture is never scaled up
	SrcHeight   int  // 0 when unknown
	SrcWidth    int  // 0 when unknown; with SrcHeight, the picture is fitted into a 16:9 box (fitSize)
	BitrateK    int  // target/peak bitrate in kbit/s
	HDR         bool // source is PQ/HLG: tone map to SDR
	File        bool // whole-file encode: slower preset, better compression
	Deinterlace bool // broadcast TV: deinterlace frames flagged interlaced
	Live        bool // live TV: steady frame-by-frame output over compression
	HWDecode    bool // VAAPI/NVENC: decode, scale and tone map on the GPU (needs Encoder.HWDecode)
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
	if o.HWDecode && e.HWDecode {
		switch e.HW {
		case "vaapi":
			return e.vaapiFull(o)
		case "nvenc":
			return e.nvencFull(o)
		}
	}
	var f []string
	if o.Deinterlace {
		// Only touches frames flagged interlaced, so progressive channels pass through.
		f = append(f, "yadif=mode=send_frame:parity=auto:deint=interlaced")
	}
	w, h, fit := fitSize(o)
	switch {
	case fit:
		f = append(f, fmt.Sprintf("scale=%d:%d", w, h))
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
		codec = nvencCodec(o)
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
	size := gpuSize(o)
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

// gpuSize is the w/h options for scale_vaapi or scale_cuda, ending in ":"
// when set.
func gpuSize(o VideoOpts) string {
	if w, h, fit := fitSize(o); fit {
		return fmt.Sprintf("w=%d:h=%d:", w, h)
	}
	switch {
	case o.MaxHeight > 0 && o.Exact:
		return fmt.Sprintf("w=-2:h=%d:", o.MaxHeight)
	case o.MaxHeight > 0 && o.SrcHeight == 0:
		return fmt.Sprintf(`w=-2:h=min(ih\,%d):`, o.MaxHeight) // size unknown: cap it, never scale up
	case o.MaxHeight > 0 && o.SrcHeight > o.MaxHeight:
		return fmt.Sprintf("w=-2:h=%d:", o.MaxHeight)
	}
	return ""
}

// fitSize is the output size for a source whose size is known: inside a 16:9
// box as tall as the height cap (1920×1080 for 1080p), keeping the shape and
// both sides even. Scaling by height alone made a 1.85:1 4K film 1998×1080
// and a 2.39:1 one about 2580×1080, wider than a TV's H.264 decoder may take.
// fit is false when the source already fits, the size is unknown, or the
// output must be exactly MaxHeight tall (Exact).
func fitSize(o VideoOpts) (w, h int, fit bool) {
	if o.MaxHeight <= 0 || o.Exact || o.SrcWidth <= 0 || o.SrcHeight <= 0 {
		return 0, 0, false
	}
	even := func(x float64) int { return max(2, 2*int(math.Round(x/2))) }
	boxW := even(float64(o.MaxHeight) * 16 / 9)
	if o.SrcWidth <= boxW && o.SrcHeight <= o.MaxHeight {
		return 0, 0, false
	}
	k := min(float64(boxW)/float64(o.SrcWidth), float64(o.MaxHeight)/float64(o.SrcHeight))
	return min(even(float64(o.SrcWidth)*k), boxW), min(even(float64(o.SrcHeight)*k), o.MaxHeight), true
}

// nvencFull is vaapiFull for NVIDIA: NVDEC decodes into CUDA frames,
// yadif_cuda deinterlaces, scale_cuda resizes (and converts 10-bit to 8-bit;
// its format option needs ffmpeg 7) and h264_nvenc encodes them in place. HDR
// is tone mapped by tonemap_cuda on the GPU, or by tonemap_opencl after a trip
// through memory, or otherwise by zscale on the CPU; in both of the latter
// only scaled-down frames move.
func (e Encoder) nvencFull(o VideoOpts) (in []string, chain string, codec []string) {
	in = append(e.cudaDevices(o), "-hwaccel", "cuda", "-hwaccel_device", "cu", "-hwaccel_output_format", "cuda")
	var f []string
	if o.Deinterlace {
		f = append(f, "yadif_cuda=mode=send_frame:parity=auto:deint=interlaced")
	}
	size := gpuSize(o)
	const tm = "tonemap=hable:desat=0:p=bt709:t=bt709:m=bt709"
	switch {
	case o.HDR && e.HWTonemap && e.TonemapFilter == "tonemap_cuda":
		f = append(f, "scale_cuda="+size+"format=p010", "tonemap_cuda=format=yuv420p:"+tm)
	case o.HDR && e.HWTonemap && e.TonemapFilter == "tonemap_opencl":
		f = append(f, "scale_cuda="+size+"format=p010", "hwdownload", "format=p010le", "hwupload",
			"tonemap_opencl=format=nv12:"+tm, "hwdownload", "format=nv12")
	case o.HDR && e.Tonemap:
		f = append(f, "scale_cuda="+size+"format=p010", "hwdownload", "format=p010le",
			"zscale=t=linear:npl=100", "format=gbrpf32le", "zscale=p=bt709",
			"tonemap=tonemap=hable:desat=0", "zscale=t=bt709:m=bt709:r=tv", "format=nv12")
	default:
		f = append(f, "scale_cuda="+size+"format=nv12")
	}
	return in, strings.Join(f, ","), nvencCodec(o)
}

// cudaDevices sets up the CUDA device, plus an OpenCL one as the filters'
// device (for hwupload) when tonemap_opencl does the tone mapping.
func (e Encoder) cudaDevices(o VideoOpts) []string {
	if o.HDR && e.HWTonemap && e.TonemapFilter == "tonemap_opencl" {
		return []string{"-init_hw_device", "cuda=cu", "-init_hw_device", "opencl=ocl", "-filter_hw_device", "ocl"}
	}
	return []string{"-init_hw_device", "cuda=cu", "-filter_hw_device", "cu"}
}

func nvencCodec(o VideoOpts) []string {
	br := fmt.Sprintf("%dk", o.BitrateK)
	preset := "p4"
	if o.File {
		preset = "p6"
	}
	return []string{"-c:v", "h264_nvenc", "-preset", preset, "-rc", "vbr", "-cq", "23",
		"-b:v", br, "-maxrate", br, "-bufsize", fmt.Sprintf("%dk", o.BitrateK*2), "-profile:v", "high"}
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
