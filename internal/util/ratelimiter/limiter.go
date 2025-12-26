package ratelimiter

import (
	"sync"
	"time"
)

// Limiter provides simple time-based rate limiting.
// It allows one action per interval and is safe for concurrent use.
type Limiter struct {
	mu          sync.Mutex
	interval    time.Duration
	lastAllowed time.Time
}

// New creates a new rate limiter with the specified interval.
// Actions will be rate-limited to at most one per interval.
func New(interval time.Duration) *Limiter {
	return &Limiter{
		interval: interval,
	}
}

// Allow checks if an action is allowed at this time.
// Returns true if allowed (and records this as the last allowed time),
// or false with the remaining wait duration if rate-limited.
func (l *Limiter) Allow() (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	timeSinceLast := now.Sub(l.lastAllowed)

	if timeSinceLast >= l.interval {
		l.lastAllowed = now
		return true, 0
	}

	return false, l.interval - timeSinceLast
}

// Reset clears the limiter state, allowing the next action immediately.
func (l *Limiter) Reset() {
	l.mu.Lock()
	l.lastAllowed = time.Time{}
	l.mu.Unlock()
}

// TimeSinceLastAllowed returns the duration since the last allowed action.
// Returns a very large duration if no action has been allowed yet.
func (l *Limiter) TimeSinceLastAllowed() time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.lastAllowed.IsZero() {
		return time.Duration(1<<63 - 1) // Max duration
	}
	return time.Since(l.lastAllowed)
}

// Interval returns the configured rate limit interval.
func (l *Limiter) Interval() time.Duration {
	return l.interval
}
