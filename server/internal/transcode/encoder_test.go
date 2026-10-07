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
