package services

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/pkg/repo"
)

var orgInheritanceAttributeWhitelist = []string{
	"legal_entity_id",
	"company_code",
	"location_id",
	"manager_user_id",
}

var orgInheritanceAttributeWhitelistSet = func() map[string]struct{} {
	out := make(map[string]struct{}, len(orgInheritanceAttributeWhitelist))
	for _, k := range orgInheritanceAttributeWhitelist {
		out[k] = struct{}{}
	}
	return out
}()

type OrgNodeAttributes struct {
	LegalEntityID *uuid.UUID `json:"legal_entity_id"`
	CompanyCode   *string    `json:"company_code"`
	LocationID    *uuid.UUID `json:"location_id"`
	ManagerUserID *int64     `json:"manager_user_id"`
}

type OrgNodeAttributeSources struct {
	LegalEntityID *uuid.UUID `json:"legal_entity_id"`
	CompanyCode   *uuid.UUID `json:"company_code"`
	LocationID    *uuid.UUID `json:"location_id"`
	ManagerUserID *uuid.UUID `json:"manager_user_id"`
}

type AttributeInheritanceRule struct {
	AttributeName            string
	CanOverride              bool
	InheritanceBreakNodeType *string
}

type HierarchyNodeWithResolvedAttributes struct {
	HierarchyNode
	Attributes         OrgNodeAttributes `json:"attributes"`
	ResolvedAttributes OrgNodeAttributes `json:"resolved_attributes"`
}

type cachedHierarchyResolvedAttributes struct {
	Nodes          []HierarchyNodeWithResolvedAttributes
	AsOf           time.Time
	Sources        map[uuid.UUID]OrgNodeAttributeSources
	RuleAttributes []string
}

type OrgRole struct {
	ID          uuid.UUID `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	IsSystem    bool      `json:"is_system"`
}

type RoleAssignmentRow struct {
	AssignmentID    uuid.UUID
	RoleID          uuid.UUID
	RoleCode        string
	SubjectType     string
	SubjectID       uuid.UUID
	SourceOrgNodeID uuid.UUID
	EffectiveDate   time.Time
	EndDate         time.Time
}

type RoleAssignmentItem struct {
	AssignmentID    uuid.UUID `json:"assignment_id"`
	RoleID          uuid.UUID `json:"role_id"`
	RoleCode        string    `json:"role_code"`
	Subject         string    `json:"subject"`
	SourceOrgNodeID uuid.UUID `json:"source_org_node_id"`
	EffectiveWindow struct {
		EffectiveDate string `json:"effective_date"`
		EndDate       string `json:"end_date"`
	} `json:"effective_window"`
}

func orgDateKey(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

func resolvePtr[T any](inheritable bool, canOverride bool, explicit *T, parent *T, parentSource *uuid.UUID, selfID uuid.UUID) (*T, *uuid.UUID) {
	if !inheritable {
		if explicit == nil {
			return nil, nil
		}
		id := selfID
		return explicit, &id
	}

	if canOverride {
		if explicit != nil {
			id := selfID
			return explicit, &id
		}
		if parent != nil {
			return parent, parentSource
		}
		return nil, nil
	}

	if parent != nil {
		return parent, parentSource
	}
	if explicit != nil {
		id := selfID
		return explicit, &id
	}
	return nil, nil
}

func resolveOrgAttributes(
	nodes []HierarchyNode,
	explicit map[uuid.UUID]OrgNodeAttributes,
	rulesByAttr map[string]AttributeInheritanceRule,
) (map[uuid.UUID]OrgNodeAttributes, map[uuid.UUID]OrgNodeAttributeSources) {
	resolved := make(map[uuid.UUID]OrgNodeAttributes, len(nodes))
	sources := make(map[uuid.UUID]OrgNodeAttributeSources, len(nodes))

	for _, n := range nodes {
		exp := explicit[n.ID]
		var (
			parentResolved OrgNodeAttributes
			parentSources  OrgNodeAttributeSources
		)
		if n.ParentID != nil {
			parentResolved = resolved[*n.ParentID]
			parentSources = sources[*n.ParentID]
		}

		ruleLegal, okLegal := rulesByAttr["legal_entity_id"]
		ruleCompany, okCompany := rulesByAttr["company_code"]
		ruleLocation, okLocation := rulesByAttr["location_id"]
		ruleManager, okManager := rulesByAttr["manager_user_id"]

		legalEntityID, legalSource := resolvePtr(okLegal, ruleLegal.CanOverride, exp.LegalEntityID, parentResolved.LegalEntityID, parentSources.LegalEntityID, n.ID)
		companyCode, companySource := resolvePtr(okCompany, ruleCompany.CanOverride, exp.CompanyCode, parentResolved.CompanyCode, parentSources.CompanyCode, n.ID)
		locationID, locationSource := resolvePtr(okLocation, ruleLocation.CanOverride, exp.LocationID, parentResolved.LocationID, parentSources.LocationID, n.ID)
		managerUserID, managerSource := resolvePtr(okManager, ruleManager.CanOverride, exp.ManagerUserID, parentResolved.ManagerUserID, parentSources.ManagerUserID, n.ID)

		resolved[n.ID] = OrgNodeAttributes{
			LegalEntityID: legalEntityID,
			CompanyCode:   companyCode,
			LocationID:    locationID,
			ManagerUserID: managerUserID,
		}
		sources[n.ID] = OrgNodeAttributeSources{
			LegalEntityID: legalSource,
			CompanyCode:   companySource,
			LocationID:    locationSource,
			ManagerUserID: managerSource,
		}
	}

	return resolved, sources
}

func (s *OrgService) GetHierarchyResolvedAttributes(ctx context.Context, tenantID uuid.UUID, hierarchyType string, asOf time.Time) ([]HierarchyNodeWithResolvedAttributes, time.Time, map[uuid.UUID]OrgNodeAttributeSources, []string, error) {
	if tenantID == uuid.Nil {
		return nil, time.Time{}, nil, nil, fmt.Errorf("tenant_id is required")
	}
	if hierarchyType != "OrgUnit" {
		return nil, time.Time{}, nil, nil, fmt.Errorf("unsupported hierarchy type: %s", hierarchyType)
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}

	if !configuration.Use().OrgInheritanceEnabled {
		return nil, time.Time{}, nil, nil, newServiceError(404, "ORG_INHERITANCE_DISABLED", "inheritance resolver is disabled", nil)
	}

	cacheKey := repo.CacheKey("org", "hierarchy_resolved_attributes", tenantID, hierarchyType, orgDateKey(asOf))
	if s != nil && s.cache != nil {
		if cachedAny, ok := s.cache.Get(cacheKey); ok {
			if cached, ok := cachedAny.(cachedHierarchyResolvedAttributes); ok {
				recordCacheRequest("hierarchy", true)
				return cached.Nodes, cached.AsOf, cached.Sources, cached.RuleAttributes, nil
			}
		}
		recordCacheRequest("hierarchy", false)
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (cachedHierarchyResolvedAttributes, error) {
		var (
			nodes []HierarchyNode
			err   error
		)
		if OrgReadStrategy() == "recursive" {
			nodes, err = s.repo.ListHierarchyAsOfRecursive(txCtx, tenantID, hierarchyType, asOf)
		} else {
			nodes, err = s.repo.ListHierarchyAsOf(txCtx, tenantID, hierarchyType, asOf)
		}
		if err != nil {
			return cachedHierarchyResolvedAttributes{}, err
		}

		ids := make([]uuid.UUID, 0, len(nodes))
		for _, n := range nodes {
			ids = append(ids, n.ID)
		}

		explicit, err := s.repo.ListNodeAttributesAsOf(txCtx, tenantID, ids, asOf)
		if err != nil {
			return cachedHierarchyResolvedAttributes{}, err
		}

		rules, err := s.repo.ListAttributeInheritanceRulesAsOf(txCtx, tenantID, hierarchyType, asOf)
		if err != nil {
			return cachedHierarchyResolvedAttributes{}, err
		}
		rulesByAttr := make(map[string]AttributeInheritanceRule, len(rules))
		ruleAttributes := make([]string, 0, len(rules))
		for _, r := range rules {
			name := strings.TrimSpace(strings.ToLower(r.AttributeName))
			if _, ok := orgInheritanceAttributeWhitelistSet[name]; !ok {
				continue
			}
			r.AttributeName = name
			rulesByAttr[name] = r
			ruleAttributes = append(ruleAttributes, name)
		}

		resolved, sources := resolveOrgAttributes(nodes, explicit, rulesByAttr)
		out := make([]HierarchyNodeWithResolvedAttributes, 0, len(nodes))
		for _, n := range nodes {
			out = append(out, HierarchyNodeWithResolvedAttributes{
				HierarchyNode:      n,
				Attributes:         explicit[n.ID],
				ResolvedAttributes: resolved[n.ID],
			})
		}
		return cachedHierarchyResolvedAttributes{
			Nodes:          out,
			AsOf:           asOf,
			Sources:        sources,
			RuleAttributes: ruleAttributes,
		}, nil
	})
	if err != nil {
		return nil, time.Time{}, nil, nil, err
	}

	if s != nil && s.cache != nil {
		s.cache.Set(tenantID, cacheKey, written)
	}

	return written.Nodes, written.AsOf, written.Sources, written.RuleAttributes, nil
}

func (s *OrgService) ListRoles(ctx context.Context, tenantID uuid.UUID) ([]OrgRole, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if !configuration.Use().OrgRoleReadEnabled {
		return nil, newServiceError(404, "ORG_ROLE_READ_DISABLED", "role read is disabled", nil)
	}

	cacheKey := repo.CacheKey("org", "roles", tenantID)
	if s != nil && s.cache != nil {
		if cachedAny, ok := s.cache.Get(cacheKey); ok {
			if cached, ok := cachedAny.([]OrgRole); ok {
				recordCacheRequest("snapshot", true)
				return cached, nil
			}
		}
		recordCacheRequest("snapshot", false)
	}

	roles, err := inTx(ctx, tenantID, func(txCtx context.Context) ([]OrgRole, error) {
		return s.repo.ListRoles(txCtx, tenantID)
	})
	if err != nil {
		return nil, err
	}
	if s != nil && s.cache != nil {
		s.cache.Set(tenantID, cacheKey, roles)
	}
	return roles, nil
}

func (s *OrgService) ListRoleAssignments(
	ctx context.Context,
	tenantID uuid.UUID,
	orgNodeID uuid.UUID,
	asOf time.Time,
	includeInherited bool,
	roleCode *string,
	subjectType *string,
	subjectID *uuid.UUID,
) ([]RoleAssignmentItem, time.Time, error) {
	if tenantID == uuid.Nil {
		return nil, time.Time{}, fmt.Errorf("tenant_id is required")
	}
	if orgNodeID == uuid.Nil {
		return nil, time.Time{}, fmt.Errorf("org_node_id is required")
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}
	if !configuration.Use().OrgRoleReadEnabled {
		return nil, time.Time{}, newServiceError(404, "ORG_ROLE_READ_DISABLED", "role read is disabled", nil)
	}

	backendKey := DeepReadBackendEdges
	if includeInherited {
		backendKey = OrgDeepReadBackendForTenant(tenantID)
	}

	cacheKey := repo.CacheKey(
		"org",
		"role_assignments",
		tenantID,
		orgNodeID,
		orgDateKey(asOf),
		includeInherited,
		backendKey,
		strings.TrimSpace(derefString(roleCode)),
		strings.TrimSpace(derefString(subjectType)),
		derefUUID(subjectID),
	)
	if s != nil && s.cache != nil {
		if cachedAny, ok := s.cache.Get(cacheKey); ok {
			if cached, ok := cachedAny.(cachedRoleAssignments); ok {
				recordCacheRequest("snapshot", true)
				return cached.Items, cached.AsOf, nil
			}
		}
		recordCacheRequest("snapshot", false)
	}

	const hierarchyType = "OrgUnit"
	rows, err := inTx(ctx, tenantID, func(txCtx context.Context) ([]RoleAssignmentRow, error) {
		exists, err := s.repo.NodeExistsAt(txCtx, tenantID, orgNodeID, hierarchyType, asOf)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, newServiceError(404, "ORG_NODE_NOT_FOUND_AT_DATE", "org_node_id not found at effective_date", nil)
		}
		return s.repo.ListRoleAssignmentsAsOf(txCtx, tenantID, hierarchyType, orgNodeID, asOf, includeInherited, backendKey, roleCode, subjectType, subjectID)
	})
	if err != nil {
		return nil, time.Time{}, err
	}

	items := make([]RoleAssignmentItem, 0, len(rows))
	for _, r := range rows {
		sub := fmt.Sprintf("%s:%s", strings.TrimSpace(r.SubjectType), r.SubjectID.String())
		var it RoleAssignmentItem
		it.AssignmentID = r.AssignmentID
		it.RoleID = r.RoleID
		it.RoleCode = r.RoleCode
		it.Subject = sub
		it.SourceOrgNodeID = r.SourceOrgNodeID
		it.EffectiveWindow.EffectiveDate = r.EffectiveDate.UTC().Format(time.RFC3339)
		it.EffectiveWindow.EndDate = r.EndDate.UTC().Format(time.RFC3339)
		items = append(items, it)
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].RoleCode != items[j].RoleCode {
			return items[i].RoleCode < items[j].RoleCode
		}
		if items[i].Subject != items[j].Subject {
			return items[i].Subject < items[j].Subject
		}
		if items[i].SourceOrgNodeID != items[j].SourceOrgNodeID {
			return items[i].SourceOrgNodeID.String() < items[j].SourceOrgNodeID.String()
		}
		return items[i].AssignmentID.String() < items[j].AssignmentID.String()
	})

	if s != nil && s.cache != nil {
		s.cache.Set(tenantID, cacheKey, cachedRoleAssignments{Items: items, AsOf: asOf})
	}

	return items, asOf, nil
}

type cachedRoleAssignments struct {
	Items []RoleAssignmentItem
	AsOf  time.Time
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func derefUUID(v *uuid.UUID) string {
	if v == nil || *v == uuid.Nil {
		return ""
	}
	return (*v).String()
}
