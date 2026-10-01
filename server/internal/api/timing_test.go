package api

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/timothydodd/couchside/internal/db"
)

func TestTrimBreaks(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s := &Server{db: d}
	ctx := context.Background()
	segs := []db.Segment{{Start: 10, End: 70}, {Start: 100, End: 102}}

	// Defaults: skip from 1s into each break to 1s before its end; a break
	// left under a second long isn't skipped at all.
	got := s.trimBreaks(ctx, segs)
	if len(got) != 1 || got[0].Start != 11 || got[0].End != 69 {
		t.Errorf("default trim = %+v", got)
	}
	if err := d.SetSetting(ctx, settingSkipAfterStart, "0"); err != nil {
		t.Fatal(err)
	}
	if err := d.SetSetting(ctx, settingSkipBeforeEnd, "2.5"); err != nil {
		t.Fatal(err)
	}
	got = s.trimBreaks(ctx, segs)
	if len(got) != 1 || got[0].Start != 10 || got[0].End != 67.5 {
		t.Errorf("custom trim = %+v", got)
	}
}
