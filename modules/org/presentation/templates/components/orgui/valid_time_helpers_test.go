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
