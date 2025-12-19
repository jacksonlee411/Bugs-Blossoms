package main

import (
	"fmt"
	"strings"
	"time"
)

func parseDateUTC(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("date is required")
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q (expected YYYY-MM-DD): %w", s, err)
	}
	return t.UTC(), nil
}
