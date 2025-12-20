package services

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	staffingMaxScopeNodes   = 10_000
	staffingDefaultLimit    = 200
	staffingMaxLimit        = 2_000
	staffingMaxRangeDays    = 366
	staffingScopeHierarchy  = "OrgUnit"
	staffingZeroEndOfWindow = time.Nanosecond
)

type StaffingSummaryInput struct {
	OrgNodeID          *uuid.UUID
	EffectiveDate      time.Time
	Scope              StaffingScope
	GroupBy            StaffingGroupBy
	LifecycleStatuses  []string
	IncludeSystem      bool
	MaxScopeNodesLimit int
}

type StaffingVacanciesInput struct {
	OrgNodeID          *uuid.UUID
	EffectiveDate      time.Time
	Scope              StaffingScope
	LifecycleStatuses  []string
	IncludeSystem      bool
	Limit              int
	Cursor             *uuid.UUID
	MaxScopeNodesLimit int
}

type StaffingTimeToFillInput struct {
	OrgNodeID          *uuid.UUID
	From               time.Time
	To                 time.Time
	Scope              StaffingScope
	GroupBy            StaffingGroupBy
	LifecycleStatuses  []string
	IncludeSystem      bool
	MaxScopeNodesLimit int
}

func (s *OrgService) GetStaffingSummary(ctx context.Context, tenantID uuid.UUID, in StaffingSummaryInput) (*StaffingSummaryResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}

	asOf := in.EffectiveDate
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	} else {
		asOf = asOf.UTC()
	}

	scope := in.Scope
	if strings.TrimSpace(string(scope)) == "" {
		scope = StaffingScopeSubtree
	}
	if scope != StaffingScopeSelf && scope != StaffingScopeSubtree {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "scope is invalid", nil)
	}

	groupBy := in.GroupBy
	if strings.TrimSpace(string(groupBy)) == "" {
		groupBy = StaffingGroupByNone
	}
	switch groupBy {
	case StaffingGroupByNone, StaffingGroupByJobLevel, StaffingGroupByPositionType:
	default:
		return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_REPORT_GROUP_BY_INVALID", "group_by is invalid", nil)
	}

	statuses, err := normalizeStaffingLifecycleStatuses(in.LifecycleStatuses)
	if err != nil {
		return nil, err
	}

	maxScope := in.MaxScopeNodesLimit
	if maxScope <= 0 {
		maxScope = staffingMaxScopeNodes
	}

	backend := OrgDeepReadBackendForTenant(tenantID)

	out, err := inTx(ctx, tenantID, func(txCtx context.Context) (*StaffingSummaryResult, error) {
		rootID, err := resolveOrgNodeID(txCtx, s.repo, tenantID, in.OrgNodeID)
		if err != nil {
			return nil, err
		}

		exists, err := s.repo.NodeExistsAt(txCtx, tenantID, rootID, staffingScopeHierarchy, asOf)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_NODE_NOT_FOUND_AT_DATE", "org_node_id not found at effective_date", nil)
		}

		nodeIDs, err := staffingResolveScopeNodeIDs(txCtx, s.repo, tenantID, rootID, asOf, scope, backend, maxScope)
		if err != nil {
			return nil, err
		}

		var snapshotBuildID *uuid.UUID
		if backend == DeepReadBackendSnapshot {
			id, err := s.repo.GetActiveSnapshotBuildID(txCtx, tenantID, staffingScopeHierarchy, asOf)
			if err != nil {
				return nil, err
			}
			snapshotBuildID = &id
		}

		repoGroupBy := groupBy
		if repoGroupBy == StaffingGroupByJobLevel {
			repoGroupBy = StaffingGroupByNone
		}

		dbRes, err := s.repo.GetStaffingSummaryReport(txCtx, tenantID, asOf, nodeIDs, statuses, in.IncludeSystem, repoGroupBy)
		if err != nil {
			return nil, err
		}

		totals := staffingTotalsFromAggregate(dbRes.Totals)
		breakdown := make([]StaffingBreakdownRow, 0, len(dbRes.Breakdown))
		for _, row := range dbRes.Breakdown {
			breakdown = append(breakdown, staffingBreakdownFromAggregate(row))
		}
		if groupBy == StaffingGroupByJobLevel && totals.PositionsTotal > 0 {
			breakdown = []StaffingBreakdownRow{{
				Key:            "unknown",
				PositionsTotal: totals.PositionsTotal,
				CapacityFTE:    totals.CapacityFTE,
				OccupiedFTE:    totals.OccupiedFTE,
				AvailableFTE:   totals.AvailableFTE,
				FillRate:       totals.FillRate,
			}}
		}

		return &StaffingSummaryResult{
			TenantID:      tenantID,
			OrgNodeID:     rootID,
			EffectiveDate: asOf,
			Scope:         scope,
			Totals:        totals,
			Breakdown:     breakdown,
			Source: StaffingReportSource{
				DeepReadBackend: backend,
				SnapshotBuildID: snapshotBuildID,
			},
		}, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *OrgService) ListStaffingVacancies(ctx context.Context, tenantID uuid.UUID, in StaffingVacanciesInput) (*StaffingVacanciesResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}

	asOf := in.EffectiveDate
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	} else {
		asOf = asOf.UTC()
	}

	scope := in.Scope
	if strings.TrimSpace(string(scope)) == "" {
		scope = StaffingScopeSubtree
	}
	if scope != StaffingScopeSelf && scope != StaffingScopeSubtree {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "scope is invalid", nil)
	}

	statuses, err := normalizeStaffingLifecycleStatuses(in.LifecycleStatuses)
	if err != nil {
		return nil, err
	}

	limit := in.Limit
	if limit <= 0 {
		limit = staffingDefaultLimit
	}
	if limit > staffingMaxLimit {
		return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_REPORT_LIMIT_TOO_LARGE", "limit exceeds maximum", nil)
	}

	maxScope := in.MaxScopeNodesLimit
	if maxScope <= 0 {
		maxScope = staffingMaxScopeNodes
	}

	backend := OrgDeepReadBackendForTenant(tenantID)

	out, err := inTx(ctx, tenantID, func(txCtx context.Context) (*StaffingVacanciesResult, error) {
		rootID, err := resolveOrgNodeID(txCtx, s.repo, tenantID, in.OrgNodeID)
		if err != nil {
			return nil, err
		}

		exists, err := s.repo.NodeExistsAt(txCtx, tenantID, rootID, staffingScopeHierarchy, asOf)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_NODE_NOT_FOUND_AT_DATE", "org_node_id not found at effective_date", nil)
		}

		nodeIDs, err := staffingResolveScopeNodeIDs(txCtx, s.repo, tenantID, rootID, asOf, scope, backend, maxScope)
		if err != nil {
			return nil, err
		}

		var snapshotBuildID *uuid.UUID
		if backend == DeepReadBackendSnapshot {
			id, err := s.repo.GetActiveSnapshotBuildID(txCtx, tenantID, staffingScopeHierarchy, asOf)
			if err != nil {
				return nil, err
			}
			snapshotBuildID = &id
		}

		dbRes, err := s.repo.ListStaffingVacanciesReport(txCtx, tenantID, asOf, nodeIDs, statuses, in.IncludeSystem, limit, in.Cursor)
		if err != nil {
			return nil, err
		}

		return &StaffingVacanciesResult{
			TenantID:      tenantID,
			OrgNodeID:     rootID,
			EffectiveDate: asOf,
			Scope:         scope,
			Items:         dbRes.Items,
			NextCursor:    dbRes.NextCursor,
			Source: StaffingReportSource{
				DeepReadBackend: backend,
				SnapshotBuildID: snapshotBuildID,
			},
		}, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *OrgService) GetStaffingTimeToFill(ctx context.Context, tenantID uuid.UUID, in StaffingTimeToFillInput) (*StaffingTimeToFillResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}

	from := in.From
	to := in.To
	if from.IsZero() || to.IsZero() {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "from/to are required", nil)
	}
	from = time.Date(from.UTC().Year(), from.UTC().Month(), from.UTC().Day(), 0, 0, 0, 0, time.UTC)
	to = time.Date(to.UTC().Year(), to.UTC().Month(), to.UTC().Day(), 0, 0, 0, 0, time.UTC)
	if !to.After(from) {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "to must be after from", nil)
	}
	if to.Sub(from) > time.Duration(staffingMaxRangeDays)*24*time.Hour {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "time range exceeds limit", nil)
	}

	scope := in.Scope
	if strings.TrimSpace(string(scope)) == "" {
		scope = StaffingScopeSubtree
	}
	if scope != StaffingScopeSelf && scope != StaffingScopeSubtree {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "scope is invalid", nil)
	}

	groupBy := in.GroupBy
	if strings.TrimSpace(string(groupBy)) == "" {
		groupBy = StaffingGroupByNone
	}
	switch groupBy {
	case StaffingGroupByNone, StaffingGroupByJobLevel, StaffingGroupByPositionType:
	default:
		return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_REPORT_GROUP_BY_INVALID", "group_by is invalid", nil)
	}

	statuses, err := normalizeStaffingLifecycleStatuses(in.LifecycleStatuses)
	if err != nil {
		return nil, err
	}

	maxScope := in.MaxScopeNodesLimit
	if maxScope <= 0 {
		maxScope = staffingMaxScopeNodes
	}

	backend := OrgDeepReadBackendForTenant(tenantID)
	scopeAsOf := to.Add(-staffingZeroEndOfWindow)

	out, err := inTx(ctx, tenantID, func(txCtx context.Context) (*StaffingTimeToFillResult, error) {
		rootID, err := resolveOrgNodeID(txCtx, s.repo, tenantID, in.OrgNodeID)
		if err != nil {
			return nil, err
		}

		exists, err := s.repo.NodeExistsAt(txCtx, tenantID, rootID, staffingScopeHierarchy, scopeAsOf)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_NODE_NOT_FOUND_AT_DATE", "org_node_id not found at effective_date", nil)
		}

		nodeIDs, err := staffingResolveScopeNodeIDs(txCtx, s.repo, tenantID, rootID, scopeAsOf, scope, backend, maxScope)
		if err != nil {
			return nil, err
		}

		var snapshotBuildID *uuid.UUID
		if backend == DeepReadBackendSnapshot {
			id, err := s.repo.GetActiveSnapshotBuildID(txCtx, tenantID, staffingScopeHierarchy, scopeAsOf)
			if err != nil {
				return nil, err
			}
			snapshotBuildID = &id
		}

		repoGroupBy := groupBy
		if repoGroupBy == StaffingGroupByJobLevel {
			repoGroupBy = StaffingGroupByNone
		}

		dbRes, err := s.repo.GetStaffingTimeToFillReport(txCtx, tenantID, from, to, nodeIDs, statuses, in.IncludeSystem, repoGroupBy)
		if err != nil {
			return nil, err
		}

		breakdown := dbRes.Breakdown
		if groupBy == StaffingGroupByJobLevel && dbRes.Summary.FilledCount > 0 {
			breakdown = []StaffingTimeToFillBreakdownRow{{
				Key:         "unknown",
				FilledCount: dbRes.Summary.FilledCount,
				AvgDays:     dbRes.Summary.AvgDays,
			}}
		}

		return &StaffingTimeToFillResult{
			TenantID:  tenantID,
			OrgNodeID: rootID,
			From:      from,
			To:        to,
			Scope:     scope,
			Summary:   dbRes.Summary,
			Breakdown: breakdown,
			Source: StaffingReportSource{
				DeepReadBackend: backend,
				SnapshotBuildID: snapshotBuildID,
			},
		}, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func resolveOrgNodeID(ctx context.Context, repo OrgRepository, tenantID uuid.UUID, orgNodeID *uuid.UUID) (uuid.UUID, error) {
	if orgNodeID != nil && *orgNodeID != uuid.Nil {
		return *orgNodeID, nil
	}
	id, err := repo.GetTenantRootNodeID(ctx, tenantID)
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func staffingResolveScopeNodeIDs(
	ctx context.Context,
	repo OrgRepository,
	tenantID uuid.UUID,
	rootNodeID uuid.UUID,
	asOf time.Time,
	scope StaffingScope,
	backend DeepReadBackend,
	maxScope int,
) ([]uuid.UUID, error) {
	if scope == StaffingScopeSelf {
		return []uuid.UUID{rootNodeID}, nil
	}

	rels, err := repo.ListDescendantsForExportAsOf(ctx, tenantID, staffingScopeHierarchy, rootNodeID, asOf, backend, nil, nil, maxScope+1)
	if err != nil {
		return nil, err
	}
	if len(rels) > maxScope {
		return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_REPORT_TOO_LARGE", "scope exceeds maximum", nil)
	}
	nodeIDs := make([]uuid.UUID, 0, len(rels))
	for _, rel := range rels {
		nodeIDs = append(nodeIDs, rel.NodeID)
	}
	if len(nodeIDs) == 0 {
		return []uuid.UUID{rootNodeID}, nil
	}
	return nodeIDs, nil
}

func normalizeStaffingLifecycleStatuses(raw []string) ([]string, error) {
	out := make([]string, 0, 3)
	seen := map[string]bool{}
	for _, v := range raw {
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "" {
			continue
		}
		if v == "rescinded" {
			return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "rescinded is not allowed in lifecycle_statuses", nil)
		}
		if v != "planned" && v != "active" && v != "inactive" {
			return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "lifecycle_statuses is invalid", nil)
		}
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	if len(out) == 0 {
		out = []string{"planned", "active"}
	}
	slices.Sort(out)
	return out, nil
}

func staffingTotalsFromAggregate(row StaffingAggregateRow) StaffingTotals {
	out := StaffingTotals{
		PositionsTotal: row.PositionsTotal,
		CapacityFTE:    row.CapacityFTE,
		OccupiedFTE:    row.OccupiedFTE,
		AvailableFTE:   row.AvailableFTE,
	}
	if out.CapacityFTE > 0 {
		out.FillRate = out.OccupiedFTE / out.CapacityFTE
	}
	return out
}

func staffingBreakdownFromAggregate(row StaffingAggregateRow) StaffingBreakdownRow {
	out := StaffingBreakdownRow{
		Key:            row.Key,
		PositionsTotal: row.PositionsTotal,
		CapacityFTE:    row.CapacityFTE,
		OccupiedFTE:    row.OccupiedFTE,
		AvailableFTE:   row.AvailableFTE,
	}
	if out.CapacityFTE > 0 {
		out.FillRate = out.OccupiedFTE / out.CapacityFTE
	}
	return out
}
