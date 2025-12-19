package services

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/pkg/repo"
)

func (s *OrgService) ListAncestorsAsOf(ctx context.Context, tenantID uuid.UUID, orgNodeID uuid.UUID, asOf time.Time) ([]DeepReadRelation, time.Time, error) {
	if tenantID == uuid.Nil {
		return nil, time.Time{}, fmt.Errorf("tenant_id is required")
	}
	if orgNodeID == uuid.Nil {
		return nil, time.Time{}, fmt.Errorf("org_node_id is required")
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}

	const hierarchyType = "OrgUnit"
	cacheKey := repo.CacheKey("org", "ancestors", tenantID, hierarchyType, orgNodeID, orgDateKey(asOf), OrgDeepReadBackendForTenant(tenantID))
	if s != nil && s.cache != nil && OrgCacheEnabled() {
		if cachedAny, ok := s.cache.Get(cacheKey); ok {
			if cached, ok := cachedAny.(cachedRelations); ok {
				return cached.Items, cached.AsOf, nil
			}
		}
	}

	out, err := inTx(ctx, tenantID, func(txCtx context.Context) ([]DeepReadRelation, error) {
		exists, err := s.repo.NodeExistsAt(txCtx, tenantID, orgNodeID, hierarchyType, asOf)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, newServiceError(404, "ORG_NODE_NOT_FOUND_AT_DATE", "org_node_id not found at effective_date", nil)
		}
		return s.repo.ListAncestorsAsOf(txCtx, tenantID, hierarchyType, orgNodeID, asOf, OrgDeepReadBackendForTenant(tenantID))
	})
	if err != nil {
		return nil, time.Time{}, err
	}

	if s != nil && s.cache != nil && OrgCacheEnabled() {
		s.cache.Set(tenantID, cacheKey, cachedRelations{Items: out, AsOf: asOf})
	}
	return out, asOf, nil
}

func (s *OrgService) ListDescendantsAsOf(ctx context.Context, tenantID uuid.UUID, orgNodeID uuid.UUID, asOf time.Time) ([]DeepReadRelation, time.Time, error) {
	if tenantID == uuid.Nil {
		return nil, time.Time{}, fmt.Errorf("tenant_id is required")
	}
	if orgNodeID == uuid.Nil {
		return nil, time.Time{}, fmt.Errorf("org_node_id is required")
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}

	const hierarchyType = "OrgUnit"
	cacheKey := repo.CacheKey("org", "descendants", tenantID, hierarchyType, orgNodeID, orgDateKey(asOf), OrgDeepReadBackendForTenant(tenantID))
	if s != nil && s.cache != nil && OrgCacheEnabled() {
		if cachedAny, ok := s.cache.Get(cacheKey); ok {
			if cached, ok := cachedAny.(cachedRelations); ok {
				return cached.Items, cached.AsOf, nil
			}
		}
	}

	out, err := inTx(ctx, tenantID, func(txCtx context.Context) ([]DeepReadRelation, error) {
		exists, err := s.repo.NodeExistsAt(txCtx, tenantID, orgNodeID, hierarchyType, asOf)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, newServiceError(404, "ORG_NODE_NOT_FOUND_AT_DATE", "org_node_id not found at effective_date", nil)
		}
		return s.repo.ListDescendantsAsOf(txCtx, tenantID, hierarchyType, orgNodeID, asOf, OrgDeepReadBackendForTenant(tenantID))
	})
	if err != nil {
		return nil, time.Time{}, err
	}

	if s != nil && s.cache != nil && OrgCacheEnabled() {
		s.cache.Set(tenantID, cacheKey, cachedRelations{Items: out, AsOf: asOf})
	}
	return out, asOf, nil
}

type cachedRelations struct {
	Items []DeepReadRelation
	AsOf  time.Time
}
