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
