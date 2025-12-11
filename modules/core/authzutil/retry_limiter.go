package authzutil

import (
	"sync"
	"time"
)

type retryLimiter struct {
	mu       sync.Mutex
	lastSeen map[string]time.Time
}

var botRetryLimiter = &retryLimiter{
	lastSeen: map[string]time.Time{},
}

// AllowBotRetry enforces a per-request cooldown.
func AllowBotRetry(requestID string, now time.Time, window time.Duration) bool {
	if requestID == "" {
		return false
	}
	botRetryLimiter.mu.Lock()
	defer botRetryLimiter.mu.Unlock()

	if last, ok := botRetryLimiter.lastSeen[requestID]; ok {
		if now.Sub(last) < window {
			return false
		}
	}
	botRetryLimiter.lastSeen[requestID] = now
	return true
}
