// internal/utils/period.go

package utils

import "time"

func PeriodRange(period string) (time.Time, time.Time) {
	now := time.Now()

	switch period {
	case "week":
		start := now.AddDate(0, 0, -7)
		return start, now

	case "year":
		start := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		return start, now

	default: // month
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		return start, now
	}
}
func PreviousPeriod(period string, start time.Time) (time.Time, time.Time) {
	switch period {
	case "week":
		return start.AddDate(0, 0, -7), start

	case "year":
		prevYear := start.AddDate(-1, 0, 0)
		return prevYear, start

	default: // month
		prevMonth := start.AddDate(0, -1, 0)
		return prevMonth, start
	}
}
