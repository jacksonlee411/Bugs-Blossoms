package orgui

import (
	"testing"
	"time"
)

func TestFormatValidDate(t *testing.T) {
	got := formatValidDate(time.Date(2025, 12, 27, 15, 4, 5, 0, time.UTC))
	if got != "2025-12-27" {
		t.Fatalf("unexpected date: %q", got)
	}
}

func TestFormatValidEndDateFromEndDate(t *testing.T) {
	t.Run("open-ended", func(t *testing.T) {
		endDate := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
		if !openEndedEndDate(endDate) {
			t.Fatalf("expected openEndedEndDate=true")
		}
		if got := formatValidEndDateFromEndDate(endDate); got != "9999-12-31" {
			t.Fatalf("unexpected end date: %q", got)
		}
	})

	t.Run("midnight", func(t *testing.T) {
		endDate := time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC)
		if openEndedEndDate(endDate) {
			t.Fatalf("expected openEndedEndDate=false")
		}
		if got := formatValidEndDateFromEndDate(endDate); got != "2025-12-28" {
			t.Fatalf("unexpected end date: %q", got)
		}
	})

	t.Run("exclusive-non-midnight", func(t *testing.T) {
		endDate := time.Date(2025, 12, 27, 12, 34, 56, 0, time.UTC)
		if got := formatValidEndDateFromEndDate(endDate); got != "2025-12-27" {
			t.Fatalf("unexpected end date: %q", got)
		}
	})
}

func TestValidTimeIncludesDay(t *testing.T) {
	start := time.Date(2025, 12, 1, 12, 0, 0, 0, time.UTC)
	end := time.Date(2025, 12, 31, 23, 59, 0, 0, time.UTC)

	t.Run("inside", func(t *testing.T) {
		asOf := time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC)
		if !validTimeIncludesDay(asOf, start, end) {
			t.Fatalf("expected true")
		}
	})

	t.Run("start-inclusive", func(t *testing.T) {
		asOf := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
		if !validTimeIncludesDay(asOf, start, end) {
			t.Fatalf("expected true")
		}
	})

	t.Run("end-inclusive", func(t *testing.T) {
		asOf := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
		if !validTimeIncludesDay(asOf, start, end) {
			t.Fatalf("expected true")
		}
	})

	t.Run("after-end", func(t *testing.T) {
		asOf := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		if validTimeIncludesDay(asOf, start, end) {
			t.Fatalf("expected false")
		}
	})

	t.Run("open-ended", func(t *testing.T) {
		openEnded := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
		asOf := time.Date(2050, 1, 1, 0, 0, 0, 0, time.UTC)
		if !validTimeIncludesDay(asOf, start, openEnded) {
			t.Fatalf("expected true")
		}
	})
}
