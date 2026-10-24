// Package ratelimit limits the attempts per key over a sliding window, in
// memory: it assumes a single instance of the server.
package ratelimit

import (
	"sync"
	"time"
)

// pruneEvery is the number of calls between two automatic prunes.
const pruneEvery = 1024

// Limiter allows at most limit attempts per key over a sliding window.
type Limiter struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu    sync.Mutex
	hits  map[string][]time.Time
	calls int
}

// New returns a Limiter of limit attempts per window.
func New(limit int, window time.Duration) *Limiter {
	return &Limiter{limit: limit, window: window, now: time.Now, hits: make(map[string][]time.Time)}
}

// Allow records an attempt for key and reports whether it is within the
// limit. A rejected attempt is not recorded; retryAfter is then the time
// until the oldest attempt leaves the window.
func (l *Limiter) Allow(key string) (allowed bool, retryAfter time.Duration) {
	return AllowAll(Attempt{l, key})
}

// Attempt is a key checked against a limiter.
type Attempt struct {
	Limiter *Limiter
	Key     string
}

// AllowAll records an attempt in every limiter only if all of them allow it,
// e.g. 10 per (IP, account) and 100 per account. retryAfter is the longest
// wait among the limiters that reject it.
func AllowAll(attempts ...Attempt) (allowed bool, retryAfter time.Duration) {
	allowed = true
	for _, a := range attempts {
		if ok, wait := a.Limiter.check(a.Key); !ok {
			allowed = false
			retryAfter = max(retryAfter, wait)
		}
	}
	if allowed {
		for _, a := range attempts {
			a.Limiter.record(a.Key)
		}
	}
	return allowed, retryAfter
}

func (l *Limiter) check(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	hits := l.recent(key, now)
	if len(hits) < l.limit {
		return true, 0
	}
	return false, hits[0].Add(l.window).Sub(now)
}

func (l *Limiter) record(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.hits[key] = append(l.recent(key, now), now)
	l.calls++
	if l.calls%pruneEvery == 0 {
		l.pruneLocked(now)
	}
}

// recent drops the attempts of key older than the window.
func (l *Limiter) recent(key string, now time.Time) []time.Time {
	hits := l.hits[key]
	start := 0
	for start < len(hits) && !hits[start].After(now.Add(-l.window)) {
		start++
	}
	if start == len(hits) {
		delete(l.hits, key)
		return nil
	}
	hits = hits[start:]
	l.hits[key] = hits
	return hits
}

// Prune forgets the keys without an attempt in the window.
func (l *Limiter) Prune() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pruneLocked(l.now())
}

func (l *Limiter) pruneLocked(now time.Time) {
	for key := range l.hits {
		l.recent(key, now)
	}
}

// Len is the number of keys remembered.
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.hits)
}
