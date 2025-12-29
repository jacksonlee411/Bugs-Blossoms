package services

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

type EdgeTimelineRow struct {
	EdgeID            uuid.UUID
	ParentNodeID      *uuid.UUID
	ChildNodeID       uuid.UUID
	EffectiveDate     time.Time
	EndDate           time.Time
	ParentNameAtStart *string
	ParentCode        *string
}

func (s *OrgService) ListNodeSlicesTimeline(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID) ([]NodeSliceRow, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if nodeID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "node_id is required", nil)
	}
	return inTx(ctx, tenantID, func(txCtx context.Context) ([]NodeSliceRow, error) {
		return s.repo.ListNodeSlicesTimeline(txCtx, tenantID, nodeID)
	})
}

func (s *OrgService) ListEdgesTimelineAsChild(ctx context.Context, tenantID uuid.UUID, hierarchyType string, childNodeID uuid.UUID) ([]EdgeTimelineRow, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	hierarchyType = strings.TrimSpace(hierarchyType)
	if hierarchyType == "" {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "hierarchy_type is required", nil)
	}
	if hierarchyType != "OrgUnit" {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "unsupported hierarchy_type", nil)
	}
	if childNodeID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "child_node_id is required", nil)
	}
	return inTx(ctx, tenantID, func(txCtx context.Context) ([]EdgeTimelineRow, error) {
		return s.repo.ListEdgesTimelineAsChild(txCtx, tenantID, hierarchyType, childNodeID)
	})
}
