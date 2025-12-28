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

func pgValidDate(t time.Time) pgtype.Date {
	return pgDateOnlyUTC(t)
}
