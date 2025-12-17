package persistence

import (
	"context"

	"github.com/google/uuid"
	"github.com/iota-uz/iota-sdk/modules/superadmin/domain"
	"github.com/iota-uz/iota-sdk/modules/superadmin/domain/entities"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"
)

type pgTenantAuthSettingsRepository struct{}

func NewPgTenantAuthSettingsRepository() domain.TenantAuthSettingsRepository {
	return &pgTenantAuthSettingsRepository{}
}

func (r *pgTenantAuthSettingsRepository) GetByTenantID(ctx context.Context, tenantID uuid.UUID) (*entities.TenantAuthSettings, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get transaction")
	}

	var s entities.TenantAuthSettings
	err = tx.QueryRow(ctx, `
		SELECT tenant_id, identity_mode, allow_password, allow_google, allow_sso, updated_at
		FROM tenant_auth_settings
		WHERE tenant_id = $1
	`, tenantID).Scan(
		&s.TenantID,
		&s.IdentityMode,
		&s.AllowPassword,
		&s.AllowGoogle,
		&s.AllowSSO,
		&s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrTenantAuthSettingsNotFound
		}
		return nil, errors.Wrap(err, "failed to get tenant auth settings")
	}
	return &s, nil
}

func (r *pgTenantAuthSettingsRepository) Upsert(ctx context.Context, settings *entities.TenantAuthSettings) (*entities.TenantAuthSettings, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get transaction")
	}

	var s entities.TenantAuthSettings
	err = tx.QueryRow(ctx, `
		INSERT INTO tenant_auth_settings (tenant_id, identity_mode, allow_password, allow_google, allow_sso, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (tenant_id) DO UPDATE
		SET
			identity_mode = EXCLUDED.identity_mode,
			allow_password = EXCLUDED.allow_password,
			allow_google = EXCLUDED.allow_google,
			allow_sso = EXCLUDED.allow_sso,
			updated_at = now()
		RETURNING tenant_id, identity_mode, allow_password, allow_google, allow_sso, updated_at
	`,
		settings.TenantID,
		settings.IdentityMode,
		settings.AllowPassword,
		settings.AllowGoogle,
		settings.AllowSSO,
	).Scan(
		&s.TenantID,
		&s.IdentityMode,
		&s.AllowPassword,
		&s.AllowGoogle,
		&s.AllowSSO,
		&s.UpdatedAt,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to upsert tenant auth settings")
	}
	return &s, nil
}
