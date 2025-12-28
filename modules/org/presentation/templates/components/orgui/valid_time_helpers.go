package orgui

import "time"

func validTimeIncludesDay(asOf, startDate, endDate time.Time) bool {
	if asOf.IsZero() {
		return false
	}
	if startDate.IsZero() {
		return false
	}
	if endDate.IsZero() {
		return false
	}
	asOfDay := formatValidTimeDay(asOf)
	startDay := formatValidTimeDay(startDate)
	endDay := formatValidTimeDay(endDate)
	if openEndedEndDate(endDay) {
		return !asOfDay.Before(startDay)
	}
	return !asOfDay.Before(startDay) && !asOfDay.After(endDay)
}

func formatValidTimeDay(t time.Time) time.Time {
	u := t.UTC()
	y, m, d := u.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func formatValidDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	u := t.UTC()
	y, m, d := u.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
}

func formatValidEndDateFromEndDate(endDate time.Time) string {
	if endDate.IsZero() {
		return ""
	}
	u := endDate.UTC()
	y, m, d := u.Date()
	if y == 9999 && m == time.December && d == 31 {
		return "9999-12-31"
	}
	return formatValidDate(u)
}

func openEndedEndDate(endDate time.Time) bool {
	u := endDate.UTC()
	y, m, d := u.Date()
	return y == 9999 && m == time.December && d == 31
}
