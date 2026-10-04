// Package keylock serialises work per key: one fetch per image, one
// conversion per subtitle track, while different keys run side by side.
package keylock

import "sync"

// Map hands out one lock per key. An entry lives while anyone holds or waits
// for it, so a waiter and a newcomer always share the same lock. (Deleting
// the entry on unlock, as a plain sync.Map of mutexes invites, lets a
// newcomer make a fresh lock while a waiter still holds the old one, and the
// two then run together.)
type Map[K comparable] struct {
	mu sync.Mutex
	m  map[K]*entry
}

type entry struct {
	mu   sync.Mutex
	refs int
}

// Lock waits for key's lock and returns the function that releases it.
func (l *Map[K]) Lock(key K) (unlock func()) {
	l.mu.Lock()
	if l.m == nil {
		l.m = map[K]*entry{}
	}
	e := l.m[key]
	if e == nil {
		e = &entry{}
		l.m[key] = e
	}
	e.refs++
	l.mu.Unlock()

	e.mu.Lock()
	return func() {
		e.mu.Unlock()
		l.mu.Lock()
		if e.refs--; e.refs == 0 {
			delete(l.m, key)
		}
		l.mu.Unlock()
	}
}
