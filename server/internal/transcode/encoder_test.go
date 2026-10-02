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
