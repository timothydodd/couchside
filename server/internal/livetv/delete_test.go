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

// A recording whose pieces couldn't be joined keeps them, tied to its row:
// Recover joins them, and deleting the row removes them.
func TestRecoverAndDeleteAFailedJoin(t *testing.T) {
	s, d, _ := newTestService(t)
	ctx := context.Background()
	dir := t.TempDir()
	failed := func(name string, parts ...int) (int64, string) {
		t.Helper()
		path := filepath.Join(dir, name+".ts")
		for _, n := range parts {
			if err := os.WriteFile(strings.TrimSuffix(path, ".ts")+".part"+string(rune('0'+n))+".ts", []byte("video"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		id, _, err := d.ScheduleRecording(ctx, db.Recording{Channel: "2.1", Title: name, StartAt: int64(100 * (len(name))), EndAt: int64(100*len(name)) + 50}, 0)
		if err != nil {
			t.Fatal(err)
		}
		if err := d.FinishRecording(ctx, id, "failed", path, 0, "Couldn't finish the file"); err != nil {
			t.Fatal(err)
		}
		return id, path
	}
	id, path := failed("Show - S01E01", 0)
	r, _ := d.Recording(ctx, id)
	if !s.Recoverable(r) {
		t.Fatal("a failed recording with a piece on disk should be recoverable")
	}
	if err := s.Recover(ctx, id); err != nil {
		t.Fatal(err)
	}
	r, _ = d.Recording(ctx, id)
	if _, err := os.Stat(path); err != nil || r.Status != "completed" || r.Size == 0 || s.Recoverable(r) {
		t.Fatalf("after recover: %+v, file %v", r, err)
	}
	if err := s.Recover(ctx, id); err == nil {
		t.Fatal("recovering a completed recording should be refused")
	}

	gone, gonePath := failed("Other Show - S01E02", 0, 2)
	if err := s.Delete(ctx, gone); err != nil {
		t.Fatal(err)
	}
	if left := partFiles(gonePath); len(left) != 0 {
		t.Fatalf("dismissing a failed recording left its pieces: %v", left)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("deleting one recording removed another's file")
	}
}
