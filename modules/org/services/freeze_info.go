package services

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
)

func (s *OrgService) GetFreezeInfo(ctx context.Context, tenantID uuid.UUID, txTime time.Time) (FreezeCheckResult, error) {
	if tenantID == uuid.Nil {
		return FreezeCheckResult{}, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if txTime.IsZero() {
		txTime = time.Now().UTC()
	}
	return inTx(ctx, tenantID, func(txCtx context.Context) (FreezeCheckResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return FreezeCheckResult{}, err
		}
		return s.freezeCheck(settings, txTime, txTime)
	})
}
