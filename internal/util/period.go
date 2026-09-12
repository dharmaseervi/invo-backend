// internal/util/period.go

package utils

import "time"

// startOfDay drops the time component.
//
// Every window below is compared against invoice_date, which is a DATE. A boundary
// carrying a time of day is cast to that instant, so an invoice dated exactly on the
// boundary — midnight — falls before it and is silently dropped from the total. A
// shopkeeper opening the app in the afternoon saw a different week's revenue from one
// opening it at breakfast.
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// endOfDay is the last instant of t's day, so an invoice dated today is inside the
// window regardless of the hour the query runs.
func endOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, t.Location())
}

// PeriodRange is the inclusive date window for a dashboard period.
//
// "week" is the last seven days including today, which is deliberately the same window
// the revenue trend chart draws. They used to differ by a day, so the headline figure
// could show a week's takings above a chart that showed none of them.
func PeriodRange(period string) (time.Time, time.Time) {
	now := time.Now()

	switch period {
	case "week":
		return startOfDay(now.AddDate(0, 0, -6)), endOfDay(now)

	case "year":
		return time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location()), endOfDay(now)

	default: // month
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()), endOfDay(now)
	}
}

// PreviousPeriod is the window immediately before start, for the change percentage.
//
// It ends the day *before* start rather than on it: sharing the boundary day counted
// that day's invoices in both periods, which flattered or punished the comparison by a
// whole day's trade.
func PreviousPeriod(period string, start time.Time) (time.Time, time.Time) {
	previousEnd := endOfDay(start.AddDate(0, 0, -1))

	switch period {
	case "week":
		return startOfDay(start.AddDate(0, 0, -7)), previousEnd

	case "year":
		return startOfDay(start.AddDate(-1, 0, 0)), previousEnd

	default: // month
		return startOfDay(start.AddDate(0, -1, 0)), previousEnd
	}
}
