package orgui

import "time"

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
