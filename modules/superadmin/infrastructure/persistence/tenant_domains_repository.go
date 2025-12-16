package persistence

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/iota-uz/iota-sdk/modules/superadmin/domain"
	"github.com/iota-uz/iota-sdk/modules/superadmin/domain/entities"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"
)

type pgTenantDomainsRepository struct{}

func NewPgTenantDomainsRepository() domain.TenantDomainsRepository {
	return &pgTenantDomainsRepository{}
}

func (r *pgTenantDomainsRepository) ListByTenantID(ctx context.Context, tenantID uuid.UUID) ([]*entities.TenantDomain, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get transaction")
	}

	rows, err := tx.Query(ctx, `
		SELECT
			id,
			tenant_id,
			hostname,
			is_primary,
			verification_token,
			last_verification_attempt_at,
			last_verification_error,
			verified_at,
			created_at,
			updated_at
		FROM tenant_domains
		WHERE tenant_id = $1
		ORDER BY is_primary DESC, created_at ASC
	`, tenantID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to query tenant domains")
	}
	defer rows.Close()

	var out []*entities.TenantDomain
	for rows.Next() {
		var d entities.TenantDomain
		if err := rows.Scan(
			&d.ID,
			&d.TenantID,
			&d.Hostname,
			&d.IsPrimary,
			&d.VerificationToken,
			&d.LastVerificationAttemptAt,
			&d.LastVerificationError,
			&d.VerifiedAt,
			&d.CreatedAt,
			&d.UpdatedAt,
		); err != nil {
			return nil, errors.Wrap(err, "failed to scan tenant domain")
		}
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, "error iterating tenant domains")
	}
	return out, nil
}

func (r *pgTenantDomainsRepository) Create(ctx context.Context, domainRow *entities.TenantDomain) (*entities.TenantDomain, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get transaction")
	}

	var d entities.TenantDomain
	err = tx.QueryRow(ctx, `
		INSERT INTO tenant_domains (
			tenant_id,
			hostname,
			is_primary,
			verification_token,
			created_at,
			updated_at
		)
		VALUES ($1, $2, false, $3, now(), now())
		RETURNING
			id,
			tenant_id,
			hostname,
			is_primary,
			verification_token,
			last_verification_attempt_at,
			last_verification_error,
			verified_at,
			created_at,
			updated_at
	`,
		domainRow.TenantID,
		domainRow.Hostname,
		domainRow.VerificationToken,
	).Scan(
		&d.ID,
		&d.TenantID,
		&d.Hostname,
		&d.IsPrimary,
		&d.VerificationToken,
		&d.LastVerificationAttemptAt,
		&d.LastVerificationError,
		&d.VerifiedAt,
		&d.CreatedAt,
		&d.UpdatedAt,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create tenant domain")
	}
	return &d, nil
}

func (r *pgTenantDomainsRepository) GetByID(ctx context.Context, id uuid.UUID) (*entities.TenantDomain, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get transaction")
	}

	var d entities.TenantDomain
	err = tx.QueryRow(ctx, `
		SELECT
			id,
			tenant_id,
			hostname,
			is_primary,
			verification_token,
			last_verification_attempt_at,
			last_verification_error,
			verified_at,
			created_at,
			updated_at
		FROM tenant_domains
		WHERE id = $1
	`, id).Scan(
		&d.ID,
		&d.TenantID,
		&d.Hostname,
		&d.IsPrimary,
		&d.VerificationToken,
		&d.LastVerificationAttemptAt,
		&d.LastVerificationError,
		&d.VerifiedAt,
		&d.CreatedAt,
		&d.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("domain not found")
		}
		return nil, errors.Wrap(err, "failed to get tenant domain")
	}
	return &d, nil
}

func (r *pgTenantDomainsRepository) UpdateVerification(ctx context.Context, id uuid.UUID, attemptedAt time.Time, verifiedAt *time.Time, lastError *string) (*entities.TenantDomain, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get transaction")
	}

	var d entities.TenantDomain
	err = tx.QueryRow(ctx, `
		UPDATE tenant_domains
		SET
			last_verification_attempt_at = $1,
			last_verification_error = $2,
			verified_at = $3,
			updated_at = now()
		WHERE id = $4
		RETURNING
			id,
			tenant_id,
			hostname,
			is_primary,
			verification_token,
			last_verification_attempt_at,
			last_verification_error,
			verified_at,
			created_at,
			updated_at
	`,
		attemptedAt,
		lastError,
		verifiedAt,
		id,
	).Scan(
		&d.ID,
		&d.TenantID,
		&d.Hostname,
		&d.IsPrimary,
		&d.VerificationToken,
		&d.LastVerificationAttemptAt,
		&d.LastVerificationError,
		&d.VerifiedAt,
		&d.CreatedAt,
		&d.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("domain not found")
		}
		return nil, errors.Wrap(err, "failed to update tenant domain verification")
	}
	return &d, nil
}

func (r *pgTenantDomainsRepository) SetPrimary(ctx context.Context, tenantID, domainID uuid.UUID) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get transaction")
	}

	if _, err := tx.Exec(ctx, `
		UPDATE tenant_domains
		SET is_primary = false, updated_at = now()
		WHERE tenant_id = $1 AND is_primary = true
	`, tenantID); err != nil {
		return errors.Wrap(err, "failed to clear primary domain")
	}

	tag, err := tx.Exec(ctx, `
		UPDATE tenant_domains
		SET is_primary = true, updated_at = now()
		WHERE tenant_id = $1 AND id = $2 AND verified_at IS NOT NULL
	`, tenantID, domainID)
	if err != nil {
		return errors.Wrap(err, "failed to set primary domain")
	}
	if tag.RowsAffected() == 0 {
		return errors.New("domain not found or not verified")
	}

	_, err = tx.Exec(ctx, `
		UPDATE tenants
		SET domain = (SELECT hostname FROM tenant_domains WHERE tenant_id = $1 AND id = $2),
			updated_at = now()
		WHERE id = $1
	`, tenantID, domainID)
	if err != nil {
		return errors.Wrap(err, "failed to sync tenants.domain")
	}
	return nil
}

func (r *pgTenantDomainsRepository) Delete(ctx context.Context, tenantID, domainID uuid.UUID) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get transaction")
	}

	tag, err := tx.Exec(ctx, `
		DELETE FROM tenant_domains
		WHERE tenant_id = $1 AND id = $2 AND is_primary = false
	`, tenantID, domainID)
	if err != nil {
		return errors.Wrap(err, "failed to delete tenant domain")
	}
	if tag.RowsAffected() == 0 {
		return errors.New("domain not found or is primary")
	}
	return nil
}
