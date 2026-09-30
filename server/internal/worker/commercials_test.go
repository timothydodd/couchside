package worker

import (
	"reflect"
	"testing"

	"github.com/timothydodd/couchside/internal/db"
)

func TestParseEDL(t *testing.T) {
	in := "610.21\t790.50\t0\n" +
		"0.00\t32.10\t0\n" +
		"790.80\t850.00\t3\n" + // touches the previous break: merged
		"garbage line\n" +
		"900\t880\t0\n" + // end before start: dropped
		"-1.5\t2\t0\n"
	got := parseEDL(in)
	want := []db.Segment{{Start: 0, End: 32.1}, {Start: 610.21, End: 850}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseEDL = %v, want %v", got, want)
	}
	if got := parseEDL(""); len(got) != 0 || got == nil {
		t.Fatalf("empty EDL = %#v, want empty non-nil slice", got)
	}
}
