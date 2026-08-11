// Package ratelimit provides a small in-memory per-IP rate limiter.
//
// State is intentionally in memory: a restart resets limits, which is
// acceptable for the prototype.
package ratelimit

import (
	"sync"
	"time"
)

// Limiter is a fixed-window per-key limiter.
type Limiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	entries map[string]*entry
	lastGC  time.Time
}

type entry struct {
	count       int
	windowStart time.Time
}

// New creates a limiter allowing limit events per window per key.
// A limit of 0 disables rate limiting.
func New(limit int, window time.Duration) *Limiter {
	return &Limiter{
		limit:   limit,
		window:  window,
		entries: make(map[string]*entry),
		lastGC:  time.Now(),
	}
}

// Allow reports whether the key may proceed. If not, it returns the
// number of seconds until the window resets.
func (l *Limiter) Allow(key string) (ok bool, retryAfter int) {
	if l.limit <= 0 {
		return true, 0
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	l.gc(now)

	e, exists := l.entries[key]
	if !exists || now.Sub(e.windowStart) >= l.window {
		e = &entry{count: 1, windowStart: now}
		l.entries[key] = e
		return true, 0
	}
	if e.count < l.limit {
		e.count++
		return true, 0
	}
	remaining := l.window - now.Sub(e.windowStart)
	return false, int(remaining.Seconds()) + 1
}

// gc drops entries whose window has fully elapsed. Called with the
// lock held, at most once per minute.
func (l *Limiter) gc(now time.Time) {
	if now.Sub(l.lastGC) < time.Minute {
		return
	}
	l.lastGC = now
	for k, e := range l.entries {
		if now.Sub(e.windowStart) >= l.window {
			delete(l.entries, k)
		}
	}
}
