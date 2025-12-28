package services

import "time"

func normalizeValidDateUTC(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	u := t.UTC()
	y, m, d := u.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func truncateEndDateFromNewEffectiveDate(newEffectiveDate time.Time) time.Time {
	if newEffectiveDate.IsZero() {
		return newEffectiveDate
	}
	return normalizeValidDateUTC(newEffectiveDate).AddDate(0, 0, -1)
}
