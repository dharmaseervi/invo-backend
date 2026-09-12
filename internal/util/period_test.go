package utils

import (
	"testing"
	"time"
)

func TestWeekMatchesTheTrendWindow(t *testing.T) {
	start, end := PeriodRange("week")
	days := int(end.Sub(start).Hours()/24) + 1
	if days != 7 {
		t.Fatalf("week should cover 7 days, covered %d (%s .. %s)", days, start, end)
	}
	if start.Hour() != 0 || start.Minute() != 0 {
		t.Fatalf("start must be midnight so a date-typed column compares correctly, got %s", start)
	}
}

func TestBoundaryDayIsIncluded(t *testing.T) {
	start, end := PeriodRange("week")
	// An invoice dated exactly on the first day of the window, stored as a DATE and so
	// read back at midnight, must fall inside it.
	boundary := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	if boundary.Before(start) || boundary.After(end) {
		t.Fatalf("invoice dated %s fell outside the window %s .. %s", boundary, start, end)
	}
	today := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, end.Location())
	if today.After(end) {
		t.Fatalf("an invoice dated today fell outside the window")
	}
}

func TestPreviousPeriodDoesNotOverlap(t *testing.T) {
	for _, period := range []string{"week", "month", "year"} {
		start, _ := PeriodRange(period)
		_, prevEnd := PreviousPeriod(period, start)
		if !prevEnd.Before(start) {
			t.Fatalf("%s: previous period ends %s, on or after the current start %s — the boundary day is counted twice",
				period, prevEnd, start)
		}
	}
}
