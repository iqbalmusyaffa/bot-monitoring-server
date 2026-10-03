package security

import (
	"sync"
	"time"
)

const (
	// RateLimitExceededMessage is returned when a user exceeds the command rate limit.
	RateLimitExceededMessage = `⚠️ Too many requests.
Please try again later.`
)

type userRateRecord struct {
	timestamps []time.Time
}

// RateLimiter manages per-user sliding window rate limiting in-memory.
type RateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	users  map[int64]*userRateRecord
}

// NewRateLimiter creates a new thread-safe in-memory rate limiter.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	if limit <= 0 {
		limit = 10
	}
	if window <= 0 {
		window = 1 * time.Minute
	}

	rl := &RateLimiter{
		limit:  limit,
		window: window,
		users:  make(map[int64]*userRateRecord),
	}

	return rl
}

// Allow checks if the request from userID is permitted under the rate limit.
func (rl *RateLimiter) Allow(userID int64) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	rec, exists := rl.users[userID]
	if !exists {
		rec = &userRateRecord{
			timestamps: []time.Time{now},
		}
		rl.users[userID] = rec
		return true
	}

	// Filter out timestamps outside the sliding window
	validTimestamps := rec.timestamps[:0]
	for _, ts := range rec.timestamps {
		if ts.After(cutoff) {
			validTimestamps = append(validTimestamps, ts)
		}
	}
	rec.timestamps = validTimestamps

	if len(rec.timestamps) >= rl.limit {
		return false
	}

	rec.timestamps = append(rec.timestamps, now)
	return true
}

// PruneStale cleans up memory for users inactive longer than the window.
func (rl *RateLimiter) PruneStale() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-rl.window)
	for id, rec := range rl.users {
		var hasActive bool
		for _, ts := range rec.timestamps {
			if ts.After(cutoff) {
				hasActive = true
				break
			}
		}
		if !hasActive {
			delete(rl.users, id)
		}
	}
}
