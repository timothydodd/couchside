package api

import (
	"reflect"
	"testing"

	"github.com/timothydodd/couchside/internal/db"
)

func TestMergeBreaks(t *testing.T) {
	in := []db.Segment{{Start: 100, End: 220}, {Start: 245, End: 300}, {Start: 900, End: 1080}, {Start: 1140, End: 1200}}
	got := mergeBreaks(in, 60)
	want := []db.Segment{{Start: 100, End: 300}, {Start: 900, End: 1080}, {Start: 1140, End: 1200}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("gap 60: got %v, want %v", got, want)
	}
	if got := mergeBreaks(in, 0); !reflect.DeepEqual(got, in) {
		t.Fatalf("gap 0 should change nothing: got %v", got)
	}
}

// A recording that starts in the previous programme's ads: the short scene
// after that lead-in is the cold open, not a promo, so it isn't skipped.
// (Two and a Half Men S6E2: ads to 2:00, a 53s cold open, then the first break.)
func TestMergeBreaksKeepsTheColdOpen(t *testing.T) {
	in := []db.Segment{{Start: 0, End: 120.86}, {Start: 174.04, End: 249.58}, {Start: 479.58, End: 586.15}, {Start: 625.96, End: 915.29}}
	got := mergeBreaks(in, 60)
	want := []db.Segment{{Start: 0, End: 120.86}, {Start: 174.04, End: 249.58}, {Start: 479.58, End: 915.29}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
}
