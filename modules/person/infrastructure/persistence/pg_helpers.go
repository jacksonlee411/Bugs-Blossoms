package persistence

import "github.com/jackc/pgx/v5/pgtype"

func pgUUIDFromUUID(id [16]byte) pgtype.UUID {
	return pgtype.UUID{
		Bytes: id,
		Valid: true,
	}
}
