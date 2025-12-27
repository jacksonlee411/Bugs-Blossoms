package persistence

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func pgDateOnlyUTC(t time.Time) pgtype.Date {
	if t.IsZero() {
		return pgtype.Date{}
	}
	u := t.UTC()
	y, m, d := u.Date()
	return pgtype.Date{Time: time.Date(y, m, d, 0, 0, 0, 0, time.UTC), Valid: true}
}

func pgEffectiveOnFromEffectiveDate(effectiveDate time.Time) pgtype.Date {
	return pgDateOnlyUTC(effectiveDate)
}

func pgEndOnFromEndDate(endDate time.Time) pgtype.Date {
	if endDate.IsZero() {
		return pgtype.Date{}
	}

	u := endDate.UTC()
	y, m, d := u.Date()
	if y == 9999 && m == time.December && d == 31 {
		return pgtype.Date{Time: time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC), Valid: true}
	}

	return pgDateOnlyUTC(u.Add(-time.Microsecond))
}
