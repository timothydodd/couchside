package api

import (
	"reflect"
	"testing"

	"github.com/timothydodd/couchside/internal/livetv"
)

// A channel's libraries and picked titles travel by name: exported from one
// server and resolved on another whose ids are different.
func TestChannelNamesRoundTrip(t *testing.T) {
	here := channelNames{
		libraries: map[int64]string{1: "Movies", 2: "TV"},
		titles:    map[int64]portableTitle{10: {"The General", 1926, "movie"}, 11: {"The Beverly Hillbillies", 1962, "series"}},
	}
	cfg := livetv.VirtualConfig{Libraries: []int64{2, 99}, Kinds: []string{"series"}, Items: []int64{11, 98}, Order: "sequential",
		Filler: livetv.Filler{Folder: "/media/Commercials", Align: 30, BreakEvery: 12, BreakLength: 120}}
	p := here.export(cfg)
	if !reflect.DeepEqual(p.Libraries, []string{"TV"}) || len(p.Items) != 1 || p.Items[0].Title != "The Beverly Hillbillies" {
		t.Fatalf("export = %+v (ids that are gone should be dropped)", p)
	}

	there := channelNames{
		libraries: map[int64]string{7: "tv", 8: "Cartoons"},
		titles:    map[int64]portableTitle{40: {"the beverly hillbillies", 1962, "series"}},
	}
	got, warnings, err := there.resolve(p)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("resolve: %v, warnings %v", err, warnings)
	}
	if !reflect.DeepEqual(got.Libraries, []int64{7}) || !reflect.DeepEqual(got.Items, []int64{40}) || got.Order != "sequential" || got.Filler != cfg.Filler {
		t.Fatalf("resolved = %+v", got)
	}

	// One name missing is a warning; all of them missing is an error, or the
	// channel would play the whole library.
	p.Libraries = []string{"TV", "Anime"}
	if _, warnings, err := there.resolve(p); err != nil || len(warnings) != 1 {
		t.Fatalf("one missing library: %v, warnings %v", err, warnings)
	}
	p.Libraries = []string{"Anime"}
	if _, _, err := there.resolve(p); err == nil {
		t.Fatal("no library found: want an error")
	}
	p.Libraries, p.Items = nil, []portableTitle{{"Nothing Here", 2000, "movie"}}
	if _, _, err := there.resolve(p); err == nil {
		t.Fatal("no picked title found: want an error")
	}
}
