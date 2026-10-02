package api

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSidecars(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{
		"Alien.mkv", "Alien.Resurrection.mkv", "Alien.Resurrection.srt", "Alien.Resurrection.en.srt",
		"Movie.mkv", "Movie.srt", "Movie.en.srt", "Movie.en.forced.srt", "Movie.pt-BR.sdh.vtt", "Movie.notes.txt",
		"Movie.Director.Commentary.srt", "Movie.en.forced.extra.srt",
		"Movie.2.mkv", "Movie.2.srt", "Movie.2.en.srt",
	} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for video, want := range map[string][]string{
		"Alien.mkv":              nil,
		"Alien.Resurrection.mkv": {"Alien.Resurrection.en.srt", "Alien.Resurrection.srt"},
		"Movie.mkv":              {"Movie.en.forced.srt", "Movie.en.srt", "Movie.pt-BR.sdh.vtt", "Movie.srt"},
		"Movie.2.mkv":            {"Movie.2.en.srt", "Movie.2.srt"},
	} {
		var got []string
		for _, p := range sidecars(filepath.Join(dir, video)) {
			got = append(got, filepath.Base(p))
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("sidecars(%s) = %v, want %v", video, got, want)
		}
	}
}

func TestSidecarTags(t *testing.T) {
	for name, want := range map[string][]string{
		"Movie.srt":            nil,
		"Movie.en.srt":         {"en"},
		"Movie.eng.forced.srt": {"eng", "forced"},
		"Movie.SDH.srt":        {"SDH"},
	} {
		got, ok := sidecarTags("Movie", name)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Errorf("sidecarTags(%s) = %v, %v; want %v", name, got, ok, want)
		}
	}
	for _, name := range []string{"Movie.2.srt", "Movie.Resurrection.srt", "Moviex.srt", "Movie.en.forced.cc.srt"} {
		if _, ok := sidecarTags("Movie", name); ok {
			t.Errorf("sidecarTags(%s) accepted", name)
		}
	}
}
