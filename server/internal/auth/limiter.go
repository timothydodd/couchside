package auth

import (
	"sync"
	"time"
)

// Limiter slows down password guessing. Each key (an IP, or an account name)
// gets Free failed attempts; after that every failure locks the key for a
// delay that doubles from Base up to Max. Failures are forgotten after Window
// without one, and a successful sign-in clears them.
type Limiter struct {
	Free   int
	Base   time.Duration
	Max    time.Duration
	Window time.Duration

	mu   sync.Mutex
	keys map[string]*strike
	now  func() time.Time
}

type strike struct {
	fails   int
	pending int // attempts begun and not yet ended
	last    time.Time
	until   time.Time
}

func NewLimiter(free int, base, max, window time.Duration) *Limiter {
	return &Limiter{Free: free, Base: base, Max: max, Window: window, keys: map[string]*strike{}, now: time.Now}
}

// Wait is how long the key must wait before trying again (0 = go ahead).
func (l *Limiter) Wait(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.keys[key]
	if s == nil {
		return 0
	}
	now := l.now()
	if s.pending == 0 && now.Sub(s.last) > l.Window {
		delete(l.keys, key)
		return 0
	}
	if d := s.until.Sub(now); d > 0 {
		return d
	}
	return 0
}

// Begin claims an attempt for the key and returns 0, or returns how long to
// wait. Every claim must be given back with End. Checking with Wait and
// recording with Fail afterwards lets a burst of parallel requests all pass
// the check before any has failed; Begin counts the attempts in flight, so a
// burst gets no more guesses than the same requests one after another.
func (l *Limiter) Begin(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	s := l.keys[key]
	if s != nil && s.pending == 0 && now.Sub(s.last) > l.Window {
		delete(l.keys, key)
		s = nil
	}
	if s == nil {
		s = &strike{last: now}
		l.keys[key] = s
		l.prune(now)
	}
	if d := s.until.Sub(now); d > 0 {
		return d
	}
	// Free failures don't lock; past those, one attempt at a time.
	if s.pending >= max(1, l.Free-s.fails) {
		return time.Second
	}
	s.pending++
	return 0
}

// End gives back an attempt claimed with Begin, recording it when it failed.
func (l *Limiter) End(key string, failed bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.keys[key]
	if s == nil { // cleared by Success meanwhile
		if failed {
			l.fail(key)
		}
		return
	}
	if s.pending > 0 {
		s.pending--
	}
	if failed {
		l.fail(key)
	} else if s.pending == 0 && s.fails == 0 {
		delete(l.keys, key)
	}
}

// Fail records a failed attempt and returns the new lock time.
func (l *Limiter) Fail(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.fail(key)
}

// fail is Fail with mu held.
func (l *Limiter) fail(key string) time.Duration {
	now := l.now()
	s := l.keys[key]
	if s == nil {
		s = &strike{}
		l.keys[key] = s
	} else if now.Sub(s.last) > l.Window {
		s.fails, s.until = 0, time.Time{}
	}
	s.fails++
	s.last = now
	if s.fails <= l.Free {
		return 0
	}
	d := l.Base
	for i := l.Free + 1; i < s.fails && d < l.Max; i++ {
		d *= 2
	}
	d = min(d, l.Max)
	s.until = now.Add(d)
	l.prune(now)
	return d
}

// Success forgets a key's failures.
func (l *Limiter) Success(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.keys, key)
}

// prune drops stale keys once the map grows, so a scan from many addresses
// can't grow it forever. Called with mu held.
func (l *Limiter) prune(now time.Time) {
	if len(l.keys) < 10000 {
		return
	}
	for k, s := range l.keys {
		if s.pending == 0 && now.Sub(s.last) > l.Window {
			delete(l.keys, k)
		}
	}
}
