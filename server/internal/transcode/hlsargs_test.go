package transcode

import (
	"strings"
	"testing"
)

func TestHLSOutputArgs(t *testing.T) {
	for _, c := range []struct {
		name string
		o    HLSOutput
		want string
	}{
		{"a file's session, restarted at segment 12",
			HLSOutput{SegDur: 4, Segments: "/s/seg%d.ts", Playlist: "/s/ffmpeg.m3u8", Start: 12},
			"-f hls -hls_time 4 -hls_segment_type mpegts -hls_list_size 0 -hls_flags temp_file -start_number 12 -hls_segment_filename /s/seg%d.ts /s/ffmpeg.m3u8"},
		{"a tuner stream with a sliding window",
			HLSOutput{SegDur: 2, Segments: "/l/seg%d.ts", Playlist: "/l/index.m3u8", Start: -1, Window: 5400, Independent: true},
			"-f hls -hls_time 2 -hls_segment_type mpegts -hls_list_size 5400 -hls_delete_threshold 1 -hls_flags temp_file+independent_segments+delete_segments -hls_segment_filename /l/seg%d.ts /l/index.m3u8"},
		{"a recording watched from its start",
			HLSOutput{SegDur: 2, Segments: "/l/seg%d.ts", Playlist: "/l/index.m3u8", Start: -1, Event: true, Independent: true},
			"-f hls -hls_time 2 -hls_segment_type mpegts -hls_list_size 0 -hls_playlist_type event -hls_flags temp_file+independent_segments -hls_segment_filename /l/seg%d.ts /l/index.m3u8"},
	} {
		if got := strings.Join(c.o.Args(), " "); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
	if got := strings.Join(ForceKeyFrames(4), " "); got != "-force_key_frames expr:gte(t,n_forced*4)" {
		t.Errorf("ForceKeyFrames = %s", got)
	}
}
