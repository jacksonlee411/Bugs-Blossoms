package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/iota-uz/iota-sdk/modules/superadmin/domain"
	"github.com/iota-uz/iota-sdk/modules/superadmin/domain/entities"
)

type SuperadminAuditLogService struct {
	repo domain.SuperadminAuditLogRepository
}

func NewSuperadminAuditLogService(repo domain.SuperadminAuditLogRepository) *SuperadminAuditLogService {
	return &SuperadminAuditLogService{repo: repo}
}

func (s *SuperadminAuditLogService) ListByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]*entities.SuperadminAuditLog, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.ListByTenantID(ctx, tenantID, limit, offset)
}
