package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// AudioStream and SubtitleStream describe selectable tracks. Index is the
// position among streams of that type (ffmpeg's 0:a:N / 0:s:N).
type AudioStream struct {
	Index    int    `json:"index"`
	Codec    string `json:"codec"`
	Channels int    `json:"channels"`
	Language string `json:"language"`
	Title    string `json:"title"`
	Default  bool   `json:"default"`
}

type SubtitleStream struct {
	Index    int    `json:"index"`
	Codec    string `json:"codec"`
	Language string `json:"language"`
	Title    string `json:"title"`
	Default  bool   `json:"default"`
	Forced   bool   `json:"forced"`
	Text     bool   `json:"text"` // text subs are sent as WebVTT; image subs (PGS, DVD) must be burned in
}

type Streams struct {
	Audio     []AudioStream    `json:"audio"`
	Subtitles []SubtitleStream `json:"subtitles"`
}

var textSubCodecs = map[string]bool{"subrip": true, "srt": true, "ass": true, "ssa": true, "webvtt": true, "mov_text": true, "text": true}

// IsTextSubtitle reports whether a subtitle codec can be converted to WebVTT.
func IsTextSubtitle(codec string) bool { return textSubCodecs[codec] }

// ListStreams reads a file's audio and subtitle tracks.
func ListStreams(ctx context.Context, bin, path string) (*Streams, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "-v", "error", "-print_format", "json", "-show_streams", path).Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe: %w", err)
	}
	var p struct {
		Streams []struct {
			CodecType   string            `json:"codec_type"`
			CodecName   string            `json:"codec_name"`
			Channels    int               `json:"channels"`
			Tags        map[string]string `json:"tags"`
			Disposition struct {
				Default int `json:"default"`
				Forced  int `json:"forced"`
			} `json:"disposition"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &p); err != nil {
		return nil, err
	}
	st := &Streams{Audio: []AudioStream{}, Subtitles: []SubtitleStream{}}
	tag := func(t map[string]string, k string) string {
		for key, v := range t {
			if strings.EqualFold(key, k) {
				return v
			}
		}
		return ""
	}
	for _, s := range p.Streams {
		switch s.CodecType {
		case "audio":
			st.Audio = append(st.Audio, AudioStream{Index: len(st.Audio), Codec: s.CodecName, Channels: s.Channels,
				Language: tag(s.Tags, "language"), Title: tag(s.Tags, "title"), Default: s.Disposition.Default == 1})
		case "subtitle":
			st.Subtitles = append(st.Subtitles, SubtitleStream{Index: len(st.Subtitles), Codec: s.CodecName,
				Language: tag(s.Tags, "language"), Title: tag(s.Tags, "title"), Default: s.Disposition.Default == 1,
				Forced: s.Disposition.Forced == 1, Text: textSubCodecs[s.CodecName]})
		}
	}
	return st, nil
}
