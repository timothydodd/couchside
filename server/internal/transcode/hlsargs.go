package transcode

import (
	"fmt"
	"strconv"
)

// HLSOutput is the end of an ffmpeg command that writes HLS: the muxer, its
// segment length and playlist options, and where the files go. Every stream
// Couchside makes (a file's session, live TV, a virtual channel's pieces)
// builds its output with this, so an option one of them needs can't be
// forgotten by another.
type HLSOutput struct {
	SegDur   int    // seconds per segment
	Segments string // segment file pattern, e.g. /dir/seg%d.ts
	Playlist string // the playlist ffmpeg writes
	// Start numbers the first segment (-start_number); negative leaves
	// ffmpeg's default of 0.
	Start int
	// Window keeps only the last Window segments, deleting older ones (a
	// tuner stream). 0 keeps every segment.
	Window int
	// Event marks a playlist that only grows (EXT-X-PLAYLIST-TYPE:EVENT), so
	// players allow seeking back to its start. Ignored with a Window.
	Event bool
	// Independent says every segment starts with a keyframe
	// (EXT-X-INDEPENDENT-SEGMENTS). True wherever keyframes are forced.
	Independent bool
}

// Args are the output arguments, to go last on the command line. Segments
// are always written under a temporary name and renamed when complete, so a
// request never reads half a segment.
func (o HLSOutput) Args() []string {
	args := []string{"-f", "hls", "-hls_time", strconv.Itoa(o.SegDur), "-hls_segment_type", "mpegts"}
	flags := "temp_file"
	if o.Independent {
		flags += "+independent_segments"
	}
	switch {
	case o.Window > 0:
		args = append(args, "-hls_list_size", strconv.Itoa(o.Window), "-hls_delete_threshold", "1")
		flags += "+delete_segments"
	case o.Event:
		args = append(args, "-hls_list_size", "0", "-hls_playlist_type", "event")
	default:
		args = append(args, "-hls_list_size", "0")
	}
	args = append(args, "-hls_flags", flags)
	if o.Start >= 0 {
		args = append(args, "-start_number", strconv.Itoa(o.Start))
	}
	return append(args, "-hls_segment_filename", o.Segments, o.Playlist)
}

// ForceKeyFrames makes an encoder start a keyframe every segDur seconds of
// its output, so segments are cut exactly on the grid.
func ForceKeyFrames(segDur int) []string {
	return []string{"-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", segDur)}
}
