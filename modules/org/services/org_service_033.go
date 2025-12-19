package services

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type NodePathNode struct {
	ID    uuid.UUID `json:"id"`
	Code  string    `json:"code"`
	Name  string    `json:"name"`
	Depth int       `json:"depth"`
}

type NodePathSource struct {
	DeepReadBackend DeepReadBackend `json:"deep_read_backend"`
	AsOfDate        string          `json:"as_of_date"`
}

type NodePathResult struct {
	TenantID      uuid.UUID
	OrgNodeID     uuid.UUID
	EffectiveDate time.Time
	Path          []NodePathNode
	Source        NodePathSource
}

func (s *OrgService) GetNodePath(ctx context.Context, tenantID uuid.UUID, orgNodeID uuid.UUID, asOf time.Time) (*NodePathResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if orgNodeID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "org_node_id is required", nil)
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}

	const hierarchyType = "OrgUnit"
	backend := OrgDeepReadBackendForTenant(tenantID)

	out, err := inTx(ctx, tenantID, func(txCtx context.Context) (*NodePathResult, error) {
		exists, err := s.repo.NodeExistsAt(txCtx, tenantID, orgNodeID, hierarchyType, asOf)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, newServiceError(http.StatusNotFound, "ORG_NODE_NOT_FOUND_AT_DATE", "org_node_id not found at effective_date", nil)
		}

		rels, err := s.repo.ListAncestorsAsOf(txCtx, tenantID, hierarchyType, orgNodeID, asOf, backend)
		if err != nil {
			return nil, err
		}
		sort.Slice(rels, func(i, j int) bool {
			if rels[i].Depth != rels[j].Depth {
				return rels[i].Depth > rels[j].Depth
			}
			return rels[i].NodeID.String() < rels[j].NodeID.String()
		})

		nodeIDs := make([]uuid.UUID, 0, len(rels))
		for _, r := range rels {
			nodeIDs = append(nodeIDs, r.NodeID)
		}

		rows, err := s.repo.ListOrgNodesAsOf(txCtx, tenantID, hierarchyType, nodeIDs, asOf)
		if err != nil {
			return nil, err
		}
		byID := make(map[uuid.UUID]OrgNodeAsOfRow, len(rows))
		for _, row := range rows {
			byID[row.ID] = row
		}

		path := make([]NodePathNode, 0, len(nodeIDs))
		for i, id := range nodeIDs {
			row, ok := byID[id]
			if !ok {
				return nil, newServiceError(http.StatusInternalServerError, "ORG_INTERNAL", "node slice missing at effective_date", nil)
			}
			path = append(path, NodePathNode{
				ID:    row.ID,
				Code:  row.Code,
				Name:  row.Name,
				Depth: i,
			})
		}

		return &NodePathResult{
			TenantID:      tenantID,
			OrgNodeID:     orgNodeID,
			EffectiveDate: asOf.UTC(),
			Path:          path,
			Source: NodePathSource{
				DeepReadBackend: backend,
				AsOfDate:        asOf.UTC().Format("2006-01-02"),
			},
		}, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

type PersonPathAssignment struct {
	AssignmentID uuid.UUID
	PositionID   uuid.UUID
	OrgNodeID    uuid.UUID
}

type PersonPathResult struct {
	TenantID      uuid.UUID
	Subject       string
	EffectiveDate time.Time
	Assignment    PersonPathAssignment
	Path          []NodePathNode
	Source        NodePathSource
}

func (s *OrgService) GetPersonPath(ctx context.Context, tenantID uuid.UUID, subject string, asOf time.Time) (*PersonPathResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	subject = strings.TrimSpace(subject)
	if !strings.HasPrefix(subject, "person:") || strings.TrimSpace(strings.TrimPrefix(subject, "person:")) == "" {
		return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_SUBJECT_INVALID", "subject must be person:{pernr}", nil)
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}

	_, assignments, _, err := s.GetAssignments(ctx, tenantID, subject, &asOf)
	if err != nil {
		return nil, err
	}
	var primary *AssignmentViewRow
	for i := range assignments {
		if assignments[i].IsPrimary || assignments[i].AssignmentType == "primary" {
			primary = &assignments[i]
			break
		}
	}
	if primary == nil {
		return nil, newServiceError(http.StatusNotFound, "ORG_ASSIGNMENT_NOT_FOUND_AT_DATE", "primary assignment not found at effective_date", nil)
	}

	path, err := s.GetNodePath(ctx, tenantID, primary.OrgNodeID, asOf)
	if err != nil {
		return nil, err
	}

	return &PersonPathResult{
		TenantID:      tenantID,
		Subject:       subject,
		EffectiveDate: asOf.UTC(),
		Assignment: PersonPathAssignment{
			AssignmentID: primary.ID,
			PositionID:   primary.PositionID,
			OrgNodeID:    primary.OrgNodeID,
		},
		Path:   path.Path,
		Source: path.Source,
	}, nil
}

type HierarchyExportNode struct {
	ID                uuid.UUID
	ParentID          *uuid.UUID
	Code              string
	Name              string
	Depth             int
	Status            string
	SecurityGroupKeys []string
	Links             []OrgLinkSummary
}

type HierarchyExportResult struct {
	TenantID      uuid.UUID
	HierarchyType string
	EffectiveDate time.Time
	RootNodeID    uuid.UUID
	Includes      []string
	Limit         int
	Nodes         []HierarchyExportNode
	NextCursorID  *uuid.UUID
}

func (s *OrgService) ExportHierarchy(
	ctx context.Context,
	tenantID uuid.UUID,
	hierarchyType string,
	asOf time.Time,
	rootNodeID *uuid.UUID,
	maxDepth *int,
	includeEdges bool,
	includeSecurityGroups bool,
	includeLinks bool,
	limit int,
	afterID *uuid.UUID,
) (*HierarchyExportResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if strings.TrimSpace(hierarchyType) == "" {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "type is required", nil)
	}
	if hierarchyType != "OrgUnit" {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "type is invalid", nil)
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}
	if maxDepth != nil {
		if *maxDepth < 0 {
			return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "max_depth is invalid", nil)
		}
		if *maxDepth > 20 {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_EXPORT_TOO_DEEP", "max_depth exceeds limit", nil)
		}
	}
	if limit <= 0 {
		limit = 2000
	}
	if limit > 10000 {
		return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_EXPORT_TOO_LARGE", "limit exceeds maximum", nil)
	}

	const ht = "OrgUnit"
	backend := OrgDeepReadBackendForTenant(tenantID)

	includes := []string{"nodes"}
	if includeEdges {
		includes = append(includes, "edges")
	}
	if includeSecurityGroups {
		includes = append(includes, "security_groups")
	}
	if includeLinks {
		includes = append(includes, "links")
	}

	res, err := inTx(ctx, tenantID, func(txCtx context.Context) (*HierarchyExportResult, error) {
		var rootID uuid.UUID
		if rootNodeID != nil && *rootNodeID != uuid.Nil {
			rootID = *rootNodeID
		} else {
			id, err := s.repo.GetTenantRootNodeID(txCtx, tenantID)
			if err != nil {
				return nil, err
			}
			rootID = id
		}

		exists, err := s.repo.NodeExistsAt(txCtx, tenantID, rootID, ht, asOf)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, newServiceError(http.StatusNotFound, "ORG_NODE_NOT_FOUND_AT_DATE", "root_node_id not found at effective_date", nil)
		}

		rels, err := s.repo.ListDescendantsForExportAsOf(txCtx, tenantID, ht, rootID, asOf, backend, afterID, maxDepth, limit+1)
		if err != nil {
			return nil, err
		}

		var nextCursorID *uuid.UUID
		if len(rels) > limit {
			id := rels[limit-1].NodeID
			nextCursorID = &id
			rels = rels[:limit]
		}

		nodeIDs := make([]uuid.UUID, 0, len(rels))
		for _, r := range rels {
			nodeIDs = append(nodeIDs, r.NodeID)
		}

		rows, err := s.repo.ListOrgNodesAsOf(txCtx, tenantID, ht, nodeIDs, asOf)
		if err != nil {
			return nil, err
		}
		byID := make(map[uuid.UUID]OrgNodeAsOfRow, len(rows))
		for _, row := range rows {
			byID[row.ID] = row
		}

		sgKeys := map[uuid.UUID][]string{}
		if includeSecurityGroups {
			sgKeys, err = s.repo.ResolveSecurityGroupKeysForNodesAsOf(txCtx, tenantID, ht, nodeIDs, asOf, backend)
			if err != nil {
				return nil, err
			}
		}

		links := map[uuid.UUID][]OrgLinkSummary{}
		if includeLinks {
			links, err = s.repo.ListOrgLinkSummariesForNodesAsOf(txCtx, tenantID, nodeIDs, asOf)
			if err != nil {
				return nil, err
			}
		}

		nodes := make([]HierarchyExportNode, 0, len(rels))
		for _, rel := range rels {
			row, ok := byID[rel.NodeID]
			if !ok {
				return nil, newServiceError(http.StatusInternalServerError, "ORG_INTERNAL", "node slice missing at effective_date", nil)
			}
			nodes = append(nodes, HierarchyExportNode{
				ID:                row.ID,
				ParentID:          row.ParentID,
				Code:              row.Code,
				Name:              row.Name,
				Status:            row.Status,
				Depth:             rel.Depth,
				SecurityGroupKeys: sgKeys[row.ID],
				Links:             links[row.ID],
			})
		}

		return &HierarchyExportResult{
			TenantID:      tenantID,
			HierarchyType: hierarchyType,
			EffectiveDate: asOf.UTC(),
			RootNodeID:    rootID,
			Includes:      includes,
			Limit:         limit,
			Nodes:         nodes,
			NextCursorID:  nextCursorID,
		}, nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}
