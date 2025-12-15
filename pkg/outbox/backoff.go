package outbox

import (
	"math"
	"math/rand"
	"time"
)

func backoff(attempts int, maxBackoff time.Duration) time.Duration {
	if attempts <= 0 {
		return 0
	}
	// 1s * 2^(attempts-1)
	seconds := math.Pow(2, float64(attempts-1))
	d := time.Duration(seconds * float64(time.Second))
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}

func jitter(r *rand.Rand, maxJitter time.Duration) time.Duration {
	if maxJitter <= 0 {
		return 0
	}
	if r == nil {
		return 0
	}
	// [0, maxJitter]
	return time.Duration(r.Int63n(int64(maxJitter) + 1)) //nolint:gosec
}
