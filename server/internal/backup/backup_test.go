package backup

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/timothydodd/couchside/internal/db"
)

func openData(t *testing.T, dir string) *db.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(dir, "couchside.db"))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// A backup made while the server runs comes back as it was, with the session
// key and server id, and the database it replaced is kept.
func TestBackupAndRestore(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	d := openData(t, dir)
	if _, err := d.SetupAdmin(ctx, "Me", "hash"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "auth.key"), []byte("the-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "server.id"), []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	b, err := Create(ctx, d, dir, Manual)
	if err != nil {
		t.Fatal(err)
	}
	if b.Kind != Manual || b.Size == 0 {
		t.Fatalf("backup = %+v", b)
	}
	zr, err := zip.OpenReader(filepath.Join(Dir(dir), b.Name))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	zr.Close()
	if !names["couchside.db"] || !names["auth.key"] || !names["server.id"] {
		t.Fatalf("zip holds %v", names)
	}
	if left, _ := filepath.Glob(filepath.Join(Dir(dir), ".*")); len(left) != 0 {
		t.Fatalf("temp files left behind: %v", left)
	}

	// Things change after the backup, then it's restored.
	if _, err := d.CreateAccount(ctx, "Later", "accent", "user", false, "", false); err != nil {
		t.Fatal(err)
	}
	d.Close()
	if err := os.WriteFile(filepath.Join(dir, "auth.key"), []byte("a-newer-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Restore(dir, filepath.Join(Dir(dir), b.Name)); err != nil {
		t.Fatal(err)
	}
	d = openData(t, dir)
	defer d.Close()
	ps, err := d.Profiles(ctx)
	if err != nil || len(ps) != 1 || ps[0].Name != "Me" {
		t.Fatalf("after restore: %+v %v", ps, err)
	}
	if key, _ := os.ReadFile(filepath.Join(dir, "auth.key")); string(key) != "the-key" {
		t.Fatalf("auth.key after restore = %q", key)
	}
	if _, err := os.Stat(filepath.Join(dir, "couchside.db.before-restore")); err != nil {
		t.Fatal("the replaced database wasn't kept")
	}
}

func TestRestoreRefusesWhatIsNotABackup(t *testing.T) {
	dir := t.TempDir()
	d := openData(t, dir)
	d.Close()
	junk := filepath.Join(t.TempDir(), "notes.db")
	if err := os.WriteFile(junk, []byte("this is not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Restore(dir, junk); err == nil {
		t.Fatal("restored a file that isn't a database")
	}
	// The live database is untouched.
	if err := db.Check(filepath.Join(dir, "couchside.db")); err != nil {
		t.Fatalf("the database was damaged by a refused restore: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "couchside.db.before-restore")); err == nil {
		t.Fatal("the database was moved aside for a restore that was refused")
	}
}

// Prune keeps the newest daily backups and leaves manual ones alone; names
// that aren't backups can't be read or deleted through Path.
func TestPruneAndPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(Dir(dir), 0o700); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 1, 3, 0, 0, 0, time.Local)
	mk := func(kind string, day int, ext string) string {
		name := "couchside-" + kind + "-" + at.AddDate(0, 0, day).Format("20060102-150405") + "." + ext
		if err := os.WriteFile(filepath.Join(Dir(dir), name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		return name
	}
	for day := range 10 {
		mk(Scheduled, day, "zip")
	}
	manual := mk(Manual, 0, "zip")
	for day := range 5 {
		mk(Upgrade, day, "db")
	}
	if err := Prune(dir, 7); err != nil {
		t.Fatal(err)
	}
	all, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	count := map[string]int{}
	for _, b := range all {
		count[b.Kind]++
	}
	if count[Scheduled] != 7 || count[Manual] != 1 || count[Upgrade] != 3 {
		t.Fatalf("after pruning: %v", count)
	}
	if all[0].At < all[len(all)-1].At {
		t.Fatal("not newest first")
	}
	if _, err := Path(dir, manual); err != nil {
		t.Fatalf("a real backup: %v", err)
	}
	for _, bad := range []string{"../couchside.db", "couchside.db", "couchside-daily-x.zip", "..", ""} {
		if _, err := Path(dir, bad); err == nil {
			t.Fatalf("Path accepted %q", bad)
		}
	}
}
