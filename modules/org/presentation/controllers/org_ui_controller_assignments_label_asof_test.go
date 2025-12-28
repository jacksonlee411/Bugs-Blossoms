package controllers

import (
	"testing"
	"time"
)

func TestLabelAsOfDayForAssignmentRow(t *testing.T) {
	start := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)

	t.Run("before-row", func(t *testing.T) {
		page := time.Date(2025, 11, 30, 0, 0, 0, 0, time.UTC)
		if got := labelAsOfDayForAssignmentRow(page, start, end); !got.Equal(start) {
			t.Fatalf("expected %s, got %s", start.Format(time.DateOnly), got.Format(time.DateOnly))
		}
	})

	t.Run("start-inclusive", func(t *testing.T) {
		page := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
		if got := labelAsOfDayForAssignmentRow(page, start, end); !got.Equal(page) {
			t.Fatalf("expected %s, got %s", page.Format(time.DateOnly), got.Format(time.DateOnly))
		}
	})

	t.Run("inside", func(t *testing.T) {
		page := time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC)
		if got := labelAsOfDayForAssignmentRow(page, start, end); !got.Equal(page) {
			t.Fatalf("expected %s, got %s", page.Format(time.DateOnly), got.Format(time.DateOnly))
		}
	})

	t.Run("end-inclusive", func(t *testing.T) {
		page := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
		if got := labelAsOfDayForAssignmentRow(page, start, end); !got.Equal(page) {
			t.Fatalf("expected %s, got %s", page.Format(time.DateOnly), got.Format(time.DateOnly))
		}
	})

	t.Run("after-row", func(t *testing.T) {
		page := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		if got := labelAsOfDayForAssignmentRow(page, start, end); !got.Equal(start) {
			t.Fatalf("expected %s, got %s", start.Format(time.DateOnly), got.Format(time.DateOnly))
		}
	})

	t.Run("open-ended", func(t *testing.T) {
		openEnded := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
		page := time.Date(2050, 1, 1, 0, 0, 0, 0, time.UTC)
		if got := labelAsOfDayForAssignmentRow(page, start, openEnded); !got.Equal(page) {
			t.Fatalf("expected %s, got %s", page.Format(time.DateOnly), got.Format(time.DateOnly))
		}
	})
}
