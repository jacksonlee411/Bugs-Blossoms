package services

import "time"

func normalizeValidTimeDayUTC(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	u := t.UTC()
	y, m, d := u.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
