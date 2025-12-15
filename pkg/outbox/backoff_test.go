package outbox

import (
	"math/rand"
	"testing"
	"time"
)

func TestBackoff(t *testing.T) {
	t.Parallel()

	maxBackoff := 60 * time.Second
	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{attempts: 0, want: 0},
		{attempts: 1, want: 1 * time.Second},
		{attempts: 2, want: 2 * time.Second},
		{attempts: 3, want: 4 * time.Second},
		{attempts: 7, want: 60 * time.Second}, // cap
	}

	for _, tc := range cases {
		if got := backoff(tc.attempts, maxBackoff); got != tc.want {
			t.Fatalf("attempts=%d: want %s got %s", tc.attempts, tc.want, got)
		}
	}
}

func TestJitterDeterministic(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(1))
	maxJitter := 200 * time.Millisecond

	got := jitter(r, maxJitter)
	if got < 0 || got > maxJitter {
		t.Fatalf("jitter out of range: %s", got)
	}

	r2 := rand.New(rand.NewSource(1))
	if got2 := jitter(r2, maxJitter); got2 != got {
		t.Fatalf("expected deterministic jitter; got %s and %s", got, got2)
	}
}
