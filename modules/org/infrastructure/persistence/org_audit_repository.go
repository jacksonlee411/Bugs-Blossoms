package persistence

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

func (r *OrgRepository) GetOrgSettings(ctx context.Context, tenantID uuid.UUID) (services.OrgSettings, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return services.OrgSettings{}, err
	}

	var mode string
	var graceDays int
	var catalogMode string
	var restrictionsMode string
	var reasonCodeMode string
	err = tx.QueryRow(ctx, `
	SELECT
		freeze_mode,
		freeze_grace_days,
		position_catalog_validation_mode,
		position_restrictions_validation_mode,
		reason_code_mode
	FROM org_settings
	WHERE tenant_id=$1
	`, pgUUID(tenantID)).Scan(&mode, &graceDays, &catalogMode, &restrictionsMode, &reasonCodeMode)
	if err == pgx.ErrNoRows {
		return services.OrgSettings{
			FreezeMode:                         "enforce",
			FreezeGraceDays:                    3,
			PositionCatalogValidationMode:      "shadow",
			PositionRestrictionsValidationMode: "shadow",
			ReasonCodeMode:                     "shadow",
		}, nil
	}
	if err != nil {
		return services.OrgSettings{}, err
	}
	return services.OrgSettings{
		FreezeMode:                         mode,
		FreezeGraceDays:                    graceDays,
		PositionCatalogValidationMode:      catalogMode,
		PositionRestrictionsValidationMode: restrictionsMode,
		ReasonCodeMode:                     reasonCodeMode,
	}, nil
}

func (r *OrgRepository) InsertAuditLog(ctx context.Context, tenantID uuid.UUID, log services.AuditLogInsert) (uuid.UUID, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return uuid.Nil, err
	}

	if err := log.Validate(); err != nil {
		return uuid.Nil, err
	}

	oldValuesJSON, oldValid, err := log.MarshalOldValues()
	if err != nil {
		return uuid.Nil, err
	}
	newValuesJSON, err := log.MarshalNewValues()
	if err != nil {
		return uuid.Nil, err
	}
	metaJSON, err := log.MarshalMeta()
	if err != nil {
		return uuid.Nil, err
	}

	var id uuid.UUID
	var old pgtype.Text
	if oldValid {
		old = pgtype.Text{String: oldValuesJSON, Valid: true}
	}

	if err := tx.QueryRow(ctx, `
		INSERT INTO org_audit_logs (
			tenant_id,
			request_id,
			transaction_time,
			initiator_id,
			change_type,
			entity_type,
			entity_id,
			effective_date,
			end_date,
			old_values,
			new_values,
			meta
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12::jsonb)
		RETURNING id
		`,
		pgUUID(tenantID),
		log.RequestID,
		log.TransactionTime.UTC(),
		pgUUID(log.InitiatorID),
		log.ChangeType,
		log.EntityType,
		pgUUID(log.EntityID),
		pgValidDate(log.EffectiveDate),
		pgValidDate(log.EndDate),
		old,
		newValuesJSON,
		metaJSON,
	).Scan(&id); err != nil {
		return uuid.Nil, err
	}

	return id, nil
}
