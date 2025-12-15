package outbox

import (
	"errors"
	"testing"
)

func TestTruncateError(t *testing.T) {
	t.Parallel()

	if got := truncateError(nil, 10); got != "" {
		t.Fatalf("expected empty for nil error, got %q", got)
	}

	err := errors.New("hello world")
	if got := truncateError(err, 5); got != "hello" {
		t.Fatalf("expected %q, got %q", "hello", got)
	}
}
