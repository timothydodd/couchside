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
	fails int
	last  time.Time
	until time.Time
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
	if now.Sub(s.last) > l.Window {
		delete(l.keys, key)
		return 0
	}
	if d := s.until.Sub(now); d > 0 {
		return d
	}
	return 0
}

// Fail records a failed attempt and returns the new lock time.
func (l *Limiter) Fail(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	s := l.keys[key]
	if s == nil || now.Sub(s.last) > l.Window {
		s = &strike{}
		l.keys[key] = s
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
		if now.Sub(s.last) > l.Window {
			delete(l.keys, k)
		}
	}
}
