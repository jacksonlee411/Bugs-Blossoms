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

type pgTenantSSOConnectionsRepository struct{}

func NewPgTenantSSOConnectionsRepository() domain.TenantSSOConnectionsRepository {
	return &pgTenantSSOConnectionsRepository{}
}

func (r *pgTenantSSOConnectionsRepository) ListByTenantID(ctx context.Context, tenantID uuid.UUID) ([]*entities.TenantSSOConnection, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get transaction")
	}

	rows, err := tx.Query(ctx, `
		SELECT
			id,
			tenant_id,
			connection_id,
			display_name,
			protocol,
			enabled,
			jackson_base_url,
			kratos_provider_id,
			saml_metadata_url,
			saml_metadata_xml,
			oidc_issuer,
			oidc_client_id,
			oidc_client_secret_ref,
			last_test_status,
			last_test_error,
			last_test_at,
			created_at,
			updated_at
		FROM tenant_sso_connections
		WHERE tenant_id = $1
		ORDER BY created_at ASC
	`, tenantID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to query tenant sso connections")
	}
	defer rows.Close()

	var out []*entities.TenantSSOConnection
	for rows.Next() {
		var c entities.TenantSSOConnection
		if err := rows.Scan(
			&c.ID,
			&c.TenantID,
			&c.ConnectionID,
			&c.DisplayName,
			&c.Protocol,
			&c.Enabled,
			&c.JacksonBaseURL,
			&c.KratosProviderID,
			&c.SAMLMetadataURL,
			&c.SAMLMetadataXML,
			&c.OIDCIssuer,
			&c.OIDCClientID,
			&c.OIDCClientSecretRef,
			&c.LastTestStatus,
			&c.LastTestError,
			&c.LastTestAt,
			&c.CreatedAt,
			&c.UpdatedAt,
		); err != nil {
			return nil, errors.Wrap(err, "failed to scan tenant sso connection")
		}
		out = append(out, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, "error iterating tenant sso connections")
	}
	return out, nil
}

func (r *pgTenantSSOConnectionsRepository) Create(ctx context.Context, conn *entities.TenantSSOConnection) (*entities.TenantSSOConnection, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get transaction")
	}

	var c entities.TenantSSOConnection
	err = tx.QueryRow(ctx, `
		INSERT INTO tenant_sso_connections (
			tenant_id,
			connection_id,
			display_name,
			protocol,
			enabled,
			jackson_base_url,
			kratos_provider_id,
			saml_metadata_url,
			saml_metadata_xml,
			oidc_issuer,
			oidc_client_id,
			oidc_client_secret_ref,
			created_at,
			updated_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,now(),now())
		RETURNING
			id,
			tenant_id,
			connection_id,
			display_name,
			protocol,
			enabled,
			jackson_base_url,
			kratos_provider_id,
			saml_metadata_url,
			saml_metadata_xml,
			oidc_issuer,
			oidc_client_id,
			oidc_client_secret_ref,
			last_test_status,
			last_test_error,
			last_test_at,
			created_at,
			updated_at
	`,
		conn.TenantID,
		conn.ConnectionID,
		conn.DisplayName,
		conn.Protocol,
		conn.Enabled,
		conn.JacksonBaseURL,
		conn.KratosProviderID,
		conn.SAMLMetadataURL,
		conn.SAMLMetadataXML,
		conn.OIDCIssuer,
		conn.OIDCClientID,
		conn.OIDCClientSecretRef,
	).Scan(
		&c.ID,
		&c.TenantID,
		&c.ConnectionID,
		&c.DisplayName,
		&c.Protocol,
		&c.Enabled,
		&c.JacksonBaseURL,
		&c.KratosProviderID,
		&c.SAMLMetadataURL,
		&c.SAMLMetadataXML,
		&c.OIDCIssuer,
		&c.OIDCClientID,
		&c.OIDCClientSecretRef,
		&c.LastTestStatus,
		&c.LastTestError,
		&c.LastTestAt,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create tenant sso connection")
	}
	return &c, nil
}

func (r *pgTenantSSOConnectionsRepository) GetByID(ctx context.Context, id uuid.UUID) (*entities.TenantSSOConnection, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get transaction")
	}

	var c entities.TenantSSOConnection
	err = tx.QueryRow(ctx, `
		SELECT
			id,
			tenant_id,
			connection_id,
			display_name,
			protocol,
			enabled,
			jackson_base_url,
			kratos_provider_id,
			saml_metadata_url,
			saml_metadata_xml,
			oidc_issuer,
			oidc_client_id,
			oidc_client_secret_ref,
			last_test_status,
			last_test_error,
			last_test_at,
			created_at,
			updated_at
		FROM tenant_sso_connections
		WHERE id = $1
	`, id).Scan(
		&c.ID,
		&c.TenantID,
		&c.ConnectionID,
		&c.DisplayName,
		&c.Protocol,
		&c.Enabled,
		&c.JacksonBaseURL,
		&c.KratosProviderID,
		&c.SAMLMetadataURL,
		&c.SAMLMetadataXML,
		&c.OIDCIssuer,
		&c.OIDCClientID,
		&c.OIDCClientSecretRef,
		&c.LastTestStatus,
		&c.LastTestError,
		&c.LastTestAt,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("sso connection not found")
		}
		return nil, errors.Wrap(err, "failed to get tenant sso connection")
	}
	return &c, nil
}

func (r *pgTenantSSOConnectionsRepository) Update(ctx context.Context, conn *entities.TenantSSOConnection) (*entities.TenantSSOConnection, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get transaction")
	}

	var c entities.TenantSSOConnection
	err = tx.QueryRow(ctx, `
		UPDATE tenant_sso_connections
		SET
			connection_id = $1,
			display_name = $2,
			protocol = $3,
			jackson_base_url = $4,
			kratos_provider_id = $5,
			saml_metadata_url = $6,
			saml_metadata_xml = $7,
			oidc_issuer = $8,
			oidc_client_id = $9,
			oidc_client_secret_ref = $10,
			updated_at = now()
		WHERE id = $11 AND tenant_id = $12
		RETURNING
			id,
			tenant_id,
			connection_id,
			display_name,
			protocol,
			enabled,
			jackson_base_url,
			kratos_provider_id,
			saml_metadata_url,
			saml_metadata_xml,
			oidc_issuer,
			oidc_client_id,
			oidc_client_secret_ref,
			last_test_status,
			last_test_error,
			last_test_at,
			created_at,
			updated_at
	`,
		conn.ConnectionID,
		conn.DisplayName,
		conn.Protocol,
		conn.JacksonBaseURL,
		conn.KratosProviderID,
		conn.SAMLMetadataURL,
		conn.SAMLMetadataXML,
		conn.OIDCIssuer,
		conn.OIDCClientID,
		conn.OIDCClientSecretRef,
		conn.ID,
		conn.TenantID,
	).Scan(
		&c.ID,
		&c.TenantID,
		&c.ConnectionID,
		&c.DisplayName,
		&c.Protocol,
		&c.Enabled,
		&c.JacksonBaseURL,
		&c.KratosProviderID,
		&c.SAMLMetadataURL,
		&c.SAMLMetadataXML,
		&c.OIDCIssuer,
		&c.OIDCClientID,
		&c.OIDCClientSecretRef,
		&c.LastTestStatus,
		&c.LastTestError,
		&c.LastTestAt,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("sso connection not found")
		}
		return nil, errors.Wrap(err, "failed to update tenant sso connection")
	}
	return &c, nil
}

func (r *pgTenantSSOConnectionsRepository) UpdateStatus(ctx context.Context, id uuid.UUID, enabled bool, lastTestStatus, lastTestError *string, lastTestAt *time.Time) (*entities.TenantSSOConnection, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get transaction")
	}

	var c entities.TenantSSOConnection
	err = tx.QueryRow(ctx, `
		UPDATE tenant_sso_connections
		SET
			enabled = $1,
			last_test_status = $2,
			last_test_error = $3,
			last_test_at = $4,
			updated_at = now()
		WHERE id = $5
		RETURNING
			id,
			tenant_id,
			connection_id,
			display_name,
			protocol,
			enabled,
			jackson_base_url,
			kratos_provider_id,
			saml_metadata_url,
			saml_metadata_xml,
			oidc_issuer,
			oidc_client_id,
			oidc_client_secret_ref,
			last_test_status,
			last_test_error,
			last_test_at,
			created_at,
			updated_at
	`,
		enabled,
		lastTestStatus,
		lastTestError,
		lastTestAt,
		id,
	).Scan(
		&c.ID,
		&c.TenantID,
		&c.ConnectionID,
		&c.DisplayName,
		&c.Protocol,
		&c.Enabled,
		&c.JacksonBaseURL,
		&c.KratosProviderID,
		&c.SAMLMetadataURL,
		&c.SAMLMetadataXML,
		&c.OIDCIssuer,
		&c.OIDCClientID,
		&c.OIDCClientSecretRef,
		&c.LastTestStatus,
		&c.LastTestError,
		&c.LastTestAt,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("sso connection not found")
		}
		return nil, errors.Wrap(err, "failed to update tenant sso connection status")
	}
	return &c, nil
}

func (r *pgTenantSSOConnectionsRepository) Delete(ctx context.Context, tenantID uuid.UUID, id uuid.UUID) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get transaction")
	}

	tag, err := tx.Exec(ctx, `
		DELETE FROM tenant_sso_connections
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)
	if err != nil {
		return errors.Wrap(err, "failed to delete tenant sso connection")
	}
	if tag.RowsAffected() == 0 {
		return errors.New("sso connection not found")
	}
	return nil
}
