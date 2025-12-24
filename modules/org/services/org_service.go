package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sirupsen/logrus"

	"github.com/iota-uz/iota-sdk/modules/org/domain/events"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/pkg/constants"
	"github.com/iota-uz/iota-sdk/pkg/outbox"
	"github.com/iota-uz/iota-sdk/pkg/repo"
)

var endOfTime = time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
var orgOutboxTable = pgx.Identifier{"public", "org_outbox"}

type OrgRepository interface {
	ListHierarchyAsOf(ctx context.Context, tenantID uuid.UUID, hierarchyType string, asOf time.Time) ([]HierarchyNode, error)
	ListHierarchyAsOfRecursive(ctx context.Context, tenantID uuid.UUID, hierarchyType string, asOf time.Time) ([]HierarchyNode, error)

	// DEV-PLAN-029: deep read (ancestors/descendants) without recursive CTE.
	ListAncestorsAsOf(ctx context.Context, tenantID uuid.UUID, hierarchyType string, descendantNodeID uuid.UUID, asOf time.Time, backend DeepReadBackend) ([]DeepReadRelation, error)
	ListDescendantsAsOf(ctx context.Context, tenantID uuid.UUID, hierarchyType string, ancestorNodeID uuid.UUID, asOf time.Time, backend DeepReadBackend) ([]DeepReadRelation, error)

	// DEV-PLAN-028: inheritance read side helpers.
	ListNodeAttributesAsOf(ctx context.Context, tenantID uuid.UUID, nodeIDs []uuid.UUID, asOf time.Time) (map[uuid.UUID]OrgNodeAttributes, error)
	ListAttributeInheritanceRulesAsOf(ctx context.Context, tenantID uuid.UUID, hierarchyType string, asOf time.Time) ([]AttributeInheritanceRule, error)

	// DEV-PLAN-028: role read side helpers.
	ListRoles(ctx context.Context, tenantID uuid.UUID) ([]OrgRole, error)
	ListRoleAssignmentsAsOf(ctx context.Context, tenantID uuid.UUID, hierarchyType string, orgNodeID uuid.UUID, asOf time.Time, includeInherited bool, backend DeepReadBackend, roleCode *string, subjectType *string, subjectID *uuid.UUID) ([]RoleAssignmentRow, error)

	// DEV-PLAN-032: security group mappings + business object links.
	ListSecurityGroupMappings(ctx context.Context, tenantID uuid.UUID, filter SecurityGroupMappingListFilter) ([]SecurityGroupMappingRow, error)
	InsertSecurityGroupMapping(ctx context.Context, tenantID uuid.UUID, in SecurityGroupMappingInsert) (uuid.UUID, error)
	LockSecurityGroupMappingByID(ctx context.Context, tenantID uuid.UUID, id uuid.UUID) (SecurityGroupMappingRow, error)
	UpdateSecurityGroupMappingEndDate(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, endDate time.Time) error
	ListSecurityGroupMappingsForNodesAsOf(ctx context.Context, tenantID uuid.UUID, orgNodeIDs []uuid.UUID, asOf time.Time) ([]SecurityGroupMappingRow, error)

	ListOrgLinks(ctx context.Context, tenantID uuid.UUID, filter OrgLinkListFilter) ([]OrgLinkRow, error)
	InsertOrgLink(ctx context.Context, tenantID uuid.UUID, in OrgLinkInsert) (uuid.UUID, error)
	LockOrgLinkByID(ctx context.Context, tenantID uuid.UUID, id uuid.UUID) (OrgLinkRow, error)
	UpdateOrgLinkEndDate(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, endDate time.Time) error
	ListOrgLinksForNodeAsOf(ctx context.Context, tenantID uuid.UUID, orgNodeID uuid.UUID, asOf time.Time, limit int) ([]OrgLinkRow, error)

	ListSnapshotNodes(ctx context.Context, tenantID uuid.UUID, asOf time.Time, afterID *uuid.UUID, limit int) ([]SnapshotItem, error)
	ListSnapshotEdges(ctx context.Context, tenantID uuid.UUID, asOf time.Time, afterID *uuid.UUID, limit int) ([]SnapshotItem, error)
	ListSnapshotPositions(ctx context.Context, tenantID uuid.UUID, asOf time.Time, afterID *uuid.UUID, limit int) ([]SnapshotItem, error)
	ListSnapshotAssignments(ctx context.Context, tenantID uuid.UUID, asOf time.Time, afterID *uuid.UUID, limit int) ([]SnapshotItem, error)

	// DEV-PLAN-029: refresh/build tools for deep read derived tables.
	BuildDeepReadSnapshot(ctx context.Context, tenantID uuid.UUID, hierarchyType string, asOfDate time.Time, apply bool, sourceRequestID string) (DeepReadBuildResult, error)
	BuildDeepReadClosure(ctx context.Context, tenantID uuid.UUID, hierarchyType string, apply bool, sourceRequestID string) (DeepReadBuildResult, error)
	ActivateDeepReadClosureBuild(ctx context.Context, tenantID uuid.UUID, hierarchyType string, buildID uuid.UUID) (uuid.UUID, error)
	PruneDeepReadClosureBuilds(ctx context.Context, tenantID uuid.UUID, hierarchyType string, keep int) (DeepReadPruneResult, error)

	// DEV-PLAN-033: BI reporting snapshot build (consumes active snapshot build from 029).
	BuildOrgReportingNodes(ctx context.Context, tenantID uuid.UUID, hierarchyType string, asOfDate time.Time, includeSecurityGroups bool, includeLinks bool, apply bool, sourceRequestID string) (OrgReportingBuildResult, error)

	// DEV-PLAN-057: staffing reports (summary/vacancies/time-to-fill).
	GetActiveSnapshotBuildID(ctx context.Context, tenantID uuid.UUID, hierarchyType string, asOf time.Time) (uuid.UUID, error)
	GetStaffingSummaryReport(ctx context.Context, tenantID uuid.UUID, asOf time.Time, orgNodeIDs []uuid.UUID, lifecycleStatuses []string, includeSystem bool, groupBy StaffingGroupBy) (StaffingSummaryDBResult, error)
	ListStaffingVacanciesReport(ctx context.Context, tenantID uuid.UUID, asOf time.Time, orgNodeIDs []uuid.UUID, lifecycleStatuses []string, includeSystem bool, limit int, cursor *uuid.UUID) (StaffingVacanciesDBResult, error)
	GetStaffingTimeToFillReport(ctx context.Context, tenantID uuid.UUID, from time.Time, to time.Time, orgNodeIDs []uuid.UUID, lifecycleStatuses []string, includeSystem bool, groupBy StaffingGroupBy) (StaffingTimeToFillDBResult, error)

	GetOrgSettings(ctx context.Context, tenantID uuid.UUID) (OrgSettings, error)
	InsertAuditLog(ctx context.Context, tenantID uuid.UUID, log AuditLogInsert) (uuid.UUID, error)

	HasRoot(ctx context.Context, tenantID uuid.UUID) (bool, error)
	InsertNode(ctx context.Context, tenantID uuid.UUID, nodeType, code string, isRoot bool) (uuid.UUID, error)
	InsertNodeSlice(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID, slice NodeSliceInsert) (uuid.UUID, error)
	InsertEdge(ctx context.Context, tenantID uuid.UUID, hierarchyType string, parentID *uuid.UUID, childID uuid.UUID, effectiveDate, endDate time.Time) (uuid.UUID, error)
	NodeExistsAt(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID, hierarchyType string, asOf time.Time) (bool, error)
	GetNode(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID) (NodeRow, error)
	GetNodeIsRoot(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID) (bool, error)
	GetTenantRootNodeID(ctx context.Context, tenantID uuid.UUID) (uuid.UUID, error)

	// DEV-PLAN-033: Export/path helpers.
	ListOrgNodesAsOf(ctx context.Context, tenantID uuid.UUID, hierarchyType string, nodeIDs []uuid.UUID, asOf time.Time) ([]OrgNodeAsOfRow, error)
	ListDescendantsForExportAsOf(ctx context.Context, tenantID uuid.UUID, hierarchyType string, rootNodeID uuid.UUID, asOf time.Time, backend DeepReadBackend, afterNodeID *uuid.UUID, maxDepth *int, limit int) ([]DeepReadRelation, error)
	ResolveSecurityGroupKeysForNodesAsOf(ctx context.Context, tenantID uuid.UUID, hierarchyType string, nodeIDs []uuid.UUID, asOf time.Time, backend DeepReadBackend) (map[uuid.UUID][]string, error)
	ListOrgLinkSummariesForNodesAsOf(ctx context.Context, tenantID uuid.UUID, nodeIDs []uuid.UUID, asOf time.Time) (map[uuid.UUID][]OrgLinkSummary, error)

	GetNodeSliceAt(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID, asOf time.Time) (NodeSliceRow, error)
	LockNodeSliceAt(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID, asOf time.Time) (NodeSliceRow, error)
	TruncateNodeSlice(ctx context.Context, tenantID uuid.UUID, sliceID uuid.UUID, endDate time.Time) error
	NextNodeSliceEffectiveDate(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID, after time.Time) (time.Time, bool, error)

	GetEdgeAt(ctx context.Context, tenantID uuid.UUID, hierarchyType string, childID uuid.UUID, asOf time.Time) (EdgeRow, error)
	LockEdgeAt(ctx context.Context, tenantID uuid.UUID, hierarchyType string, childID uuid.UUID, asOf time.Time) (EdgeRow, error)
	LockEdgesInSubtree(ctx context.Context, tenantID uuid.UUID, hierarchyType string, asOf time.Time, movedPath string) ([]EdgeRow, error)
	TruncateEdge(ctx context.Context, tenantID uuid.UUID, edgeID uuid.UUID, endDate time.Time) error

	PositionExistsAt(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, asOf time.Time) (bool, error)
	InsertAutoPosition(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, orgNodeID uuid.UUID, code string, effectiveDate time.Time) error
	GetPositionOrgNodeAt(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, asOf time.Time) (uuid.UUID, error)
	LockPositionSliceAt(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, asOf time.Time) (PositionSliceRow, error)
	SumAllocatedFTEAt(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, asOf time.Time) (float64, error)
	InsertPosition(ctx context.Context, tenantID uuid.UUID, in PositionInsert) (uuid.UUID, error)
	GetPositionIsAutoCreated(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID) (bool, error)
	InsertPositionSlice(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, in PositionSliceInsert) (uuid.UUID, error)
	GetPositionSliceAt(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, asOf time.Time) (PositionSliceRow, error)
	LockPositionSliceStartingAt(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, effectiveDate time.Time) (PositionSliceRow, error)
	LockPositionSliceEndingAt(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, endDate time.Time) (PositionSliceRow, error)
	TruncatePositionSlice(ctx context.Context, tenantID uuid.UUID, sliceID uuid.UUID, endDate time.Time) error
	UpdatePositionSliceEffectiveDate(ctx context.Context, tenantID uuid.UUID, sliceID uuid.UUID, effectiveDate time.Time) error
	UpdatePositionSliceEndDate(ctx context.Context, tenantID uuid.UUID, sliceID uuid.UUID, endDate time.Time) error
	NextPositionSliceEffectiveDate(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, after time.Time) (time.Time, bool, error)
	ListPositionSlicesTimeline(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID) ([]PositionSliceRow, error)
	ListPositionsAsOf(ctx context.Context, tenantID uuid.UUID, asOf time.Time, filter PositionListFilter) ([]PositionViewRow, error)
	GetPositionAsOf(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, asOf time.Time) (PositionViewRow, error)
	DeletePositionSlicesFrom(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, from time.Time) error
	UpdatePositionSliceInPlace(ctx context.Context, tenantID uuid.UUID, sliceID uuid.UUID, patch PositionSliceInPlacePatch) error
	HasPositionSubordinatesAt(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, asOf time.Time) (bool, error)

	ListJobFamilyGroups(ctx context.Context, tenantID uuid.UUID) ([]JobFamilyGroupRow, error)
	CreateJobFamilyGroup(ctx context.Context, tenantID uuid.UUID, in JobFamilyGroupCreate) (JobFamilyGroupRow, error)
	UpdateJobFamilyGroup(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, in JobFamilyGroupUpdate) (JobFamilyGroupRow, error)

	ListJobFamilies(ctx context.Context, tenantID uuid.UUID, jobFamilyGroupID uuid.UUID) ([]JobFamilyRow, error)
	CreateJobFamily(ctx context.Context, tenantID uuid.UUID, in JobFamilyCreate) (JobFamilyRow, error)
	UpdateJobFamily(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, in JobFamilyUpdate) (JobFamilyRow, error)

	ListJobRoles(ctx context.Context, tenantID uuid.UUID, jobFamilyID uuid.UUID) ([]JobRoleRow, error)
	CreateJobRole(ctx context.Context, tenantID uuid.UUID, in JobRoleCreate) (JobRoleRow, error)
	UpdateJobRole(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, in JobRoleUpdate) (JobRoleRow, error)

	ListJobLevels(ctx context.Context, tenantID uuid.UUID, jobRoleID uuid.UUID) ([]JobLevelRow, error)
	CreateJobLevel(ctx context.Context, tenantID uuid.UUID, in JobLevelCreate) (JobLevelRow, error)
	UpdateJobLevel(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, in JobLevelUpdate) (JobLevelRow, error)

	ListJobProfiles(ctx context.Context, tenantID uuid.UUID, jobRoleID *uuid.UUID) ([]JobProfileRow, error)
	CreateJobProfile(ctx context.Context, tenantID uuid.UUID, in JobProfileCreate) (JobProfileRow, error)
	UpdateJobProfile(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, in JobProfileUpdate) (JobProfileRow, error)
	SetJobProfileAllowedLevels(ctx context.Context, tenantID uuid.UUID, jobProfileID uuid.UUID, in JobProfileAllowedLevelsSet) error
	GetJobProfileRef(ctx context.Context, tenantID uuid.UUID, jobProfileID uuid.UUID) (JobProfileRef, error)
	ResolveJobCatalogPathByCodes(ctx context.Context, tenantID uuid.UUID, codes JobCatalogCodes) (JobCatalogResolvedPath, error)
	JobProfileAllowsLevel(ctx context.Context, tenantID uuid.UUID, jobProfileID uuid.UUID, jobLevelID uuid.UUID) (bool, bool, error)
	JobLevelExistsUnderRole(ctx context.Context, tenantID uuid.UUID, jobRoleID uuid.UUID, jobLevelID uuid.UUID) (bool, error)
	ListJobProfileAllowedLevels(ctx context.Context, tenantID uuid.UUID, jobProfileID uuid.UUID) ([]uuid.UUID, error)

	LockAssignmentAt(ctx context.Context, tenantID uuid.UUID, assignmentID uuid.UUID, asOf time.Time) (AssignmentRow, error)
	TruncateAssignment(ctx context.Context, tenantID uuid.UUID, assignmentID uuid.UUID, endDate time.Time) error
	NextAssignmentEffectiveDate(ctx context.Context, tenantID uuid.UUID, assignmentID uuid.UUID, after time.Time) (time.Time, bool, error)

	InsertAssignment(ctx context.Context, tenantID uuid.UUID, assignment AssignmentInsert) (uuid.UUID, error)
	ListAssignmentsTimeline(ctx context.Context, tenantID uuid.UUID, subjectID uuid.UUID) ([]AssignmentViewRow, error)
	ListAssignmentsAsOf(ctx context.Context, tenantID uuid.UUID, subjectID uuid.UUID, asOf time.Time) ([]AssignmentViewRow, error)
	ResolvePersonUUIDByPernr(ctx context.Context, tenantID uuid.UUID, pernr string) (uuid.UUID, error)

	HasChildrenAt(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID, asOf time.Time) (bool, error)
	HasPositionsAt(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID, asOf time.Time) (bool, error)

	LockNodeSliceStartingAt(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID, effectiveDate time.Time) (NodeSliceRow, error)
	LockNodeSliceEndingAt(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID, endDate time.Time) (NodeSliceRow, error)
	UpdateNodeSliceInPlace(ctx context.Context, tenantID uuid.UUID, sliceID uuid.UUID, patch NodeSliceInPlacePatch) error
	UpdateNodeSliceEffectiveDate(ctx context.Context, tenantID uuid.UUID, sliceID uuid.UUID, effectiveDate time.Time) error
	UpdateNodeSliceEndDate(ctx context.Context, tenantID uuid.UUID, sliceID uuid.UUID, endDate time.Time) error
	DeleteNodeSlicesFrom(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID, from time.Time) error

	LockEdgeStartingAt(ctx context.Context, tenantID uuid.UUID, hierarchyType string, childID uuid.UUID, effectiveDate time.Time) (EdgeRow, error)
	DeleteEdgeByID(ctx context.Context, tenantID uuid.UUID, edgeID uuid.UUID) error
	DeleteEdgesFrom(ctx context.Context, tenantID uuid.UUID, hierarchyType string, childID uuid.UUID, from time.Time) error

	LockAssignmentByID(ctx context.Context, tenantID uuid.UUID, assignmentID uuid.UUID) (AssignmentRow, error)
	UpdateAssignmentInPlace(ctx context.Context, tenantID uuid.UUID, assignmentID uuid.UUID, patch AssignmentInPlacePatch) error
	UpdateAssignmentEndDate(ctx context.Context, tenantID uuid.UUID, assignmentID uuid.UUID, endDate time.Time) error

	UpsertPersonnelEvent(ctx context.Context, tenantID uuid.UUID, in PersonnelEventInsert) (PersonnelEventRow, bool, error)
}

type HierarchyNode struct {
	ID           uuid.UUID  `json:"id"`
	Code         string     `json:"code"`
	Name         string     `json:"name"`
	ParentID     *uuid.UUID `json:"parent_id"`
	Depth        int        `json:"depth"`
	DisplayOrder int        `json:"display_order"`
	Status       string     `json:"status"`
}

type NodeRow struct {
	ID     uuid.UUID
	Code   string
	IsRoot bool
	Type   string
}

type NodeSliceInsert struct {
	Name          string
	I18nNames     map[string]string
	Status        string
	LegalEntityID *uuid.UUID
	CompanyCode   *string
	LocationID    *uuid.UUID
	DisplayOrder  int
	ParentHint    *uuid.UUID
	ManagerUserID *int64
	EffectiveDate time.Time
	EndDate       time.Time
}

type NodeSliceRow struct {
	ID            uuid.UUID
	Name          string
	I18nNames     map[string]string
	Status        string
	LegalEntityID *uuid.UUID
	CompanyCode   *string
	LocationID    *uuid.UUID
	DisplayOrder  int
	ParentHint    *uuid.UUID
	ManagerUserID *int64
	EffectiveDate time.Time
	EndDate       time.Time
}

type NodeSliceInPlacePatch struct {
	Name          *string
	I18nNames     map[string]string
	Status        *string
	LegalEntityID **uuid.UUID
	CompanyCode   **string
	LocationID    **uuid.UUID
	DisplayOrder  *int
	ParentHint    **uuid.UUID
	ManagerUserID **int64
}

type EdgeRow struct {
	ID            uuid.UUID
	ParentNodeID  *uuid.UUID
	ChildNodeID   uuid.UUID
	Path          string
	Depth         int
	EffectiveDate time.Time
	EndDate       time.Time
}

type AssignmentInsert struct {
	PositionID      uuid.UUID
	SubjectType     string
	SubjectID       uuid.UUID
	Pernr           string
	AssignmentType  string
	IsPrimary       bool
	AllocatedFTE    float64
	EffectiveDate   time.Time
	EndDate         time.Time
	AssignmentSlice uuid.UUID
}

type AssignmentRow struct {
	ID             uuid.UUID
	PositionID     uuid.UUID
	SubjectType    string
	SubjectID      uuid.UUID
	Pernr          string
	AssignmentType string
	IsPrimary      bool
	AllocatedFTE   float64
	EffectiveDate  time.Time
	EndDate        time.Time
}

type AssignmentInPlacePatch struct {
	PositionID *uuid.UUID
	Pernr      *string
	SubjectID  *uuid.UUID
}

type AssignmentViewRow struct {
	ID             uuid.UUID  `json:"id"`
	PositionID     uuid.UUID  `json:"position_id"`
	OrgNodeID      uuid.UUID  `json:"org_node_id"`
	AssignmentType string     `json:"assignment_type"`
	IsPrimary      bool       `json:"is_primary"`
	AllocatedFTE   float64    `json:"allocated_fte"`
	EffectiveDate  time.Time  `json:"effective_date"`
	EndDate        time.Time  `json:"end_date"`
	PositionCode   *string    `json:"position_code,omitempty"`
	Pernr          *string    `json:"pernr,omitempty"`
	SubjectID      *uuid.UUID `json:"subject_id,omitempty"`
	StartEventType *string    `json:"start_event_type,omitempty"`
	EndEventType   *string    `json:"end_event_type,omitempty"`
}

type OrgService struct {
	repo      OrgRepository
	publisher outbox.Publisher
	cache     *orgCache
}

func NewOrgService(repo OrgRepository) *OrgService {
	var cache *orgCache
	if configuration.Use().OrgCacheEnabled {
		cache = newOrgCache()
	}
	return &OrgService{
		repo:      repo,
		publisher: outbox.NewPublisher(),
		cache:     cache,
	}
}

type ServiceError struct {
	Status  int
	Code    string
	Message string
	Cause   error
}

func (e *ServiceError) Error() string {
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

func (e *ServiceError) Unwrap() error { return e.Cause }

func newServiceError(status int, code, message string, cause error) *ServiceError {
	return &ServiceError{Status: status, Code: code, Message: message, Cause: cause}
}

func (s *OrgService) InvalidateTenantCacheWithReason(tenantID uuid.UUID, reason string) {
	if s == nil || s.cache == nil {
		return
	}
	s.cache.InvalidateTenant(tenantID)
	recordCacheInvalidate(reason)
}

func (s *OrgService) InvalidateTenantCache(tenantID uuid.UUID) {
	s.InvalidateTenantCacheWithReason(tenantID, "manual")
}

func (s *OrgService) GetHierarchyAsOf(ctx context.Context, tenantID uuid.UUID, hierarchyType string, asOf time.Time) ([]HierarchyNode, time.Time, error) {
	if tenantID == uuid.Nil {
		return nil, time.Time{}, fmt.Errorf("tenant_id is required")
	}
	if hierarchyType != "OrgUnit" {
		return nil, time.Time{}, fmt.Errorf("unsupported hierarchy type: %s", hierarchyType)
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}

	cacheKey := repo.CacheKey("org", "hierarchy", tenantID, hierarchyType, asOf.UTC().Format(time.RFC3339Nano))
	if s != nil && s.cache != nil {
		if cachedAny, ok := s.cache.Get(cacheKey); ok {
			if cached, ok := cachedAny.(cachedHierarchy); ok {
				recordCacheRequest("hierarchy", true)
				return cached.Nodes, cached.AsOf, nil
			}
		}
		recordCacheRequest("hierarchy", false)
	}

	nodes, err := inTx(ctx, tenantID, func(txCtx context.Context) ([]HierarchyNode, error) {
		if OrgReadStrategy() == "recursive" {
			return s.repo.ListHierarchyAsOfRecursive(txCtx, tenantID, hierarchyType, asOf)
		}
		return s.repo.ListHierarchyAsOf(txCtx, tenantID, hierarchyType, asOf)
	})
	if err != nil {
		return nil, time.Time{}, err
	}
	if s != nil && s.cache != nil {
		s.cache.Set(tenantID, cacheKey, cachedHierarchy{Nodes: nodes, AsOf: asOf})
	}
	return nodes, asOf, nil
}

type NodeAsOf struct {
	Node  NodeRow
	Slice NodeSliceRow
}

func (s *OrgService) GetNodeAsOf(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID, asOf time.Time) (*NodeAsOf, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(400, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if nodeID == uuid.Nil {
		return nil, newServiceError(400, "ORG_INVALID_QUERY", "node_id is required", nil)
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}

	return inTx(ctx, tenantID, func(txCtx context.Context) (*NodeAsOf, error) {
		node, err := s.repo.GetNode(txCtx, tenantID, nodeID)
		if err != nil {
			return nil, mapPgError(err)
		}
		slice, err := s.repo.GetNodeSliceAt(txCtx, tenantID, nodeID, asOf)
		if err != nil {
			return nil, mapPgError(err)
		}
		return &NodeAsOf{Node: node, Slice: slice}, nil
	})
}

type CreateNodeInput struct {
	Code          string
	Name          string
	ParentID      *uuid.UUID
	EffectiveDate time.Time
	I18nNames     map[string]string
	Status        string
	DisplayOrder  int
	LegalEntityID *uuid.UUID
	CompanyCode   *string
	LocationID    *uuid.UUID
	ManagerUserID *int64
}

type CreateNodeResult struct {
	NodeID          uuid.UUID
	EdgeID          uuid.UUID
	EffectiveDate   time.Time
	EndDate         time.Time
	GeneratedEvents []events.OrgEventV1
}

func (s *OrgService) CreateNode(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in CreateNodeInput) (*CreateNodeResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(400, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	txTime := time.Now().UTC()
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	if in.Code == "" || in.Name == "" || in.EffectiveDate.IsZero() {
		return nil, newServiceError(400, "ORG_INVALID_BODY", "code/name/effective_date are required", nil)
	}
	if in.Status == "" {
		in.Status = "active"
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*CreateNodeResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}
		freeze, err := s.freezeCheck(settings, txTime, in.EffectiveDate)
		if err != nil {
			return nil, err
		}

		hierarchyType := "OrgUnit"
		if in.ParentID == nil {
			hasRoot, err := s.repo.HasRoot(txCtx, tenantID)
			if err != nil {
				return nil, err
			}
			if hasRoot {
				return nil, newServiceError(409, "ORG_OVERLAP", "root already exists", nil)
			}
		} else {
			exists, err := s.repo.NodeExistsAt(txCtx, tenantID, *in.ParentID, hierarchyType, in.EffectiveDate)
			if err != nil {
				return nil, err
			}
			if !exists {
				return nil, newServiceError(422, "ORG_PARENT_NOT_FOUND", "parent not found at effective_date", nil)
			}
		}

		nodeID, err := s.repo.InsertNode(txCtx, tenantID, hierarchyType, in.Code, in.ParentID == nil)
		if err != nil {
			return nil, mapPgError(err)
		}

		_, err = s.repo.InsertNodeSlice(txCtx, tenantID, nodeID, NodeSliceInsert{
			Name:          in.Name,
			I18nNames:     in.I18nNames,
			Status:        in.Status,
			LegalEntityID: in.LegalEntityID,
			CompanyCode:   in.CompanyCode,
			LocationID:    in.LocationID,
			DisplayOrder:  in.DisplayOrder,
			ParentHint:    in.ParentID,
			ManagerUserID: in.ManagerUserID,
			EffectiveDate: in.EffectiveDate,
			EndDate:       endOfTime,
		})
		if err != nil {
			return nil, mapPgError(err)
		}

		edgeID, err := s.repo.InsertEdge(txCtx, tenantID, hierarchyType, in.ParentID, nodeID, in.EffectiveDate, endOfTime)
		if err != nil {
			return nil, mapPgError(err)
		}

		_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
			RequestID:       requestID,
			TransactionTime: txTime,
			InitiatorID:     initiatorID,
			ChangeType:      "node.created",
			EntityType:      "org_node",
			EntityID:        nodeID,
			EffectiveDate:   in.EffectiveDate,
			EndDate:         endOfTime,
			OldValues:       nil,
			NewValues: map[string]any{
				"org_node_id":     nodeID.String(),
				"type":            hierarchyType,
				"code":            in.Code,
				"is_root":         in.ParentID == nil,
				"name":            in.Name,
				"i18n_names":      in.I18nNames,
				"status":          in.Status,
				"display_order":   in.DisplayOrder,
				"parent_hint":     in.ParentID,
				"manager_user_id": in.ManagerUserID,
				"effective_date":  in.EffectiveDate.UTC().Format(time.RFC3339),
				"end_date":        endOfTime.UTC().Format(time.RFC3339),
			},
			Operation:       "Create",
			FreezeMode:      freeze.Mode,
			FreezeViolation: freeze.Violation,
			FreezeCutoffUTC: freeze.CutoffUTC,
			AffectedAtUTC:   in.EffectiveDate,
		})
		if err != nil {
			return nil, err
		}

		_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
			RequestID:       requestID,
			TransactionTime: txTime,
			InitiatorID:     initiatorID,
			ChangeType:      "edge.created",
			EntityType:      "org_edge",
			EntityID:        edgeID,
			EffectiveDate:   in.EffectiveDate,
			EndDate:         endOfTime,
			OldValues:       nil,
			NewValues: map[string]any{
				"edge_id":        edgeID.String(),
				"hierarchy_type": hierarchyType,
				"parent_node_id": in.ParentID,
				"child_node_id":  nodeID.String(),
				"effective_date": in.EffectiveDate.UTC().Format(time.RFC3339),
				"end_date":       endOfTime.UTC().Format(time.RFC3339),
			},
			Operation:       "Create",
			FreezeMode:      freeze.Mode,
			FreezeViolation: freeze.Violation,
			FreezeCutoffUTC: freeze.CutoffUTC,
			AffectedAtUTC:   in.EffectiveDate,
		})
		if err != nil {
			return nil, err
		}

		evNode := buildEventV1(requestID, tenantID, initiatorID, txTime, "node.created", "org_node", nodeID, in.EffectiveDate, endOfTime)
		evEdge := buildEventV1(requestID, tenantID, initiatorID, txTime, "edge.created", "org_edge", edgeID, in.EffectiveDate, endOfTime)
		res := &CreateNodeResult{
			NodeID:        nodeID,
			EdgeID:        edgeID,
			EffectiveDate: in.EffectiveDate,
			EndDate:       endOfTime,
			GeneratedEvents: []events.OrgEventV1{
				evNode,
				evEdge,
			},
		}
		if err := s.enqueueOutboxEvents(txCtx, tenantID, res.GeneratedEvents); err != nil {
			return nil, err
		}
		return res, nil
	})
	if err != nil {
		return nil, err
	}
	if !shouldSkipCacheInvalidation(ctx) {
		s.InvalidateTenantCacheWithReason(tenantID, "write_commit")
	}
	return written, nil
}

func outboxTopicForEntityType(entityType string) string {
	if entityType == "org_assignment" {
		return events.TopicOrgAssignmentChangedV1
	}
	if entityType == "org_personnel_event" {
		return events.TopicOrgPersonnelEventChangedV1
	}
	return events.TopicOrgChangedV1
}

func (s *OrgService) enqueueOutboxEvents(txCtx context.Context, tenantID uuid.UUID, evs []events.OrgEventV1) error {
	if shouldSkipOutboxEnqueue(txCtx) {
		return nil
	}
	if s == nil || s.publisher == nil || len(evs) == 0 {
		return nil
	}
	tx, err := composables.UseTx(txCtx)
	if err != nil {
		return err
	}
	for _, ev := range evs {
		payload, err := json.Marshal(ev)
		if err != nil {
			return newServiceError(500, "ORG_OUTBOX_ENCODE_FAILED", "failed to encode outbox payload", err)
		}
		_, err = s.publisher.Enqueue(txCtx, tx, orgOutboxTable, outbox.Message{
			TenantID: tenantID,
			Topic:    outboxTopicForEntityType(ev.EntityType),
			EventID:  ev.EventID,
			Payload:  payload,
		})
		if err != nil {
			return newServiceError(500, "ORG_OUTBOX_ENQUEUE_FAILED", "failed to enqueue outbox message", err)
		}
	}
	return nil
}

type UpdateNodeInput struct {
	NodeID        uuid.UUID
	EffectiveDate time.Time
	Name          *string
	I18nNames     map[string]string
	Status        *string
	DisplayOrder  *int
	LegalEntityID **uuid.UUID
	CompanyCode   **string
	LocationID    **uuid.UUID
	ManagerUserID **int64
}

type UpdateNodeResult struct {
	NodeID          uuid.UUID
	EffectiveDate   time.Time
	EndDate         time.Time
	GeneratedEvents []events.OrgEventV1
}

func (s *OrgService) UpdateNode(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in UpdateNodeInput) (*UpdateNodeResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(400, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	txTime := time.Now().UTC()
	if in.NodeID == uuid.Nil || in.EffectiveDate.IsZero() {
		return nil, newServiceError(400, "ORG_INVALID_BODY", "id/effective_date are required", nil)
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*UpdateNodeResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}
		freeze, err := s.freezeCheck(settings, txTime, in.EffectiveDate)
		if err != nil {
			return nil, err
		}

		current, err := s.repo.LockNodeSliceAt(txCtx, tenantID, in.NodeID, in.EffectiveDate)
		if err != nil {
			return nil, mapPgError(err)
		}
		if current.EffectiveDate.Equal(in.EffectiveDate) {
			return nil, newServiceError(422, "ORG_USE_CORRECT", "use correct for in-place updates", nil)
		}

		next, hasNext, err := s.repo.NextNodeSliceEffectiveDate(txCtx, tenantID, in.NodeID, in.EffectiveDate)
		if err != nil {
			return nil, err
		}

		newEnd := current.EndDate
		if hasNext && next.Before(newEnd) {
			newEnd = next
		}

		if err := s.repo.TruncateNodeSlice(txCtx, tenantID, current.ID, in.EffectiveDate); err != nil {
			return nil, mapPgError(err)
		}

		nextSlice := current
		nextSlice.EffectiveDate = in.EffectiveDate
		nextSlice.EndDate = newEnd
		if in.Name != nil {
			nextSlice.Name = strings.TrimSpace(*in.Name)
		}
		if in.I18nNames != nil {
			nextSlice.I18nNames = in.I18nNames
		}
		if in.Status != nil {
			nextSlice.Status = *in.Status
		}
		if in.DisplayOrder != nil {
			nextSlice.DisplayOrder = *in.DisplayOrder
		}
		if in.LegalEntityID != nil {
			nextSlice.LegalEntityID = *in.LegalEntityID
		}
		if in.CompanyCode != nil {
			nextSlice.CompanyCode = *in.CompanyCode
		}
		if in.LocationID != nil {
			nextSlice.LocationID = *in.LocationID
		}
		if in.ManagerUserID != nil {
			nextSlice.ManagerUserID = *in.ManagerUserID
		}

		_, err = s.repo.InsertNodeSlice(txCtx, tenantID, in.NodeID, NodeSliceInsert{
			Name:          nextSlice.Name,
			I18nNames:     nextSlice.I18nNames,
			Status:        nextSlice.Status,
			LegalEntityID: nextSlice.LegalEntityID,
			CompanyCode:   nextSlice.CompanyCode,
			LocationID:    nextSlice.LocationID,
			DisplayOrder:  nextSlice.DisplayOrder,
			ParentHint:    nextSlice.ParentHint,
			ManagerUserID: nextSlice.ManagerUserID,
			EffectiveDate: in.EffectiveDate,
			EndDate:       newEnd,
		})
		if err != nil {
			return nil, mapPgError(err)
		}

		_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
			RequestID:       requestID,
			TransactionTime: txTime,
			InitiatorID:     initiatorID,
			ChangeType:      "node.updated",
			EntityType:      "org_node",
			EntityID:        in.NodeID,
			EffectiveDate:   in.EffectiveDate,
			EndDate:         newEnd,
			OldValues: map[string]any{
				"slice_id":        current.ID.String(),
				"org_node_id":     in.NodeID.String(),
				"name":            current.Name,
				"i18n_names":      current.I18nNames,
				"status":          current.Status,
				"display_order":   current.DisplayOrder,
				"parent_hint":     current.ParentHint,
				"manager_user_id": current.ManagerUserID,
				"effective_date":  current.EffectiveDate.UTC().Format(time.RFC3339),
				"end_date":        current.EndDate.UTC().Format(time.RFC3339),
			},
			NewValues: map[string]any{
				"org_node_id":     in.NodeID.String(),
				"name":            nextSlice.Name,
				"i18n_names":      nextSlice.I18nNames,
				"status":          nextSlice.Status,
				"display_order":   nextSlice.DisplayOrder,
				"parent_hint":     nextSlice.ParentHint,
				"manager_user_id": nextSlice.ManagerUserID,
				"effective_date":  in.EffectiveDate.UTC().Format(time.RFC3339),
				"end_date":        newEnd.UTC().Format(time.RFC3339),
			},
			Operation:       "Update",
			FreezeMode:      freeze.Mode,
			FreezeViolation: freeze.Violation,
			FreezeCutoffUTC: freeze.CutoffUTC,
			AffectedAtUTC:   in.EffectiveDate,
		})
		if err != nil {
			return nil, err
		}

		ev := buildEventV1(requestID, tenantID, initiatorID, txTime, "node.updated", "org_node", in.NodeID, in.EffectiveDate, newEnd)
		res := &UpdateNodeResult{
			NodeID:        in.NodeID,
			EffectiveDate: in.EffectiveDate,
			EndDate:       newEnd,
			GeneratedEvents: []events.OrgEventV1{
				ev,
			},
		}
		if err := s.enqueueOutboxEvents(txCtx, tenantID, res.GeneratedEvents); err != nil {
			return nil, err
		}
		return res, nil
	})
	if err != nil {
		return nil, err
	}
	if !shouldSkipCacheInvalidation(ctx) {
		s.InvalidateTenantCacheWithReason(tenantID, "write_commit")
	}
	return written, nil
}

type MoveNodeInput struct {
	NodeID        uuid.UUID
	NewParentID   uuid.UUID
	EffectiveDate time.Time
}

type MoveNodeResult struct {
	EdgeID          uuid.UUID
	EffectiveDate   time.Time
	EndDate         time.Time
	GeneratedEvents []events.OrgEventV1
}

func (s *OrgService) MoveNode(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in MoveNodeInput) (*MoveNodeResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(400, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	txTime := time.Now().UTC()
	if in.NodeID == uuid.Nil || in.NewParentID == uuid.Nil || in.EffectiveDate.IsZero() {
		return nil, newServiceError(400, "ORG_INVALID_BODY", "id/new_parent_id/effective_date are required", nil)
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*MoveNodeResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}
		freeze, err := s.freezeCheck(settings, txTime, in.EffectiveDate)
		if err != nil {
			return nil, err
		}

		isRoot, err := s.repo.GetNodeIsRoot(txCtx, tenantID, in.NodeID)
		if err != nil {
			return nil, err
		}
		if isRoot {
			return nil, newServiceError(422, "ORG_CANNOT_MOVE_ROOT", "cannot move root", nil)
		}

		hierarchyType := "OrgUnit"
		parentExists, err := s.repo.NodeExistsAt(txCtx, tenantID, in.NewParentID, hierarchyType, in.EffectiveDate)
		if err != nil {
			return nil, err
		}
		if !parentExists {
			return nil, newServiceError(422, "ORG_PARENT_NOT_FOUND", "new_parent_id not found at effective_date", nil)
		}

		movedEdge, err := s.repo.LockEdgeAt(txCtx, tenantID, hierarchyType, in.NodeID, in.EffectiveDate)
		if err != nil {
			return nil, mapPgError(err)
		}
		if movedEdge.EffectiveDate.Equal(in.EffectiveDate) {
			return nil, newServiceError(422, "ORG_USE_CORRECT_MOVE", "use correct-move for in-place updates", nil)
		}

		subtree, err := s.repo.LockEdgesInSubtree(txCtx, tenantID, hierarchyType, in.EffectiveDate, movedEdge.Path)
		if err != nil {
			return nil, err
		}
		for _, e := range subtree {
			if e.EffectiveDate.Equal(in.EffectiveDate) {
				return nil, newServiceError(422, "ORG_USE_CORRECT_MOVE", "subtree contains edges requiring correct-move at effective_date", nil)
			}
		}

		if err := s.repo.TruncateEdge(txCtx, tenantID, movedEdge.ID, in.EffectiveDate); err != nil {
			return nil, mapPgError(err)
		}
		newEdgeID, err := s.repo.InsertEdge(txCtx, tenantID, hierarchyType, &in.NewParentID, in.NodeID, in.EffectiveDate, movedEdge.EndDate)
		if err != nil {
			return nil, mapPgError(err)
		}

		currentSlice, err := s.repo.LockNodeSliceAt(txCtx, tenantID, in.NodeID, in.EffectiveDate)
		if err != nil {
			return nil, mapPgError(err)
		}
		if currentSlice.EffectiveDate.Equal(in.EffectiveDate) {
			return nil, newServiceError(422, "ORG_USE_CORRECT_MOVE", "use correct for in-place parent_hint updates", nil)
		}
		next, hasNext, err := s.repo.NextNodeSliceEffectiveDate(txCtx, tenantID, in.NodeID, in.EffectiveDate)
		if err != nil {
			return nil, err
		}
		newEnd := currentSlice.EndDate
		if hasNext && next.Before(newEnd) {
			newEnd = next
		}
		if err := s.repo.TruncateNodeSlice(txCtx, tenantID, currentSlice.ID, in.EffectiveDate); err != nil {
			return nil, mapPgError(err)
		}
		_, err = s.repo.InsertNodeSlice(txCtx, tenantID, in.NodeID, NodeSliceInsert{
			Name:          currentSlice.Name,
			I18nNames:     currentSlice.I18nNames,
			Status:        currentSlice.Status,
			LegalEntityID: currentSlice.LegalEntityID,
			CompanyCode:   currentSlice.CompanyCode,
			LocationID:    currentSlice.LocationID,
			DisplayOrder:  currentSlice.DisplayOrder,
			ParentHint:    &in.NewParentID,
			ManagerUserID: currentSlice.ManagerUserID,
			EffectiveDate: in.EffectiveDate,
			EndDate:       newEnd,
		})
		if err != nil {
			return nil, mapPgError(err)
		}

		for _, e := range subtree {
			if e.ChildNodeID == in.NodeID {
				continue
			}
			if err := s.repo.TruncateEdge(txCtx, tenantID, e.ID, in.EffectiveDate); err != nil {
				return nil, mapPgError(err)
			}
			if _, err := s.repo.InsertEdge(txCtx, tenantID, hierarchyType, e.ParentNodeID, e.ChildNodeID, in.EffectiveDate, e.EndDate); err != nil {
				return nil, mapPgError(err)
			}
		}

		newEdge := EdgeRow{
			ID:            newEdgeID,
			ParentNodeID:  &in.NewParentID,
			ChildNodeID:   in.NodeID,
			EffectiveDate: in.EffectiveDate,
			EndDate:       movedEdge.EndDate,
		}

		_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
			RequestID:       requestID,
			TransactionTime: txTime,
			InitiatorID:     initiatorID,
			ChangeType:      "edge.updated",
			EntityType:      "org_edge",
			EntityID:        newEdgeID,
			EffectiveDate:   in.EffectiveDate,
			EndDate:         movedEdge.EndDate,
			OldValues: map[string]any{
				"edge_id":        movedEdge.ID.String(),
				"parent_node_id": movedEdge.ParentNodeID,
				"child_node_id":  movedEdge.ChildNodeID.String(),
				"effective_date": movedEdge.EffectiveDate.UTC().Format(time.RFC3339),
				"end_date":       movedEdge.EndDate.UTC().Format(time.RFC3339),
				"path":           movedEdge.Path,
				"depth":          movedEdge.Depth,
			},
			NewValues: map[string]any{
				"edge_id":        newEdgeID.String(),
				"parent_node_id": in.NewParentID.String(),
				"child_node_id":  in.NodeID.String(),
				"effective_date": in.EffectiveDate.UTC().Format(time.RFC3339),
				"end_date":       movedEdge.EndDate.UTC().Format(time.RFC3339),
			},
			Operation:       "Move",
			FreezeMode:      freeze.Mode,
			FreezeViolation: freeze.Violation,
			FreezeCutoffUTC: freeze.CutoffUTC,
			AffectedAtUTC:   in.EffectiveDate,
		})
		if err != nil {
			return nil, err
		}

		ev := buildEventV1(requestID, tenantID, initiatorID, txTime, "edge.updated", "org_edge", newEdge.ID, newEdge.EffectiveDate, newEdge.EndDate)
		res := &MoveNodeResult{
			EdgeID:        newEdge.ID,
			EffectiveDate: newEdge.EffectiveDate,
			EndDate:       newEdge.EndDate,
			GeneratedEvents: []events.OrgEventV1{
				ev,
			},
		}
		if err := s.enqueueOutboxEvents(txCtx, tenantID, res.GeneratedEvents); err != nil {
			return nil, err
		}
		return res, nil
	})
	if err != nil {
		return nil, err
	}
	if !shouldSkipCacheInvalidation(ctx) {
		s.InvalidateTenantCacheWithReason(tenantID, "write_commit")
	}
	return written, nil
}

type CreateAssignmentInput struct {
	Pernr          string
	EffectiveDate  time.Time
	ReasonCode     string
	ReasonNote     *string
	AssignmentType string
	AllocatedFTE   float64
	PositionID     *uuid.UUID
	OrgNodeID      *uuid.UUID
	SubjectID      *uuid.UUID
}

type CreateAssignmentResult struct {
	AssignmentID    uuid.UUID
	PositionID      uuid.UUID
	SubjectID       uuid.UUID
	EffectiveDate   time.Time
	EndDate         time.Time
	GeneratedEvents []events.OrgEventV1
}

func (s *OrgService) CreateAssignment(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in CreateAssignmentInput) (*CreateAssignmentResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(400, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	txTime := time.Now().UTC()
	in.Pernr = strings.TrimSpace(in.Pernr)
	if in.Pernr == "" || in.EffectiveDate.IsZero() {
		return nil, newServiceError(400, "ORG_INVALID_BODY", "pernr/effective_date are required", nil)
	}
	allocatedFTE := in.AllocatedFTE
	if allocatedFTE < 0 {
		return nil, newServiceError(400, "ORG_INVALID_BODY", "allocated_fte must be > 0", nil)
	}
	if allocatedFTE == 0 {
		allocatedFTE = 1.0
	}

	assignmentType := strings.TrimSpace(in.AssignmentType)
	if assignmentType == "" {
		assignmentType = "primary"
	}
	switch assignmentType {
	case "primary", "matrix", "dotted":
	default:
		return nil, newServiceError(400, "ORG_INVALID_BODY", "assignment_type is invalid", nil)
	}
	if assignmentType != "primary" && !configuration.Use().EnableOrgExtendedAssignmentTypes {
		return nil, newServiceError(422, "ORG_ASSIGNMENT_TYPE_DISABLED", "assignment_type is disabled", nil)
	}
	isPrimary := assignmentType == "primary"

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*CreateAssignmentResult, error) {
		personUUID, err := s.repo.ResolvePersonUUIDByPernr(txCtx, tenantID, in.Pernr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, newServiceError(http.StatusNotFound, "ORG_PERSON_NOT_FOUND", "pernr not found", nil)
			}
			return nil, err
		}
		if in.SubjectID != nil && *in.SubjectID != personUUID {
			return nil, newServiceError(422, "ORG_SUBJECT_MISMATCH", "subject_id does not match persons mapping", nil)
		}

		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}
		reasonCode, reasonInfo, svcErr := normalizeReasonCode(settings, in.ReasonCode)
		if svcErr != nil {
			logReasonCodeRejected(txCtx, tenantID, requestID, "assignment.created", reasonInfo, svcErr)
			return nil, svcErr
		}
		freeze, err := s.freezeCheck(settings, txTime, in.EffectiveDate)
		if err != nil {
			maybeLogFrozenWindowRejected(txCtx, tenantID, requestID, initiatorID, "assignment.created", "org_assignment", uuid.Nil, in.EffectiveDate, freeze, err, logrus.Fields{
				"pernr":           in.Pernr,
				"subject_id":      personUUID.String(),
				"assignment_type": assignmentType,
				"operation":       "Create",
			})
			return nil, err
		}

		var positionID uuid.UUID
		if in.PositionID != nil {
			positionID = *in.PositionID
			exists, err := s.repo.PositionExistsAt(txCtx, tenantID, positionID, in.EffectiveDate)
			if err != nil {
				return nil, err
			}
			if !exists {
				return nil, newServiceError(422, "ORG_POSITION_NOT_FOUND_AT_DATE", "position_id not found at effective_date", nil)
			}
		} else {
			if in.OrgNodeID == nil || *in.OrgNodeID == uuid.Nil {
				return nil, newServiceError(400, "ORG_INVALID_BODY", "position_id or org_node_id is required", nil)
			}
			if !configuration.Use().EnableOrgAutoPositions {
				return nil, newServiceError(422, "ORG_AUTO_POSITION_DISABLED", "auto position creation is disabled", nil)
			}
			hierarchyType := "OrgUnit"
			exists, err := s.repo.NodeExistsAt(txCtx, tenantID, *in.OrgNodeID, hierarchyType, in.EffectiveDate)
			if err != nil {
				return nil, err
			}
			if !exists {
				return nil, newServiceError(422, "ORG_NODE_NOT_FOUND_AT_DATE", "org_node_id not found at effective_date", nil)
			}
			positionID, err = autoPositionID(tenantID, *in.OrgNodeID, personUUID)
			if err != nil {
				return nil, err
			}
			code := autoPositionCode(positionID)
			if err := s.repo.InsertAutoPosition(txCtx, tenantID, positionID, *in.OrgNodeID, code, in.EffectiveDate); err != nil {
				return nil, mapPgError(err)
			}
		}

		var posSlice PositionSliceRow
		if isPrimary {
			posSlice, err = s.repo.LockPositionSliceAt(txCtx, tenantID, positionID, in.EffectiveDate)
		} else {
			posSlice, err = s.repo.GetPositionSliceAt(txCtx, tenantID, positionID, in.EffectiveDate)
		}
		if err != nil {
			return nil, mapPgError(err)
		}

		if isPrimary {
			occupied, err := s.repo.SumAllocatedFTEAt(txCtx, tenantID, positionID, in.EffectiveDate)
			if err != nil {
				return nil, err
			}
			if occupied+allocatedFTE > posSlice.CapacityFTE {
				return nil, newServiceError(422, "ORG_POSITION_OVER_CAPACITY", "position capacity exceeded", nil)
			}
		}

		restrictionsMode := normalizeValidationMode(settings.PositionRestrictionsValidationMode)
		var restrictionsShadowErr *ServiceError
		if restrictionsMode != "disabled" {
			isAutoCreated, err := s.repo.GetPositionIsAutoCreated(txCtx, tenantID, positionID)
			if err != nil {
				return nil, mapPgError(err)
			}
			restrictionsJSON, err := extractRestrictionsFromProfile(posSlice.Profile)
			if err != nil {
				if restrictionsMode == "enforce" {
					maybeLogModeRejected(txCtx, "org.position_restrictions.rejected", tenantID, requestID, initiatorID, "assignment.created", "org_assignment", uuid.Nil, in.EffectiveDate, restrictionsMode, err, logrus.Fields{
						"position_id": positionID.String(),
						"operation":   "Create",
					})
					return nil, err
				}
				var svcErr *ServiceError
				if !errors.As(err, &svcErr) {
					return nil, err
				}
				restrictionsShadowErr = svcErr
			} else {
				parsedRestrictions, err := parsePositionRestrictions(restrictionsJSON)
				if err != nil {
					if restrictionsMode == "enforce" {
						maybeLogModeRejected(txCtx, "org.position_restrictions.rejected", tenantID, requestID, initiatorID, "assignment.created", "org_assignment", uuid.Nil, in.EffectiveDate, restrictionsMode, err, logrus.Fields{
							"position_id": positionID.String(),
							"operation":   "Create",
						})
						return nil, err
					}
					var svcErr *ServiceError
					if !errors.As(err, &svcErr) {
						return nil, err
					}
					restrictionsShadowErr = svcErr
				} else if err := s.validatePositionRestrictionsAgainstSlice(txCtx, tenantID, isAutoCreated, posSlice, parsedRestrictions); err != nil {
					if restrictionsMode == "enforce" {
						maybeLogModeRejected(txCtx, "org.position_restrictions.rejected", tenantID, requestID, initiatorID, "assignment.created", "org_assignment", uuid.Nil, in.EffectiveDate, restrictionsMode, err, logrus.Fields{
							"position_id": positionID.String(),
							"operation":   "Create",
						})
						return nil, err
					}
					var svcErr *ServiceError
					if !errors.As(err, &svcErr) {
						return nil, err
					}
					restrictionsShadowErr = svcErr
				}
			}
		}

		assignmentID, err := s.repo.InsertAssignment(txCtx, tenantID, AssignmentInsert{
			PositionID:     positionID,
			SubjectType:    "person",
			SubjectID:      personUUID,
			Pernr:          in.Pernr,
			AssignmentType: assignmentType,
			IsPrimary:      isPrimary,
			AllocatedFTE:   allocatedFTE,
			EffectiveDate:  in.EffectiveDate,
			EndDate:        endOfTime,
		})
		if err != nil {
			return nil, mapPgError(err)
		}
		if !isPrimary {
			logWithFields(txCtx, logrus.InfoLevel, "org.assignment.created", logrus.Fields{
				"tenant_id":       tenantID.String(),
				"assignment_id":   assignmentID.String(),
				"position_id":     positionID.String(),
				"pernr":           in.Pernr,
				"subject_id":      personUUID.String(),
				"assignment_type": assignmentType,
				"effective_date":  in.EffectiveDate.UTC().Format(time.RFC3339),
			})
		}

		_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
			RequestID:       requestID,
			TransactionTime: txTime,
			InitiatorID:     initiatorID,
			ChangeType:      "assignment.created",
			EntityType:      "org_assignment",
			EntityID:        assignmentID,
			EffectiveDate:   in.EffectiveDate,
			EndDate:         endOfTime,
			OldValues:       nil,
			NewValues: map[string]any{
				"assignment_id":   assignmentID.String(),
				"position_id":     positionID.String(),
				"subject_type":    "person",
				"subject_id":      personUUID.String(),
				"pernr":           in.Pernr,
				"assignment_type": assignmentType,
				"is_primary":      isPrimary,
				"allocated_fte":   allocatedFTE,
				"effective_date":  in.EffectiveDate.UTC().Format(time.RFC3339),
				"end_date":        endOfTime.UTC().Format(time.RFC3339),
			},
			Meta: func() map[string]any {
				meta := map[string]any{
					"reason_code": reasonCode,
					"reason_note": in.ReasonNote,
				}
				addReasonCodeMeta(meta, reasonInfo)
				if restrictionsShadowErr != nil {
					meta["position_restrictions_validation_mode"] = restrictionsMode
					meta["position_restrictions_validation_error_code"] = restrictionsShadowErr.Code
				}
				return meta
			}(),
			Operation:       "Create",
			FreezeMode:      freeze.Mode,
			FreezeViolation: freeze.Violation,
			FreezeCutoffUTC: freeze.CutoffUTC,
			AffectedAtUTC:   in.EffectiveDate,
		})
		if err != nil {
			return nil, err
		}

		ev := buildEventV1(requestID, tenantID, initiatorID, txTime, "assignment.created", "org_assignment", assignmentID, in.EffectiveDate, endOfTime)
		res := &CreateAssignmentResult{
			AssignmentID:  assignmentID,
			PositionID:    positionID,
			SubjectID:     personUUID,
			EffectiveDate: in.EffectiveDate,
			EndDate:       endOfTime,
			GeneratedEvents: []events.OrgEventV1{
				ev,
			},
		}
		if err := s.enqueueOutboxEvents(txCtx, tenantID, res.GeneratedEvents); err != nil {
			return nil, err
		}
		return res, nil
	})
	if err != nil {
		return nil, err
	}
	if !shouldSkipCacheInvalidation(ctx) {
		s.InvalidateTenantCacheWithReason(tenantID, "write_commit")
	}
	return written, nil
}

type UpdateAssignmentInput struct {
	AssignmentID  uuid.UUID
	EffectiveDate time.Time
	ReasonCode    string
	ReasonNote    *string
	AllocatedFTE  *float64
	PositionID    *uuid.UUID
	OrgNodeID     *uuid.UUID
}

type UpdateAssignmentResult struct {
	AssignmentID    uuid.UUID
	PositionID      uuid.UUID
	EffectiveDate   time.Time
	EndDate         time.Time
	GeneratedEvents []events.OrgEventV1
}

func (s *OrgService) UpdateAssignment(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in UpdateAssignmentInput) (*UpdateAssignmentResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(400, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	txTime := time.Now().UTC()
	if in.AssignmentID == uuid.Nil || in.EffectiveDate.IsZero() {
		return nil, newServiceError(400, "ORG_INVALID_BODY", "id/effective_date are required", nil)
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*UpdateAssignmentResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}
		reasonCode, reasonInfo, svcErr := normalizeReasonCode(settings, in.ReasonCode)
		if svcErr != nil {
			logReasonCodeRejected(txCtx, tenantID, requestID, "assignment.updated", reasonInfo, svcErr)
			return nil, svcErr
		}
		restrictionsMode := normalizeValidationMode(settings.PositionRestrictionsValidationMode)
		var restrictionsShadowErr *ServiceError

		current, err := s.repo.LockAssignmentAt(txCtx, tenantID, in.AssignmentID, in.EffectiveDate)
		if err != nil {
			return nil, mapPgError(err)
		}
		freeze, err := s.freezeCheck(settings, txTime, in.EffectiveDate)
		if err != nil {
			maybeLogFrozenWindowRejected(txCtx, tenantID, requestID, initiatorID, "assignment.updated", "org_assignment", current.ID, in.EffectiveDate, freeze, err, logrus.Fields{
				"assignment_id":   current.ID.String(),
				"position_id":     current.PositionID.String(),
				"pernr":           current.Pernr,
				"subject_id":      current.SubjectID.String(),
				"assignment_type": current.AssignmentType,
				"operation":       "Update",
			})
			return nil, err
		}
		if current.EffectiveDate.Equal(in.EffectiveDate) {
			return nil, newServiceError(422, "ORG_USE_CORRECT", "use correct for in-place updates", nil)
		}

		next, hasNext, err := s.repo.NextAssignmentEffectiveDate(txCtx, tenantID, current.ID, in.EffectiveDate)
		if err != nil {
			return nil, err
		}
		newEnd := current.EndDate
		if hasNext && next.Before(newEnd) {
			newEnd = next
		}

		var positionID uuid.UUID
		if in.PositionID != nil {
			positionID = *in.PositionID
			exists, err := s.repo.PositionExistsAt(txCtx, tenantID, positionID, in.EffectiveDate)
			if err != nil {
				return nil, err
			}
			if !exists {
				return nil, newServiceError(422, "ORG_POSITION_NOT_FOUND_AT_DATE", "position_id not found at effective_date", nil)
			}
		} else {
			if in.OrgNodeID == nil || *in.OrgNodeID == uuid.Nil {
				return nil, newServiceError(400, "ORG_INVALID_BODY", "position_id or org_node_id is required", nil)
			}
			if !configuration.Use().EnableOrgAutoPositions {
				return nil, newServiceError(422, "ORG_AUTO_POSITION_DISABLED", "auto position creation is disabled", nil)
			}
			hierarchyType := "OrgUnit"
			exists, err := s.repo.NodeExistsAt(txCtx, tenantID, *in.OrgNodeID, hierarchyType, in.EffectiveDate)
			if err != nil {
				return nil, err
			}
			if !exists {
				return nil, newServiceError(422, "ORG_NODE_NOT_FOUND_AT_DATE", "org_node_id not found at effective_date", nil)
			}
			positionID, err = autoPositionID(tenantID, *in.OrgNodeID, current.SubjectID)
			if err != nil {
				return nil, err
			}
			code := autoPositionCode(positionID)
			if err := s.repo.InsertAutoPosition(txCtx, tenantID, positionID, *in.OrgNodeID, code, in.EffectiveDate); err != nil {
				return nil, mapPgError(err)
			}
		}

		newAllocatedFTE := current.AllocatedFTE
		if in.AllocatedFTE != nil {
			if *in.AllocatedFTE <= 0 {
				return nil, newServiceError(400, "ORG_INVALID_BODY", "allocated_fte must be > 0", nil)
			}
			newAllocatedFTE = *in.AllocatedFTE
		}

		var newPosSlice PositionSliceRow
		if current.IsPrimary {
			idsToLock := []uuid.UUID{current.PositionID, positionID}
			if current.PositionID == positionID {
				idsToLock = []uuid.UUID{positionID}
			} else if strings.Compare(current.PositionID.String(), positionID.String()) > 0 {
				idsToLock[0], idsToLock[1] = idsToLock[1], idsToLock[0]
			}
			for _, id := range idsToLock {
				slice, err := s.repo.LockPositionSliceAt(txCtx, tenantID, id, in.EffectiveDate)
				if err != nil {
					return nil, mapPgError(err)
				}
				if id == positionID {
					newPosSlice = slice
				}
			}
			occupiedNew, err := s.repo.SumAllocatedFTEAt(txCtx, tenantID, positionID, in.EffectiveDate)
			if err != nil {
				return nil, err
			}
			if current.PositionID == positionID {
				occupiedNew -= current.AllocatedFTE
				if occupiedNew < 0 {
					occupiedNew = 0
				}
			}
			if occupiedNew+newAllocatedFTE > newPosSlice.CapacityFTE {
				return nil, newServiceError(422, "ORG_POSITION_OVER_CAPACITY", "position capacity exceeded", nil)
			}
		} else if restrictionsMode != "disabled" {
			newPosSlice, err = s.repo.GetPositionSliceAt(txCtx, tenantID, positionID, in.EffectiveDate)
			if err != nil {
				return nil, mapPgError(err)
			}
		}

		if restrictionsMode != "disabled" {
			isAutoCreated, err := s.repo.GetPositionIsAutoCreated(txCtx, tenantID, positionID)
			if err != nil {
				return nil, mapPgError(err)
			}
			restrictionsJSON, err := extractRestrictionsFromProfile(newPosSlice.Profile)
			if err != nil {
				if restrictionsMode == "enforce" {
					maybeLogModeRejected(txCtx, "org.position_restrictions.rejected", tenantID, requestID, initiatorID, "assignment.updated", "org_assignment", current.ID, in.EffectiveDate, restrictionsMode, err, logrus.Fields{
						"assignment_id": current.ID.String(),
						"position_id":   positionID.String(),
						"operation":     "Update",
					})
					return nil, err
				}
				var svcErr *ServiceError
				if !errors.As(err, &svcErr) {
					return nil, err
				}
				restrictionsShadowErr = svcErr
			} else {
				parsedRestrictions, err := parsePositionRestrictions(restrictionsJSON)
				if err != nil {
					if restrictionsMode == "enforce" {
						maybeLogModeRejected(txCtx, "org.position_restrictions.rejected", tenantID, requestID, initiatorID, "assignment.updated", "org_assignment", current.ID, in.EffectiveDate, restrictionsMode, err, logrus.Fields{
							"assignment_id": current.ID.String(),
							"position_id":   positionID.String(),
							"operation":     "Update",
						})
						return nil, err
					}
					var svcErr *ServiceError
					if !errors.As(err, &svcErr) {
						return nil, err
					}
					restrictionsShadowErr = svcErr
				} else if err := s.validatePositionRestrictionsAgainstSlice(txCtx, tenantID, isAutoCreated, newPosSlice, parsedRestrictions); err != nil {
					if restrictionsMode == "enforce" {
						maybeLogModeRejected(txCtx, "org.position_restrictions.rejected", tenantID, requestID, initiatorID, "assignment.updated", "org_assignment", current.ID, in.EffectiveDate, restrictionsMode, err, logrus.Fields{
							"assignment_id": current.ID.String(),
							"position_id":   positionID.String(),
							"operation":     "Update",
						})
						return nil, err
					}
					var svcErr *ServiceError
					if !errors.As(err, &svcErr) {
						return nil, err
					}
					restrictionsShadowErr = svcErr
				}
			}
		}

		if err := s.repo.TruncateAssignment(txCtx, tenantID, current.ID, in.EffectiveDate); err != nil {
			return nil, mapPgError(err)
		}

		newID, err := s.repo.InsertAssignment(txCtx, tenantID, AssignmentInsert{
			PositionID:     positionID,
			SubjectType:    current.SubjectType,
			SubjectID:      current.SubjectID,
			Pernr:          current.Pernr,
			AssignmentType: current.AssignmentType,
			IsPrimary:      current.IsPrimary,
			AllocatedFTE:   newAllocatedFTE,
			EffectiveDate:  in.EffectiveDate,
			EndDate:        newEnd,
		})
		if err != nil {
			return nil, mapPgError(err)
		}
		if current.PositionID != positionID {
			logWithFields(txCtx, logrus.InfoLevel, "org.assignment.transferred", logrus.Fields{
				"tenant_id":       tenantID.String(),
				"assignment_id":   newID.String(),
				"previous_id":     current.ID.String(),
				"previous_pos_id": current.PositionID.String(),
				"position_id":     positionID.String(),
				"pernr":           current.Pernr,
				"subject_id":      current.SubjectID.String(),
				"assignment_type": current.AssignmentType,
				"effective_date":  in.EffectiveDate.UTC().Format(time.RFC3339),
			})
		} else if !current.IsPrimary {
			logWithFields(txCtx, logrus.InfoLevel, "org.assignment.updated", logrus.Fields{
				"tenant_id":       tenantID.String(),
				"assignment_id":   newID.String(),
				"previous_id":     current.ID.String(),
				"position_id":     positionID.String(),
				"pernr":           current.Pernr,
				"subject_id":      current.SubjectID.String(),
				"assignment_type": current.AssignmentType,
				"effective_date":  in.EffectiveDate.UTC().Format(time.RFC3339),
			})
		}

		_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
			RequestID:       requestID,
			TransactionTime: txTime,
			InitiatorID:     initiatorID,
			ChangeType:      "assignment.updated",
			EntityType:      "org_assignment",
			EntityID:        newID,
			EffectiveDate:   in.EffectiveDate,
			EndDate:         newEnd,
			OldValues: map[string]any{
				"assignment_id":   current.ID.String(),
				"position_id":     current.PositionID.String(),
				"subject_type":    current.SubjectType,
				"subject_id":      current.SubjectID.String(),
				"pernr":           current.Pernr,
				"assignment_type": current.AssignmentType,
				"is_primary":      current.IsPrimary,
				"allocated_fte":   current.AllocatedFTE,
				"effective_date":  current.EffectiveDate.UTC().Format(time.RFC3339),
				"end_date":        current.EndDate.UTC().Format(time.RFC3339),
			},
			NewValues: map[string]any{
				"assignment_id":   newID.String(),
				"position_id":     positionID.String(),
				"subject_type":    current.SubjectType,
				"subject_id":      current.SubjectID.String(),
				"pernr":           current.Pernr,
				"assignment_type": current.AssignmentType,
				"is_primary":      current.IsPrimary,
				"allocated_fte":   newAllocatedFTE,
				"effective_date":  in.EffectiveDate.UTC().Format(time.RFC3339),
				"end_date":        newEnd.UTC().Format(time.RFC3339),
			},
			Meta: func() map[string]any {
				meta := map[string]any{
					"reason_code": reasonCode,
					"reason_note": in.ReasonNote,
				}
				addReasonCodeMeta(meta, reasonInfo)
				if restrictionsShadowErr != nil {
					meta["position_restrictions_validation_mode"] = restrictionsMode
					meta["position_restrictions_validation_error_code"] = restrictionsShadowErr.Code
				}
				return meta
			}(),
			Operation:       "Update",
			FreezeMode:      freeze.Mode,
			FreezeViolation: freeze.Violation,
			FreezeCutoffUTC: freeze.CutoffUTC,
			AffectedAtUTC:   in.EffectiveDate,
		})
		if err != nil {
			return nil, err
		}

		ev := buildEventV1(requestID, tenantID, initiatorID, txTime, "assignment.updated", "org_assignment", newID, in.EffectiveDate, newEnd)
		res := &UpdateAssignmentResult{
			AssignmentID:  newID,
			PositionID:    positionID,
			EffectiveDate: in.EffectiveDate,
			EndDate:       newEnd,
			GeneratedEvents: []events.OrgEventV1{
				ev,
			},
		}
		if err := s.enqueueOutboxEvents(txCtx, tenantID, res.GeneratedEvents); err != nil {
			return nil, err
		}
		return res, nil
	})
	if err != nil {
		return nil, err
	}
	if !shouldSkipCacheInvalidation(ctx) {
		s.InvalidateTenantCacheWithReason(tenantID, "write_commit")
	}
	return written, nil
}

func (s *OrgService) GetAssignments(ctx context.Context, tenantID uuid.UUID, subject string, asOf *time.Time) (uuid.UUID, []AssignmentViewRow, time.Time, error) {
	if tenantID == uuid.Nil {
		return uuid.Nil, nil, time.Time{}, newServiceError(400, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	subject = strings.TrimSpace(subject)
	if !strings.HasPrefix(subject, "person:") {
		return uuid.Nil, nil, time.Time{}, newServiceError(400, "ORG_INVALID_QUERY", "subject must be person:{pernr}", nil)
	}
	pernr := strings.TrimPrefix(subject, "person:")
	pernr = strings.TrimSpace(pernr)

	if asOf == nil || (*asOf).IsZero() {
		type result struct {
			subjectUUID uuid.UUID
			rows        []AssignmentViewRow
		}
		out, err := inTx(ctx, tenantID, func(txCtx context.Context) (result, error) {
			subjectUUID, err := s.repo.ResolvePersonUUIDByPernr(txCtx, tenantID, pernr)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return result{}, newServiceError(http.StatusNotFound, "ORG_PERSON_NOT_FOUND", "pernr not found", nil)
				}
				return result{}, err
			}
			rows, err := s.repo.ListAssignmentsTimeline(txCtx, tenantID, subjectUUID)
			if err != nil {
				return result{}, err
			}
			return result{subjectUUID: subjectUUID, rows: rows}, nil
		})
		if err != nil {
			return uuid.Nil, nil, time.Time{}, err
		}
		return out.subjectUUID, out.rows, time.Time{}, nil
	}

	out, err := inTx(ctx, tenantID, func(txCtx context.Context) (cachedAssignments, error) {
		subjectUUID, err := s.repo.ResolvePersonUUIDByPernr(txCtx, tenantID, pernr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return cachedAssignments{}, newServiceError(http.StatusNotFound, "ORG_PERSON_NOT_FOUND", "pernr not found", nil)
			}
			return cachedAssignments{}, err
		}

		cacheKey := repo.CacheKey("org", "assignments_asof", tenantID, subjectUUID, (*asOf).UTC().Format(time.RFC3339Nano))
		if s != nil && s.cache != nil {
			if cachedAny, ok := s.cache.Get(cacheKey); ok {
				if cached, ok := cachedAny.(cachedAssignments); ok {
					recordCacheRequest("assignments", true)
					return cached, nil
				}
			}
			recordCacheRequest("assignments", false)
		}

		rows, err := s.repo.ListAssignmentsAsOf(txCtx, tenantID, subjectUUID, *asOf)
		if err != nil {
			return cachedAssignments{}, err
		}
		if s != nil && s.cache != nil {
			s.cache.Set(tenantID, cacheKey, cachedAssignments{
				SubjectID: subjectUUID,
				Rows:      rows,
				AsOf:      *asOf,
			})
		}
		return cachedAssignments{
			SubjectID: subjectUUID,
			Rows:      rows,
			AsOf:      *asOf,
		}, nil
	})
	if err != nil {
		return uuid.Nil, nil, time.Time{}, err
	}
	return out.SubjectID, out.Rows, out.AsOf, nil
}

func buildEventV1(requestID string, tenantID, initiatorID uuid.UUID, txTime time.Time, changeType, entityType string, entityID uuid.UUID, effectiveDate, endDate time.Time) events.OrgEventV1 {
	if requestID == "" {
		requestID = uuid.NewString()
	}
	return events.OrgEventV1{
		EventID:         uuid.New(),
		EventVersion:    events.EventVersionV1,
		RequestID:       requestID,
		TenantID:        tenantID,
		TransactionTime: txTime,
		InitiatorID:     initiatorID,
		ChangeType:      changeType,
		EntityType:      entityType,
		EntityID:        entityID,
		EntityVersion:   0,
		EffectiveWindow: events.EffectiveWindowV1{
			EffectiveDate: effectiveDate.UTC(),
			EndDate:       endDate.UTC(),
		},
		NewValues: []byte(`{}`),
	}
}

func autoPositionID(tenantID, orgNodeID, subjectID uuid.UUID) (uuid.UUID, error) {
	if tenantID == uuid.Nil || orgNodeID == uuid.Nil || subjectID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("tenant_id/org_node_id/subject_id are required")
	}
	autoNamespace := uuid.MustParse("2ee72897-775c-49eb-94a2-1d6b9e157701")
	payload := fmt.Sprintf("%s:%s:%s:%s", tenantID, orgNodeID, "person", subjectID)
	return uuid.NewSHA1(autoNamespace, []byte(payload)), nil
}

func autoPositionCode(positionID uuid.UUID) string {
	raw := strings.ToUpper(strings.ReplaceAll(positionID.String(), "-", ""))
	if len(raw) > 16 {
		raw = raw[:16]
	}
	return "AUTO-" + raw
}

func mapPgError(err error) error {
	var svcErr *ServiceError
	if errors.As(err, &svcErr) {
		return svcErr
	}
	return mapPgErrorToServiceError(err)
}

func inTx[T any](ctx context.Context, tenantID uuid.UUID, fn func(txCtx context.Context) (T, error)) (T, error) {
	var zero T

	hasExistingTx := ctx.Value(constants.TxKey) != nil
	tx, err := composables.BeginTx(ctx)
	if err != nil {
		return zero, err
	}
	if !hasExistingTx {
		defer func() { _ = tx.Rollback(ctx) }()
	}

	txCtx := ctx
	if !hasExistingTx {
		txCtx = composables.WithTx(ctx, tx)
	}
	txCtx = composables.WithTenantID(txCtx, tenantID)
	if err := composables.ApplyTenantRLS(txCtx, tx); err != nil {
		return zero, err
	}

	out, err := fn(txCtx)
	if err != nil {
		return zero, err
	}
	if !hasExistingTx {
		if err := tx.Commit(ctx); err != nil {
			return zero, err
		}
	}
	return out, nil
}
