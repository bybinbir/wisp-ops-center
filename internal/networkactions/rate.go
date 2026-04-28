package networkactions

import (
	"sync"
	"time"
)

// RateLimiter is a tiny token-bucket suitable for action throttling.
// Phase 8 only uses it to demonstrate the policy hook; later phases
// can swap in a Redis-backed limiter without touching callers.
type RateLimiter struct {
	mu       sync.Mutex
	capacity int
	tokens   int
	refill   time.Duration
	last     time.Time
}

// NewRateLimiter creates a bucket with the given capacity (max
// burst) and per-token refill interval.
func NewRateLimiter(capacity int, refill time.Duration) *RateLimiter {
	if capacity < 1 {
		capacity = 1
	}
	if refill <= 0 {
		refill = time.Second
	}
	return &RateLimiter{
		capacity: capacity,
		tokens:   capacity,
		refill:   refill,
		last:     time.Now(),
	}
}

// Allow consumes one token. Returns false if the bucket is empty.
func (r *RateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Refill based on elapsed time.
	elapsed := time.Since(r.last)
	if elapsed >= r.refill {
		add := int(elapsed / r.refill)
		r.tokens += add
		if r.tokens > r.capacity {
			r.tokens = r.capacity
		}
		r.last = r.last.Add(time.Duration(add) * r.refill)
	}

	if r.tokens <= 0 {
		return false
	}
	r.tokens--
	return true
}
