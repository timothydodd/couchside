package keylock

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Three callers for one key, arriving while it's held and while others wait,
// never run together; another key isn't held up; nothing is left behind.
func TestOneAtATimePerKey(t *testing.T) {
	var l Map[string]
	var in, peak atomic.Int32
	var wg sync.WaitGroup
	for i := range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(time.Duration(i%5) * time.Millisecond)
			unlock := l.Lock("a")
			if n := in.Add(1); n > peak.Load() {
				peak.Store(n)
			}
			time.Sleep(time.Millisecond)
			in.Add(-1)
			unlock()
		}()
	}
	hold := l.Lock("b")
	other := make(chan struct{})
	go func() { l.Lock("c")(); close(other) }()
	select {
	case <-other:
	case <-time.After(2 * time.Second):
		t.Fatal("a different key waited")
	}
	hold()
	wg.Wait()
	if peak.Load() != 1 {
		t.Fatalf("%d callers held one key at once", peak.Load())
	}
	l.mu.Lock()
	left := len(l.m)
	l.mu.Unlock()
	if left != 0 {
		t.Fatalf("%d entries left behind", left)
	}
}
