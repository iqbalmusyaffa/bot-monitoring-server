package security

import (
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(3, 100*time.Millisecond)

	userID := int64(12345)

	// Requests 1, 2, 3 should be allowed
	if !rl.Allow(userID) {
		t.Errorf("expected request 1 to be allowed")
	}
	if !rl.Allow(userID) {
		t.Errorf("expected request 2 to be allowed")
	}
	if !rl.Allow(userID) {
		t.Errorf("expected request 3 to be allowed")
	}

	// Request 4 within window should be blocked
	if rl.Allow(userID) {
		t.Errorf("expected request 4 to be blocked by rate limit")
	}

	// Different user should still be allowed
	if !rl.Allow(99999) {
		t.Errorf("expected other user to be allowed")
	}

	// Wait for window to expire
	time.Sleep(150 * time.Millisecond)

	// Should be allowed again
	if !rl.Allow(userID) {
		t.Errorf("expected request to be allowed after rate window reset")
	}
}
