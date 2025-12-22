package persistence

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/pkg/composables"
)

func (r *OrgRepository) ResolvePersonUUIDByPernr(ctx context.Context, tenantID uuid.UUID, pernr string) (uuid.UUID, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return uuid.Nil, err
	}

	var personUUID uuid.UUID
	if err := tx.QueryRow(ctx, `
SELECT person_uuid
FROM persons
WHERE tenant_id = $1 AND pernr = $2
LIMIT 1
`, pgUUID(tenantID), strings.TrimSpace(pernr)).Scan(&personUUID); err != nil {
		return uuid.Nil, err
	}

	return personUUID, nil
}
