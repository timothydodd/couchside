package transcode

import (
	"strings"
	"testing"
)

func TestVAAPIFullPipeline(t *testing.T) {
	e := Encoder{HW: "vaapi", VAAPIDevice: "/dev/dri/renderD128", HWDecode: true, HWTonemap: true, Tonemap: true}
	in, chain, codec := e.VideoParts(VideoOpts{MaxHeight: 720, SrcHeight: 2160, BitrateK: 4000, HDR: true, HWDecode: true})
	if got := strings.Join(in, " "); !strings.Contains(got, "-hwaccel vaapi") || !strings.Contains(got, "-hwaccel_output_format vaapi") ||
		!strings.Contains(got, "vaapi=va:/dev/dri/renderD128") {
		t.Errorf("input args = %q", got)
	}
	if chain != "scale_vaapi=w=-2:h=720:format=p010,tonemap_vaapi=format=nv12:p=bt709:t=bt709:m=bt709" {
		t.Errorf("4K HDR → 720p chain = %q (nothing should leave the GPU)", chain)
	}
	if codec[1] != "h264_vaapi" || !strings.Contains(strings.Join(codec, " "), "-b:v 4000k") {
		t.Errorf("codec = %v", codec)
	}

	// No GPU tone mapping: only the scaled-down frames visit the CPU.
	e.HWTonemap = false
	_, chain, _ = e.VideoParts(VideoOpts{MaxHeight: 720, SrcHeight: 2160, BitrateK: 4000, HDR: true, HWDecode: true})
	if !strings.HasPrefix(chain, "scale_vaapi=w=-2:h=720:format=p010,hwdownload,") || !strings.HasSuffix(chain, ",format=nv12,hwupload") {
		t.Errorf("CPU tone map chain = %q", chain)
	}

	// SDR at source size: just the 8-bit conversion, on the GPU.
	_, chain, _ = e.VideoParts(VideoOpts{MaxHeight: 1080, SrcHeight: 1080, BitrateK: 8000, HWDecode: true})
	if chain != "scale_vaapi=format=nv12" {
		t.Errorf("SDR chain = %q", chain)
	}
}

func TestVAAPILiveTV(t *testing.T) {
	e := Encoder{HW: "vaapi", VAAPIDevice: "/dev/dri/renderD128", HWDecode: true}
	_, chain, codec := e.VideoParts(VideoOpts{MaxHeight: 720, BitrateK: 4000, Deinterlace: true, Live: true, HWDecode: true})
	if chain != `deinterlace_vaapi=auto=1,scale_vaapi=w=-2:h=min(ih\,720):format=nv12` {
		t.Errorf("live chain = %q", chain)
	}
	if !strings.Contains(strings.Join(codec, " "), "-bf 0") {
		t.Errorf("live codec = %v (B-frames make live segments bursty)", codec)
	}
}

func TestVAAPIWithoutGPUDecodeKeepsTheOldPipeline(t *testing.T) {
	// Not verified at start-up, or a session that fell back: CPU decodes, GPU encodes.
	for _, e := range []Encoder{
		{HW: "vaapi", VAAPIDevice: "/dev/dri/renderD128"},
		{HW: "vaapi", VAAPIDevice: "/dev/dri/renderD128", HWDecode: true},
	} {
		in, chain, _ := e.VideoParts(VideoOpts{MaxHeight: 720, SrcHeight: 1080, BitrateK: 4000, HWDecode: false})
		if strings.Contains(strings.Join(in, " "), "-hwaccel") || chain != "scale=-2:720,format=nv12,hwupload" {
			t.Errorf("encoder %+v: in=%v chain=%q", e, in, chain)
		}
	}
}

// A live stream's size isn't known up front: the height is a cap the filter
// applies per frame, so a 480-line broadcast asked for at 720p stays 480.
func TestUnknownSourceNeverScalesUp(t *testing.T) {
	e := Encoder{}
	_, chain, _ := e.VideoParts(VideoOpts{MaxHeight: 720, BitrateK: 4000, Live: true})
	if !strings.Contains(chain, "scale=-2:'min(ih,720)'") {
		t.Errorf("chain = %q", chain)
	}
	_, chain, _ = e.VideoParts(VideoOpts{MaxHeight: 720, SrcHeight: 480, BitrateK: 4000})
	if strings.Contains(chain, "scale") {
		t.Errorf("a known smaller source was scaled: %q", chain)
	}
	_, chain, _ = e.VideoParts(VideoOpts{MaxHeight: 0, BitrateK: 4000, Live: true})
	if strings.Contains(chain, "scale") {
		t.Errorf("as-broadcast was scaled: %q", chain)
	}
}

func TestFFmpegReason(t *testing.T) {
	out := "[h264_nvenc @ 0000022751d81fc0] Driver does not support the required nvenc API version. Required: 13.1 Found: 13.0\r\n" +
		"[h264_nvenc @ 0000022751d81fc0] The minimum required Nvidia driver for nvenc is 610.00 or newer\r\n" +
		"[vost#0:0/h264_nvenc @ 0000022751d81d40] [enc:h264_nvenc @ 0000022751d498c0] Error while opening encoder\r\n"
	want := "Driver does not support the required nvenc API version. Required: 13.1 Found: 13.0 The minimum required Nvidia driver for nvenc is 610.00 or newer"
	if got := ffmpegReason(out); got != want {
		t.Errorf("ffmpegReason = %q, want %q", got, want)
	}
	if got := ffmpegReason("[out#0 @ 0x1] Nothing was written\n"); got != "Nothing was written" {
		t.Errorf("fallback = %q", got)
	}
	if got := ffmpegReason(""); got != "" {
		t.Errorf("empty = %q", got)
	}
}

func TestNVENCFullPipeline(t *testing.T) {
	e := Encoder{HW: "nvenc", HWDecode: true, Tonemap: true}
	o := VideoOpts{MaxHeight: 720, SrcHeight: 2160, BitrateK: 4000, HDR: true, HWDecode: true}
	in, chain, codec := e.VideoParts(o)
	if got := strings.Join(in, " "); got != "-init_hw_device cuda=cu -filter_hw_device cu -hwaccel cuda -hwaccel_device cu -hwaccel_output_format cuda" {
		t.Errorf("input args = %q", got)
	}
	// No GPU tone mapper: only the scaled-down frames visit the CPU.
	if !strings.HasPrefix(chain, "scale_cuda=w=-2:h=720:format=p010,hwdownload,format=p010le,zscale") || !strings.HasSuffix(chain, ",format=nv12") {
		t.Errorf("CPU tone map chain = %q", chain)
	}
	if codec[1] != "h264_nvenc" || !strings.Contains(strings.Join(codec, " "), "-b:v 4000k") {
		t.Errorf("codec = %v", codec)
	}

	e.HWTonemap, e.TonemapFilter = true, "tonemap_cuda"
	_, chain, _ = e.VideoParts(o)
	if chain != "scale_cuda=w=-2:h=720:format=p010,tonemap_cuda=format=yuv420p:tonemap=hable:desat=0:p=bt709:t=bt709:m=bt709" {
		t.Errorf("tonemap_cuda chain = %q (nothing should leave the GPU)", chain)
	}

	e.TonemapFilter = "tonemap_opencl"
	in, chain, _ = e.VideoParts(o)
	if got := strings.Join(in, " "); !strings.Contains(got, "-init_hw_device opencl=ocl -filter_hw_device ocl") {
		t.Errorf("opencl input args = %q", got)
	}
	if chain != "scale_cuda=w=-2:h=720:format=p010,hwdownload,format=p010le,hwupload,tonemap_opencl=format=nv12:tonemap=hable:desat=0:p=bt709:t=bt709:m=bt709,hwdownload,format=nv12" {
		t.Errorf("tonemap_opencl chain = %q", chain)
	}

	// SDR live TV: deinterlace and scale on the GPU, OpenCL not involved.
	in, chain, _ = e.VideoParts(VideoOpts{MaxHeight: 720, BitrateK: 4000, Deinterlace: true, Live: true, HWDecode: true})
	if chain != `yadif_cuda=mode=send_frame:parity=auto:deint=interlaced,scale_cuda=w=-2:h=min(ih\,720):format=nv12` || strings.Contains(strings.Join(in, " "), "opencl") {
		t.Errorf("live in=%v chain=%q", in, chain)
	}

	// A session that fell back: CPU decodes, GPU encodes.
	in, chain, _ = e.VideoParts(VideoOpts{MaxHeight: 720, SrcHeight: 1080, BitrateK: 4000})
	if len(in) != 0 || chain != "scale=-2:720,format=yuv420p" {
		t.Errorf("fallback in=%v chain=%q", in, chain)
	}
}

// A wide source is fitted into the 16:9 box for its height cap, on every
// path, rather than scaled by height alone (1998×1080 from a 1.85:1 4K film).
func TestFitSize(t *testing.T) {
	cases := []struct {
		srcW, srcH, maxH int
		w, h             int
		fit              bool
	}{
		{3840, 2076, 1080, 1920, 1038, true}, // 1.85:1 4K (The Blob)
		{3840, 1608, 1080, 1920, 804, true},  // 2.39:1 4K
		{3840, 2160, 1080, 1920, 1080, true},
		{2560, 1080, 1080, 1920, 810, true}, // ultrawide already at the cap
		{1920, 1080, 720, 1280, 720, true},
		{1440, 1080, 720, 960, 720, true}, // 4:3 stays tall
		{1920, 800, 480, 854, 356, true},
		{1920, 1080, 1080, 0, 0, false}, // already fits
		{1280, 720, 1080, 0, 0, false},
		{0, 2160, 1080, 0, 0, false}, // width unknown
	}
	for _, c := range cases {
		w, h, fit := fitSize(VideoOpts{MaxHeight: c.maxH, SrcHeight: c.srcH, SrcWidth: c.srcW})
		if w != c.w || h != c.h || fit != c.fit {
			t.Errorf("fitSize(%dx%d, %d) = %dx%d %v, want %dx%d %v", c.srcW, c.srcH, c.maxH, w, h, fit, c.w, c.h, c.fit)
		}
	}
	if _, _, fit := fitSize(VideoOpts{MaxHeight: 1080, SrcHeight: 2076, SrcWidth: 3840, Exact: true}); fit {
		t.Error("Exact output was fitted")
	}
	o := VideoOpts{MaxHeight: 1080, SrcHeight: 2076, SrcWidth: 3840, BitrateK: 8000, HDR: true, HWDecode: true}
	if _, chain, _ := (Encoder{HW: "vaapi", HWDecode: true, HWTonemap: true}).VideoParts(o); !strings.HasPrefix(chain, "scale_vaapi=w=1920:h=1038:format=p010,") {
		t.Errorf("vaapi chain = %q", chain)
	}
	if _, chain, _ := (Encoder{HW: "nvenc", HWDecode: true}).VideoParts(o); !strings.HasPrefix(chain, "scale_cuda=w=1920:h=1038:") {
		t.Errorf("nvenc chain = %q", chain)
	}
	o.HWDecode = false
	if _, chain, _ := (Encoder{HW: "vaapi"}).VideoParts(o); !strings.HasPrefix(chain, "scale=1920:1038,") {
		t.Errorf("cpu chain = %q", chain)
	}
}
