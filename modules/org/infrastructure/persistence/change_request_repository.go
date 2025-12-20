package persistence

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/iota-uz/iota-sdk/modules/org/domain/changerequest"
	changerequests_sqlc "github.com/iota-uz/iota-sdk/modules/org/infrastructure/sqlc/changerequests"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/jackc/pgx/v5/pgtype"
)

type ChangeRequestRepository struct{}

func NewChangeRequestRepository() changerequest.Repository {
	return &ChangeRequestRepository{}
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func pgUUIDArray(ids []uuid.UUID) pgtype.FlatArray[uuid.UUID] {
	return pgtype.FlatArray[uuid.UUID](ids)
}

func asUUID(v pgtype.UUID) uuid.UUID {
	if !v.Valid {
		return uuid.Nil
	}
	return uuid.UUID(v.Bytes)
}

func asTime(v pgtype.Timestamptz) time.Time {
	if !v.Valid {
		return time.Time{}
	}
	return v.Time
}

func (r *ChangeRequestRepository) Upsert(ctx context.Context, cr *changerequest.ChangeRequest) (*changerequest.ChangeRequest, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		return nil, err
	}

	q := changerequests_sqlc.New(tx)
	row, err := q.UpsertOrgChangeRequest(ctx, changerequests_sqlc.UpsertOrgChangeRequestParams{
		TenantID:             pgUUID(tenantID),
		RequestID:            cr.RequestID,
		RequesterID:          pgUUID(cr.RequesterID),
		Status:               cr.Status,
		PayloadSchemaVersion: cr.PayloadSchemaVersion,
		Payload:              []byte(cr.Payload),
		Notes:                cr.Notes,
	})
	if err != nil {
		return nil, err
	}

	return &changerequest.ChangeRequest{
		TenantID:             asUUID(row.TenantID),
		ID:                   asUUID(row.ID),
		RequestID:            row.RequestID,
		RequesterID:          asUUID(row.RequesterID),
		Status:               row.Status,
		PayloadSchemaVersion: row.PayloadSchemaVersion,
		Payload:              json.RawMessage(row.Payload),
		Notes:                row.Notes,
		CreatedAt:            asTime(row.CreatedAt),
		UpdatedAt:            asTime(row.UpdatedAt),
	}, nil
}

func (r *ChangeRequestRepository) GetByRequestID(ctx context.Context, requestID string) (*changerequest.ChangeRequest, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		return nil, err
	}

	q := changerequests_sqlc.New(tx)
	row, err := q.GetOrgChangeRequestByRequestID(ctx, changerequests_sqlc.GetOrgChangeRequestByRequestIDParams{
		TenantID:  pgUUID(tenantID),
		RequestID: requestID,
	})
	if err != nil {
		return nil, err
	}

	return &changerequest.ChangeRequest{
		TenantID:             asUUID(row.TenantID),
		ID:                   asUUID(row.ID),
		RequestID:            row.RequestID,
		RequesterID:          asUUID(row.RequesterID),
		Status:               row.Status,
		PayloadSchemaVersion: row.PayloadSchemaVersion,
		Payload:              json.RawMessage(row.Payload),
		Notes:                row.Notes,
		CreatedAt:            asTime(row.CreatedAt),
		UpdatedAt:            asTime(row.UpdatedAt),
	}, nil
}

func (r *ChangeRequestRepository) GetByID(ctx context.Context, id uuid.UUID) (*changerequest.ChangeRequest, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		return nil, err
	}

	q := changerequests_sqlc.New(tx)
	row, err := q.GetOrgChangeRequestByID(ctx, changerequests_sqlc.GetOrgChangeRequestByIDParams{
		TenantID: pgUUID(tenantID),
		ID:       pgUUID(id),
	})
	if err != nil {
		return nil, err
	}

	return &changerequest.ChangeRequest{
		TenantID:             asUUID(row.TenantID),
		ID:                   asUUID(row.ID),
		RequestID:            row.RequestID,
		RequesterID:          asUUID(row.RequesterID),
		Status:               row.Status,
		PayloadSchemaVersion: row.PayloadSchemaVersion,
		Payload:              json.RawMessage(row.Payload),
		Notes:                row.Notes,
		CreatedAt:            asTime(row.CreatedAt),
		UpdatedAt:            asTime(row.UpdatedAt),
	}, nil
}

func (r *ChangeRequestRepository) UpdateDraftByID(ctx context.Context, id uuid.UUID, payload []byte, notes *string) (*changerequest.ChangeRequest, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		return nil, err
	}

	q := changerequests_sqlc.New(tx)
	row, err := q.UpdateOrgChangeRequestDraftByID(ctx, changerequests_sqlc.UpdateOrgChangeRequestDraftByIDParams{
		TenantID: pgUUID(tenantID),
		ID:       pgUUID(id),
		Payload:  payload,
		Notes:    notes,
	})
	if err != nil {
		return nil, err
	}

	return &changerequest.ChangeRequest{
		TenantID:             asUUID(row.TenantID),
		ID:                   asUUID(row.ID),
		RequestID:            row.RequestID,
		RequesterID:          asUUID(row.RequesterID),
		Status:               row.Status,
		PayloadSchemaVersion: row.PayloadSchemaVersion,
		Payload:              json.RawMessage(row.Payload),
		Notes:                row.Notes,
		CreatedAt:            asTime(row.CreatedAt),
		UpdatedAt:            asTime(row.UpdatedAt),
	}, nil
}

func (r *ChangeRequestRepository) UpdateStatusByID(ctx context.Context, id uuid.UUID, status string) (*changerequest.ChangeRequest, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		return nil, err
	}

	q := changerequests_sqlc.New(tx)
	row, err := q.UpdateOrgChangeRequestStatusByID(ctx, changerequests_sqlc.UpdateOrgChangeRequestStatusByIDParams{
		TenantID: pgUUID(tenantID),
		ID:       pgUUID(id),
		Status:   status,
	})
	if err != nil {
		return nil, err
	}

	return &changerequest.ChangeRequest{
		TenantID:             asUUID(row.TenantID),
		ID:                   asUUID(row.ID),
		RequestID:            row.RequestID,
		RequesterID:          asUUID(row.RequesterID),
		Status:               row.Status,
		PayloadSchemaVersion: row.PayloadSchemaVersion,
		Payload:              json.RawMessage(row.Payload),
		Notes:                row.Notes,
		CreatedAt:            asTime(row.CreatedAt),
		UpdatedAt:            asTime(row.UpdatedAt),
	}, nil
}

func (r *ChangeRequestRepository) List(ctx context.Context, status string, limit int, cursorUpdatedAt *time.Time, cursorID *uuid.UUID) ([]*changerequest.ChangeRequest, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		return nil, err
	}

	var cursorAt pgtype.Timestamptz
	if cursorUpdatedAt != nil && !cursorUpdatedAt.IsZero() {
		cursorAt = pgtype.Timestamptz{Time: cursorUpdatedAt.UTC(), Valid: true}
	}
	var cursorUUID pgtype.UUID
	if cursorID != nil && *cursorID != uuid.Nil {
		cursorUUID = pgUUID(*cursorID)
	}

	q := changerequests_sqlc.New(tx)
	rows, err := q.ListOrgChangeRequests(ctx, changerequests_sqlc.ListOrgChangeRequestsParams{
		TenantID: pgUUID(tenantID),
		Column2:  status,
		Column3:  cursorAt,
		Column4:  cursorUUID,
		Limit:    int32(limit),
	})
	if err != nil {
		return nil, err
	}

	out := make([]*changerequest.ChangeRequest, 0, len(rows))
	for _, row := range rows {
		out = append(out, &changerequest.ChangeRequest{
			TenantID:             asUUID(row.TenantID),
			ID:                   asUUID(row.ID),
			RequestID:            row.RequestID,
			RequesterID:          asUUID(row.RequesterID),
			Status:               row.Status,
			PayloadSchemaVersion: row.PayloadSchemaVersion,
			Payload:              json.RawMessage(row.Payload),
			Notes:                row.Notes,
			CreatedAt:            asTime(row.CreatedAt),
			UpdatedAt:            asTime(row.UpdatedAt),
		})
	}
	return out, nil
}
