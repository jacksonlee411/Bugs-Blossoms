package services

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type SecurityGroupMappingRow struct {
	ID               uuid.UUID
	OrgNodeID        uuid.UUID
	SecurityGroupKey string
	AppliesToSubtree bool
	EffectiveDate    time.Time
	EndDate          time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type SecurityGroupMappingInsert struct {
	OrgNodeID        uuid.UUID
	SecurityGroupKey string
	AppliesToSubtree bool
	EffectiveDate    time.Time
	EndDate          time.Time
}

type SecurityGroupMappingListFilter struct {
	OrgNodeID        *uuid.UUID
	SecurityGroupKey *string
	AsOf             *time.Time
	Limit            int
	CursorAt         *time.Time
	CursorID         *uuid.UUID
}

type OrgLinkRow struct {
	ID            uuid.UUID
	OrgNodeID     uuid.UUID
	ObjectType    string
	ObjectKey     string
	LinkType      string
	Metadata      json.RawMessage
	EffectiveDate time.Time
	EndDate       time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type OrgLinkInsert struct {
	OrgNodeID     uuid.UUID
	ObjectType    string
	ObjectKey     string
	LinkType      string
	Metadata      json.RawMessage
	EffectiveDate time.Time
	EndDate       time.Time
}

type OrgLinkListFilter struct {
	OrgNodeID  *uuid.UUID
	ObjectType *string
	ObjectKey  *string
	AsOf       *time.Time
	Limit      int
	CursorAt   *time.Time
	CursorID   *uuid.UUID
}

type CreateSecurityGroupMappingInput struct {
	OrgNodeID        uuid.UUID
	SecurityGroupKey string
	AppliesToSubtree bool
	EffectiveDate    time.Time
}

type CreateSecurityGroupMappingResult struct {
	ID            uuid.UUID
	EffectiveDate time.Time
	EndDate       time.Time
}

func (s *OrgService) CreateSecurityGroupMapping(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in CreateSecurityGroupMappingInput) (*CreateSecurityGroupMappingResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	txTime := time.Now().UTC()

	in.SecurityGroupKey = strings.TrimSpace(in.SecurityGroupKey)
	if in.OrgNodeID == uuid.Nil || in.SecurityGroupKey == "" || in.EffectiveDate.IsZero() {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "org_node_id/security_group_key/effective_date are required", nil)
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*CreateSecurityGroupMappingResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}
		if _, err := s.freezeCheck(settings, txTime, in.EffectiveDate); err != nil {
			return nil, err
		}

		const hierarchyType = "OrgUnit"
		exists, err := s.repo.NodeExistsAt(txCtx, tenantID, in.OrgNodeID, hierarchyType, in.EffectiveDate)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_NODE_NOT_FOUND_AT_DATE", "org_node_id not found at effective_date", nil)
		}

		id, err := s.repo.InsertSecurityGroupMapping(txCtx, tenantID, SecurityGroupMappingInsert{
			OrgNodeID:        in.OrgNodeID,
			SecurityGroupKey: in.SecurityGroupKey,
			AppliesToSubtree: in.AppliesToSubtree,
			EffectiveDate:    in.EffectiveDate,
			EndDate:          endOfTime,
		})
		if err != nil {
			return nil, mapPgError(err)
		}
		return &CreateSecurityGroupMappingResult{ID: id, EffectiveDate: in.EffectiveDate, EndDate: endOfTime}, nil
	})
	if err != nil {
		return nil, err
	}
	return written, nil
}

type RescindSecurityGroupMappingInput struct {
	ID            uuid.UUID
	EffectiveDate time.Time
	Reason        string
}

type RescindSecurityGroupMappingResult struct {
	ID            uuid.UUID
	EffectiveDate time.Time
	EndDate       time.Time
}

func (s *OrgService) RescindSecurityGroupMapping(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in RescindSecurityGroupMappingInput) (*RescindSecurityGroupMappingResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	txTime := time.Now().UTC()

	if in.ID == uuid.Nil || in.EffectiveDate.IsZero() {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "id/effective_date are required", nil)
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*RescindSecurityGroupMappingResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}
		if _, err := s.freezeCheck(settings, txTime, in.EffectiveDate); err != nil {
			return nil, err
		}

		current, err := s.repo.LockSecurityGroupMappingByID(txCtx, tenantID, in.ID)
		if err != nil {
			return nil, mapPgError(err)
		}
		if !in.EffectiveDate.After(current.EffectiveDate) || !in.EffectiveDate.Before(current.EndDate) {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_INVALID_RESCIND_DATE", "effective_date must be within current window", nil)
		}

		if err := s.repo.UpdateSecurityGroupMappingEndDate(txCtx, tenantID, current.ID, in.EffectiveDate); err != nil {
			return nil, mapPgError(err)
		}

		updated, err := s.repo.LockSecurityGroupMappingByID(txCtx, tenantID, current.ID)
		if err != nil {
			return nil, mapPgError(err)
		}
		_ = strings.TrimSpace(in.Reason)

		return &RescindSecurityGroupMappingResult{ID: updated.ID, EffectiveDate: updated.EffectiveDate, EndDate: updated.EndDate}, nil
	})
	if err != nil {
		return nil, err
	}
	return written, nil
}

type ListSecurityGroupMappingsResult struct {
	TenantID      uuid.UUID
	EffectiveDate *time.Time
	Items         []SecurityGroupMappingRow
	NextCursor    *string
}

func (s *OrgService) ListSecurityGroupMappings(ctx context.Context, tenantID uuid.UUID, filter SecurityGroupMappingListFilter) (*ListSecurityGroupMappingsResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if filter.Limit <= 0 {
		filter.Limit = 200
	}
	if filter.Limit > 1000 {
		filter.Limit = 1000
	}

	var asOf *time.Time
	if filter.AsOf != nil && !filter.AsOf.IsZero() {
		u := filter.AsOf.UTC()
		y, m, d := u.Date()
		t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		asOf = &t
		filter.AsOf = asOf
	}

	rows, err := inTx(ctx, tenantID, func(txCtx context.Context) ([]SecurityGroupMappingRow, error) {
		return s.repo.ListSecurityGroupMappings(txCtx, tenantID, filter)
	})
	if err != nil {
		return nil, err
	}

	nextCursor := (*string)(nil)
	if len(rows) > filter.Limit {
		last := rows[filter.Limit-1]
		v := "effective_date:" + last.EffectiveDate.UTC().Format(time.DateOnly) + ":id:" + last.ID.String()
		nextCursor = &v
		rows = rows[:filter.Limit]
	}
	return &ListSecurityGroupMappingsResult{
		TenantID:      tenantID,
		EffectiveDate: asOf,
		Items:         rows,
		NextCursor:    nextCursor,
	}, nil
}

type CreateOrgLinkInput struct {
	OrgNodeID     uuid.UUID
	ObjectType    string
	ObjectKey     string
	LinkType      string
	Metadata      map[string]any
	EffectiveDate time.Time
}

type CreateOrgLinkResult struct {
	ID            uuid.UUID
	EffectiveDate time.Time
	EndDate       time.Time
}

func (s *OrgService) CreateOrgLink(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in CreateOrgLinkInput) (*CreateOrgLinkResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	txTime := time.Now().UTC()

	in.ObjectType = strings.TrimSpace(in.ObjectType)
	in.ObjectKey = strings.TrimSpace(in.ObjectKey)
	in.LinkType = strings.TrimSpace(in.LinkType)
	if in.Metadata == nil {
		in.Metadata = map[string]any{}
	}
	if in.OrgNodeID == uuid.Nil || in.ObjectType == "" || in.ObjectKey == "" || in.LinkType == "" || in.EffectiveDate.IsZero() {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "org_node_id/object_type/object_key/link_type/effective_date are required", nil)
	}

	switch in.ObjectType {
	case "project", "cost_center", "budget_item", "custom":
	default:
		return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_INVALID_BODY", "object_type is invalid", nil)
	}
	switch in.LinkType {
	case "owns", "uses", "reports_to", "custom":
	default:
		return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_INVALID_BODY", "link_type is invalid", nil)
	}

	metaRaw, err := json.Marshal(in.Metadata)
	if err != nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "metadata is invalid", err)
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*CreateOrgLinkResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}
		if _, err := s.freezeCheck(settings, txTime, in.EffectiveDate); err != nil {
			return nil, err
		}

		const hierarchyType = "OrgUnit"
		exists, err := s.repo.NodeExistsAt(txCtx, tenantID, in.OrgNodeID, hierarchyType, in.EffectiveDate)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_NODE_NOT_FOUND_AT_DATE", "org_node_id not found at effective_date", nil)
		}

		id, err := s.repo.InsertOrgLink(txCtx, tenantID, OrgLinkInsert{
			OrgNodeID:     in.OrgNodeID,
			ObjectType:    in.ObjectType,
			ObjectKey:     in.ObjectKey,
			LinkType:      in.LinkType,
			Metadata:      metaRaw,
			EffectiveDate: in.EffectiveDate,
			EndDate:       endOfTime,
		})
		if err != nil {
			return nil, mapPgError(err)
		}
		return &CreateOrgLinkResult{ID: id, EffectiveDate: in.EffectiveDate, EndDate: endOfTime}, nil
	})
	if err != nil {
		return nil, err
	}
	return written, nil
}

type RescindOrgLinkInput struct {
	ID            uuid.UUID
	EffectiveDate time.Time
	Reason        string
}

type RescindOrgLinkResult struct {
	ID            uuid.UUID
	EffectiveDate time.Time
	EndDate       time.Time
}

func (s *OrgService) RescindOrgLink(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in RescindOrgLinkInput) (*RescindOrgLinkResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	txTime := time.Now().UTC()

	if in.ID == uuid.Nil || in.EffectiveDate.IsZero() {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "id/effective_date are required", nil)
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*RescindOrgLinkResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}
		if _, err := s.freezeCheck(settings, txTime, in.EffectiveDate); err != nil {
			return nil, err
		}

		current, err := s.repo.LockOrgLinkByID(txCtx, tenantID, in.ID)
		if err != nil {
			return nil, mapPgError(err)
		}
		if !in.EffectiveDate.After(current.EffectiveDate) || !in.EffectiveDate.Before(current.EndDate) {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_INVALID_RESCIND_DATE", "effective_date must be within current window", nil)
		}

		if err := s.repo.UpdateOrgLinkEndDate(txCtx, tenantID, current.ID, in.EffectiveDate); err != nil {
			return nil, mapPgError(err)
		}

		updated, err := s.repo.LockOrgLinkByID(txCtx, tenantID, current.ID)
		if err != nil {
			return nil, mapPgError(err)
		}
		_ = strings.TrimSpace(in.Reason)

		return &RescindOrgLinkResult{ID: updated.ID, EffectiveDate: updated.EffectiveDate, EndDate: updated.EndDate}, nil
	})
	if err != nil {
		return nil, err
	}
	return written, nil
}

type ListOrgLinksResult struct {
	TenantID      uuid.UUID
	EffectiveDate *time.Time
	Items         []OrgLinkRow
	NextCursor    *string
}

func (s *OrgService) ListOrgLinks(ctx context.Context, tenantID uuid.UUID, filter OrgLinkListFilter) (*ListOrgLinksResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if filter.Limit <= 0 {
		filter.Limit = 200
	}
	if filter.Limit > 1000 {
		filter.Limit = 1000
	}

	var asOf *time.Time
	if filter.AsOf != nil && !filter.AsOf.IsZero() {
		u := filter.AsOf.UTC()
		y, m, d := u.Date()
		t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		asOf = &t
		filter.AsOf = asOf
	}

	rows, err := inTx(ctx, tenantID, func(txCtx context.Context) ([]OrgLinkRow, error) {
		return s.repo.ListOrgLinks(txCtx, tenantID, filter)
	})
	if err != nil {
		return nil, err
	}

	nextCursor := (*string)(nil)
	if len(rows) > filter.Limit {
		last := rows[filter.Limit-1]
		v := "effective_date:" + last.EffectiveDate.UTC().Format(time.DateOnly) + ":id:" + last.ID.String()
		nextCursor = &v
		rows = rows[:filter.Limit]
	}

	return &ListOrgLinksResult{
		TenantID:      tenantID,
		EffectiveDate: asOf,
		Items:         rows,
		NextCursor:    nextCursor,
	}, nil
}

type PermissionPreviewSecurityGroup struct {
	SecurityGroupKey string
	AppliesToSubtree bool
	SourceOrgNodeID  uuid.UUID
	SourceDepth      int
}

type PermissionPreviewLinkSource struct {
	LinkID    uuid.UUID `json:"link_id"`
	OrgNodeID uuid.UUID `json:"org_node_id"`
}

type PermissionPreviewLink struct {
	ObjectType string                      `json:"object_type"`
	ObjectKey  string                      `json:"object_key"`
	LinkType   string                      `json:"link_type"`
	Source     PermissionPreviewLinkSource `json:"source"`
}

type PermissionPreviewResult struct {
	TenantID       uuid.UUID
	OrgNodeID      uuid.UUID
	EffectiveDate  time.Time
	SecurityGroups []PermissionPreviewSecurityGroup
	Links          []PermissionPreviewLink
	Warnings       []string
}

type PermissionPreviewInput struct {
	OrgNodeID             uuid.UUID
	EffectiveDate         time.Time
	IncludeSecurityGroups bool
	IncludeLinks          bool
	LimitLinks            int
}

func (s *OrgService) PermissionPreview(ctx context.Context, tenantID uuid.UUID, in PermissionPreviewInput) (*PermissionPreviewResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if in.OrgNodeID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "org_node_id is required", nil)
	}
	if in.EffectiveDate.IsZero() {
		in.EffectiveDate = time.Now().UTC()
	}
	if in.LimitLinks <= 0 {
		in.LimitLinks = 200
	}
	if in.LimitLinks > 1000 {
		in.LimitLinks = 1000
	}

	out, err := inTx(ctx, tenantID, func(txCtx context.Context) (*PermissionPreviewResult, error) {
		res := &PermissionPreviewResult{
			TenantID:       tenantID,
			OrgNodeID:      in.OrgNodeID,
			EffectiveDate:  in.EffectiveDate.UTC(),
			SecurityGroups: []PermissionPreviewSecurityGroup{},
			Links:          []PermissionPreviewLink{},
			Warnings:       []string{},
		}

		const hierarchyType = "OrgUnit"
		exists, err := s.repo.NodeExistsAt(txCtx, tenantID, in.OrgNodeID, hierarchyType, in.EffectiveDate)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, newServiceError(http.StatusNotFound, "ORG_NODE_NOT_FOUND_AT_DATE", "org_node_id not found at effective_date", nil)
		}

		var depths map[uuid.UUID]int
		var ancestorIDs []uuid.UUID
		if in.IncludeSecurityGroups {
			anc, _, err := s.ListAncestorsAsOf(txCtx, tenantID, in.OrgNodeID, in.EffectiveDate)
			if err != nil {
				return nil, err
			}
			depths = make(map[uuid.UUID]int, len(anc))
			ancestorIDs = make([]uuid.UUID, 0, len(anc))
			for _, r := range anc {
				depths[r.NodeID] = r.Depth
				ancestorIDs = append(ancestorIDs, r.NodeID)
			}

			rows, err := s.repo.ListSecurityGroupMappingsForNodesAsOf(txCtx, tenantID, ancestorIDs, in.EffectiveDate)
			if err != nil {
				return nil, err
			}

			type best struct {
				key     string
				depth   int
				node    uuid.UUID
				applies bool
			}
			bestByKey := map[string]best{}
			for _, row := range rows {
				if row.OrgNodeID != in.OrgNodeID && !row.AppliesToSubtree {
					continue
				}
				d, ok := depths[row.OrgNodeID]
				if !ok {
					continue
				}
				prev, ok := bestByKey[row.SecurityGroupKey]
				cur := best{key: row.SecurityGroupKey, depth: d, node: row.OrgNodeID, applies: row.AppliesToSubtree}
				if !ok || cur.depth < prev.depth || (cur.depth == prev.depth && cur.node.String() < prev.node.String()) {
					bestByKey[row.SecurityGroupKey] = cur
				}
			}

			items := make([]PermissionPreviewSecurityGroup, 0, len(bestByKey))
			for _, v := range bestByKey {
				items = append(items, PermissionPreviewSecurityGroup{
					SecurityGroupKey: v.key,
					AppliesToSubtree: v.applies,
					SourceOrgNodeID:  v.node,
					SourceDepth:      v.depth,
				})
			}
			sort.Slice(items, func(i, j int) bool {
				if items[i].SourceDepth != items[j].SourceDepth {
					return items[i].SourceDepth < items[j].SourceDepth
				}
				return items[i].SecurityGroupKey < items[j].SecurityGroupKey
			})
			res.SecurityGroups = items
		}

		if in.IncludeLinks {
			rows, err := s.repo.ListOrgLinksForNodeAsOf(txCtx, tenantID, in.OrgNodeID, in.EffectiveDate, in.LimitLinks+1)
			if err != nil {
				return nil, err
			}
			if len(rows) > in.LimitLinks {
				res.Warnings = append(res.Warnings, "links_truncated")
				rows = rows[:in.LimitLinks]
			}
			items := make([]PermissionPreviewLink, 0, len(rows))
			for _, row := range rows {
				items = append(items, PermissionPreviewLink{
					ObjectType: row.ObjectType,
					ObjectKey:  row.ObjectKey,
					LinkType:   row.LinkType,
					Source: PermissionPreviewLinkSource{
						LinkID:    row.ID,
						OrgNodeID: row.OrgNodeID,
					},
				})
			}
			res.Links = items
		}

		return res, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
