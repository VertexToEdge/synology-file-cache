package ratelimiter

import (
	"sync"
	"testing"
	"time"
)

func TestLimiter_Allow(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		delays   []time.Duration // delays before each Allow() call
		want     []bool          // expected Allow() results
	}{
		{
			name:     "first call always allowed",
			interval: 100 * time.Millisecond,
			delays:   []time.Duration{0},
			want:     []bool{true},
		},
		{
			name:     "second call immediately after is blocked",
			interval: 100 * time.Millisecond,
			delays:   []time.Duration{0, 0},
			want:     []bool{true, false},
		},
		{
			name:     "call after interval is allowed",
			interval: 50 * time.Millisecond,
			delays:   []time.Duration{0, 60 * time.Millisecond},
			want:     []bool{true, true},
		},
		{
			name:     "multiple rapid calls",
			interval: 100 * time.Millisecond,
			delays:   []time.Duration{0, 0, 0, 0},
			want:     []bool{true, false, false, false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter := New(tt.interval)

			for i, delay := range tt.delays {
				if delay > 0 {
					time.Sleep(delay)
				}

				allowed, waitTime := limiter.Allow()
				if allowed != tt.want[i] {
					t.Errorf("call %d: Allow() = %v, want %v", i, allowed, tt.want[i])
				}

				if !allowed && waitTime <= 0 {
					t.Errorf("call %d: blocked but waitTime = %v, want > 0", i, waitTime)
				}

				if allowed && waitTime != 0 {
					t.Errorf("call %d: allowed but waitTime = %v, want 0", i, waitTime)
				}
			}
		})
	}
}

func TestLimiter_Reset(t *testing.T) {
	interval := 1 * time.Second
	limiter := New(interval)

	// First call - should be allowed
	allowed, _ := limiter.Allow()
	if !allowed {
		t.Fatal("first call should be allowed")
	}

	// Second call immediately - should be blocked
	allowed, _ = limiter.Allow()
	if allowed {
		t.Fatal("second call should be blocked")
	}

	// Reset the limiter
	limiter.Reset()

	// Call after reset - should be allowed immediately
	allowed, _ = limiter.Allow()
	if !allowed {
		t.Fatal("call after reset should be allowed")
	}
}

func TestLimiter_TimeSinceLastAllowed(t *testing.T) {
	limiter := New(100 * time.Millisecond)

	// Before any allowed call, should return max duration
	duration := limiter.TimeSinceLastAllowed()
	if duration < time.Hour {
		t.Errorf("before any call, TimeSinceLastAllowed() = %v, want very large", duration)
	}

	// After an allowed call
	limiter.Allow()
	time.Sleep(50 * time.Millisecond)

	duration = limiter.TimeSinceLastAllowed()
	if duration < 40*time.Millisecond || duration > 100*time.Millisecond {
		t.Errorf("after 50ms, TimeSinceLastAllowed() = %v, want ~50ms", duration)
	}
}

func TestLimiter_Interval(t *testing.T) {
	interval := 42 * time.Second
	limiter := New(interval)

	if got := limiter.Interval(); got != interval {
		t.Errorf("Interval() = %v, want %v", got, interval)
	}
}

func TestLimiter_Concurrent(t *testing.T) {
	interval := 100 * time.Millisecond
	limiter := New(interval)

	var wg sync.WaitGroup
	var mu sync.Mutex
	allowedCount := 0

	// Launch 100 goroutines simultaneously
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed, _ := limiter.Allow()
			if allowed {
				mu.Lock()
				allowedCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Only one should be allowed
	if allowedCount != 1 {
		t.Errorf("concurrent calls: %d allowed, want exactly 1", allowedCount)
	}
}

func TestLimiter_WaitTimeAccuracy(t *testing.T) {
	interval := 100 * time.Millisecond
	limiter := New(interval)

	// First call
	limiter.Allow()

	// Immediate second call
	allowed, waitTime := limiter.Allow()
	if allowed {
		t.Fatal("second call should be blocked")
	}

	// Wait time should be close to interval
	if waitTime < 80*time.Millisecond || waitTime > 110*time.Millisecond {
		t.Errorf("waitTime = %v, want close to %v", waitTime, interval)
	}

	// Wait for half the interval
	time.Sleep(50 * time.Millisecond)

	// Check wait time again
	allowed, waitTime = limiter.Allow()
	if allowed {
		t.Fatal("call after 50ms should still be blocked")
	}

	// Wait time should be about half now
	if waitTime < 30*time.Millisecond || waitTime > 60*time.Millisecond {
		t.Errorf("waitTime after 50ms = %v, want ~50ms", waitTime)
	}
}
