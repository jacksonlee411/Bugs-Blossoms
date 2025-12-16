package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/iota-uz/iota-sdk/modules/superadmin/domain/entities"
)

type TenantDomainsRepository interface {
	ListByTenantID(ctx context.Context, tenantID uuid.UUID) ([]*entities.TenantDomain, error)
	Create(ctx context.Context, domain *entities.TenantDomain) (*entities.TenantDomain, error)
	GetByID(ctx context.Context, id uuid.UUID) (*entities.TenantDomain, error)
	UpdateVerification(ctx context.Context, id uuid.UUID, attemptedAt time.Time, verifiedAt *time.Time, lastError *string) (*entities.TenantDomain, error)
	SetPrimary(ctx context.Context, tenantID, domainID uuid.UUID) error
	Delete(ctx context.Context, tenantID, domainID uuid.UUID) error
}

type TenantAuthSettingsRepository interface {
	GetByTenantID(ctx context.Context, tenantID uuid.UUID) (*entities.TenantAuthSettings, error)
	Upsert(ctx context.Context, settings *entities.TenantAuthSettings) (*entities.TenantAuthSettings, error)
}

type TenantSSOConnectionsRepository interface {
	ListByTenantID(ctx context.Context, tenantID uuid.UUID) ([]*entities.TenantSSOConnection, error)
	Create(ctx context.Context, conn *entities.TenantSSOConnection) (*entities.TenantSSOConnection, error)
	GetByID(ctx context.Context, id uuid.UUID) (*entities.TenantSSOConnection, error)
	Update(ctx context.Context, conn *entities.TenantSSOConnection) (*entities.TenantSSOConnection, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, enabled bool, lastTestStatus, lastTestError *string, lastTestAt *time.Time) (*entities.TenantSSOConnection, error)
	Delete(ctx context.Context, tenantID, id uuid.UUID) error
}

type SuperadminAuditLogRepository interface {
	ListByTenantID(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]*entities.SuperadminAuditLog, int, error)
}
