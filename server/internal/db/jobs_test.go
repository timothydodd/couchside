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

func TestAcknowledgedFailuresLeaveTheCount(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	fail := func(ref int64) int64 {
		t.Helper()
		if err := d.Enqueue(ctx, "trickplay", ref, "Preview thumbnails"); err != nil {
			t.Fatal(err)
		}
		j, err := d.ClaimJob(ctx, true)
		if err != nil || j == nil {
			t.Fatal(j, err)
		}
		if err := d.FinishJob(ctx, j.ID, context.DeadlineExceeded); err != nil {
			t.Fatal(err)
		}
		return j.ID
	}
	first, second := fail(1), fail(2)

	if ok, err := d.AcknowledgeJob(ctx, first); err != nil || !ok {
		t.Fatalf("AcknowledgeJob = %v, %v", ok, err)
	}
	if c, _ := d.JobCounts(ctx); c.Failed != 1 {
		t.Fatalf("failed count after acknowledging one of two = %d, want 1", c.Failed)
	}
	// Still a failure in Activity, so catch-up doesn't queue the work again.
	if refs, _ := d.FailedJobRefs(ctx, "trickplay"); !refs[1] || !refs[2] {
		t.Fatalf("FailedJobRefs = %v, want both refs", refs)
	}
	// The one that still needs someone is listed first.
	jobs, err := d.RecentJobs(ctx, 10)
	if err != nil || len(jobs) != 2 || jobs[0].ID != second || !jobs[1].Acknowledged {
		t.Fatalf("RecentJobs = %+v, %v", jobs, err)
	}

	if err := d.AcknowledgeFailedJobs(ctx); err != nil {
		t.Fatal(err)
	}
	if c, _ := d.JobCounts(ctx); c.Failed != 0 {
		t.Fatalf("failed count after acknowledging all = %d, want 0", c.Failed)
	}

	// A retry that fails again is a new failure.
	if err := d.RetryJob(ctx, first); err != nil {
		t.Fatal(err)
	}
	j, err := d.ClaimJob(ctx, true)
	if err != nil || j == nil || j.ID != first {
		t.Fatal(j, err)
	}
	if err := d.FinishJob(ctx, first, context.DeadlineExceeded); err != nil {
		t.Fatal(err)
	}
	if c, _ := d.JobCounts(ctx); c.Failed != 1 {
		t.Fatalf("failed count after a retry failed again = %d, want 1", c.Failed)
	}
}
