package handlers

import "time"

// addMonthsClamped adds the given number of months to date, clamping the
// day-of-month to the last day of the target month when necessary
// (e.g. Jan 31 + 1 month → Feb 28).
func addMonthsClamped(date time.Time, months int) time.Time {
	if months == 0 {
		return date
	}
	targetFirst := time.Date(date.Year(), date.Month()+time.Month(months), 1, 0, 0, 0, 0, date.Location())
	targetLastDay := targetFirst.AddDate(0, 1, -1).Day()
	day := date.Day()
	if day > targetLastDay {
		day = targetLastDay
	}
	return time.Date(targetFirst.Year(), targetFirst.Month(), day, 0, 0, 0, 0, date.Location())
}

// monthsBetween returns the calendar-month difference between two times
// (year delta × 12 + month delta).
func monthsBetween(start, end time.Time) int {
	years := end.Year() - start.Year()
	months := int(end.Month()) - int(start.Month())
	return years*12 + months
}
