package livetv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/usererr"
)

// A recording whose file can't be removed: the error is one the API may
// show, and it doesn't name the path.
func TestDeleteErrorHidesThePath(t *testing.T) {
	s, d, _ := newTestService(t)
	ctx := context.Background()
	// A non-empty folder named like a recording: os.Remove fails on it.
	path := filepath.Join(t.TempDir(), "Show - S01E01.ts")
	if err := os.MkdirAll(filepath.Join(path, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	id, _, err := d.ScheduleRecording(ctx, db.Recording{Channel: "2.1", Title: "Show", StartAt: 100, EndAt: 200}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.FinishRecording(ctx, id, "completed", path, 1, ""); err != nil {
		t.Fatal(err)
	}
	err = s.Delete(ctx, id)
	if err == nil {
		t.Fatal("deleting a recording whose file can't be removed succeeded")
	}
	if !usererr.Is(err) || strings.Contains(err.Error(), filepath.Dir(path)) {
		t.Fatalf("error = %q: want a message for the user without the path", err)
	}
	if _, err := d.Recording(ctx, id); err != nil {
		t.Fatalf("the row should stay when the file couldn't be removed: %v", err)
	}
}
