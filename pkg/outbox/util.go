package outbox

import (
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func uuidZero() uuid.UUID {
	return uuid.UUID{}
}

func TableLabel(table pgx.Identifier) string {
	if len(table) == 0 {
		return ""
	}
	return strings.Join(table, ".")
}
