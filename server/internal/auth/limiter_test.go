package auth

import (
	"sync"
	"testing"
	"time"
)

// A burst of parallel attempts gets no more checks than the same attempts in
// a row: Free of them, then one at a time with a lock after each failure.
func TestLimiterBurst(t *testing.T) {
	l := NewLimiter(5, time.Minute, time.Hour, time.Hour)
	var wg sync.WaitGroup
	var mu sync.Mutex
	in := 0
	hold := make(chan struct{})
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.Begin("k") != 0 {
				return
			}
			mu.Lock()
			in++
			mu.Unlock()
			<-hold // the password check
			l.End("k", true)
		}()
	}
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	got := in
	mu.Unlock()
	if got != 5 {
		t.Fatalf("%d of 50 parallel attempts got a check, want 5", got)
	}
	close(hold)
	wg.Wait()
	// Five failures are free; the sixth attempt is let in and locks the key.
	if w := l.Begin("k"); w != 0 {
		t.Fatalf("sixth attempt waits %v", w)
	}
	if w := l.Begin("k"); w == 0 {
		t.Fatal("a second attempt got in beside the sixth")
	}
	l.End("k", true)
	if w := l.Begin("k"); w < 30*time.Second {
		t.Fatalf("after six failures wait = %v, want the lock", w)
	}
}

func TestLimiterEndWithoutFailure(t *testing.T) {
	l := NewLimiter(2, time.Minute, time.Hour, time.Hour)
	for range 10 {
		if w := l.Begin("k"); w != 0 {
			t.Fatalf("a successful attempt was counted: wait %v", w)
		}
		l.End("k", false)
	}
	if len(l.keys) != 0 {
		t.Fatalf("keys left behind: %d", len(l.keys))
	}
	// Success while another attempt is in flight doesn't break its End.
	l.Begin("k")
	l.Success("k")
	l.End("k", true)
	if l.keys["k"] == nil || l.keys["k"].fails != 1 {
		t.Fatal("a failure after Success wasn't recorded")
	}
}
