package utils

import (
	"fmt"
	"time"
)

func FinancialYear(date time.Time) string {
	year := date.Year()
	month := date.Month()

	if month >= time.April {
		return fmt.Sprintf("FY%02d-%02d", year%100, (year+1)%100)
	}
	return fmt.Sprintf("FY%02d-%02d", (year-1)%100, year%100)
}
