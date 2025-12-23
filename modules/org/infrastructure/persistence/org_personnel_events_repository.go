package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

func (r *OrgRepository) UpsertPersonnelEvent(ctx context.Context, tenantID uuid.UUID, in services.PersonnelEventInsert) (services.PersonnelEventRow, bool, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.PersonnelEventRow{}, false, err
	}

	requestID := strings.TrimSpace(in.RequestID)
	pernr := strings.TrimSpace(in.Pernr)
	reasonCode := strings.TrimSpace(in.ReasonCode)
	eventType := strings.TrimSpace(in.EventType)
	payload := in.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}

	var row services.PersonnelEventRow
	var initiator pgtype.UUID
	var personUUID pgtype.UUID
	var createdAt pgtype.Timestamptz
	var updatedAt pgtype.Timestamptz
	var payloadOut []byte

	err = tx.QueryRow(ctx, `
INSERT INTO org_personnel_events (
  tenant_id,
  request_id,
  initiator_id,
  event_type,
  person_uuid,
  pernr,
  effective_date,
  reason_code,
  payload
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (tenant_id, request_id) DO NOTHING
RETURNING id, request_id, initiator_id, event_type, person_uuid, pernr, effective_date, reason_code, payload, created_at, updated_at
`, pgUUID(tenantID), requestID, pgUUID(in.InitiatorID), eventType, pgUUID(in.PersonUUID), pernr, in.EffectiveDate, reasonCode, payload).Scan(
		&row.ID,
		&row.RequestID,
		&initiator,
		&row.EventType,
		&personUUID,
		&row.Pernr,
		&row.EffectiveDate,
		&row.ReasonCode,
		&payloadOut,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			existing, err := r.getPersonnelEventByRequestID(ctx, tenantID, requestID)
			return existing, false, err
		}
		return services.PersonnelEventRow{}, false, err
	}

	if initiator.Valid {
		row.InitiatorID = uuid.UUID(initiator.Bytes)
	}
	if personUUID.Valid {
		row.PersonUUID = uuid.UUID(personUUID.Bytes)
	}
	if createdAt.Valid {
		row.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		row.UpdatedAt = updatedAt.Time
	}
	row.Payload = payloadOut

	return row, true, nil
}

func (r *OrgRepository) getPersonnelEventByRequestID(ctx context.Context, tenantID uuid.UUID, requestID string) (services.PersonnelEventRow, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.PersonnelEventRow{}, err
	}

	var row services.PersonnelEventRow
	var initiator pgtype.UUID
	var personUUID pgtype.UUID
	var createdAt pgtype.Timestamptz
	var updatedAt pgtype.Timestamptz
	var payloadOut []byte

	err = tx.QueryRow(ctx, `
SELECT
  id,
  request_id,
  initiator_id,
  event_type,
  person_uuid,
  pernr,
  effective_date,
  reason_code,
  payload,
  created_at,
  updated_at
FROM org_personnel_events
WHERE tenant_id = $1 AND request_id = $2
LIMIT 1
`, pgUUID(tenantID), strings.TrimSpace(requestID)).Scan(
		&row.ID,
		&row.RequestID,
		&initiator,
		&row.EventType,
		&personUUID,
		&row.Pernr,
		&row.EffectiveDate,
		&row.ReasonCode,
		&payloadOut,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return services.PersonnelEventRow{}, err
	}

	if initiator.Valid {
		row.InitiatorID = uuid.UUID(initiator.Bytes)
	}
	if personUUID.Valid {
		row.PersonUUID = uuid.UUID(personUUID.Bytes)
	}
	if createdAt.Valid {
		row.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		row.UpdatedAt = updatedAt.Time
	}
	row.Payload = payloadOut

	return row, nil
}
