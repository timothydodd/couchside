package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestClaimJobOnABrokenDatabase(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_ = d.Enqueue(ctx, "scan", 1, "Scan")
	d.Close()
	job, err := d.ClaimJob(ctx, false)
	if err == nil || job != nil {
		t.Fatalf("ClaimJob on a closed DB = %+v, %v; want nil job and an error", job, err)
	}
}

func TestRequeueRunningFailsRepeatOffenders(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	if err := d.Enqueue(ctx, "scan", 1, "Scan"); err != nil {
		t.Fatal(err)
	}
	var id int64
	for i := 0; i < maxAttempts; i++ {
		j, err := d.ClaimJob(ctx, false)
		if err != nil || j == nil {
			t.Fatal(j, err)
		}
		id = j.ID
		if err := d.RequeueRunning(ctx); err != nil { // the server restarts mid-job
			t.Fatal(err)
		}
	}
	j, _ := d.Job(ctx, id)
	if j.Status != "failed" {
		t.Fatalf("after %d crashed attempts the job is %q, want failed", maxAttempts, j.Status)
	}
}

func TestPendingMatches(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	lib, _ := d.CreateLibrary(ctx, "Films", "/m", "movies")
	failed, _, _ := d.EnsureItem(ctx, lib, "movie", "Heat", 1995)  // its match failed: still pending
	queued, _, _ := d.EnsureItem(ctx, lib, "movie", "Alien", 1979) // pending, match already queued
	notFound, _, _ := d.EnsureItem(ctx, lib, "movie", "Zzz", 2001) // the provider said not found
	_ = d.Enqueue(ctx, "match", queued, "Match Alien")
	if err := d.ClearMatch(ctx, notFound); err != nil {
		t.Fatal(err)
	}
	got, err := d.PendingMatches(ctx, lib)
	if err != nil || len(got) != 1 || got[0].ID != failed {
		t.Fatalf("PendingMatches = %+v, %v; want only Heat", got, err)
	}
}
