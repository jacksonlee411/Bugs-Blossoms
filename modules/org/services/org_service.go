package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/org/domain/events"
	"github.com/iota-uz/iota-sdk/modules/org/domain/subjectid"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
)

var endOfTime = time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)

type OrgRepository interface {
	ListHierarchyAsOf(ctx context.Context, tenantID uuid.UUID, hierarchyType string, asOf time.Time) ([]HierarchyNode, error)

	HasRoot(ctx context.Context, tenantID uuid.UUID) (bool, error)
	InsertNode(ctx context.Context, tenantID uuid.UUID, nodeType, code string, isRoot bool) (uuid.UUID, error)
	InsertNodeSlice(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID, slice NodeSliceInsert) (uuid.UUID, error)
	InsertEdge(ctx context.Context, tenantID uuid.UUID, hierarchyType string, parentID *uuid.UUID, childID uuid.UUID, effectiveDate, endDate time.Time) (uuid.UUID, error)
	NodeExistsAt(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID, hierarchyType string, asOf time.Time) (bool, error)
	GetNodeIsRoot(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID) (bool, error)

	LockNodeSliceAt(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID, asOf time.Time) (NodeSliceRow, error)
	TruncateNodeSlice(ctx context.Context, tenantID uuid.UUID, sliceID uuid.UUID, endDate time.Time) error
	NextNodeSliceEffectiveDate(ctx context.Context, tenantID uuid.UUID, nodeID uuid.UUID, after time.Time) (time.Time, bool, error)

	LockEdgeAt(ctx context.Context, tenantID uuid.UUID, hierarchyType string, childID uuid.UUID, asOf time.Time) (EdgeRow, error)
	LockEdgesInSubtree(ctx context.Context, tenantID uuid.UUID, hierarchyType string, asOf time.Time, movedPath string) ([]EdgeRow, error)
	TruncateEdge(ctx context.Context, tenantID uuid.UUID, edgeID uuid.UUID, endDate time.Time) error

	PositionExistsAt(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, asOf time.Time) (bool, error)
	InsertAutoPosition(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, orgNodeID uuid.UUID, code string, effectiveDate time.Time) error
	GetPositionOrgNodeAt(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, asOf time.Time) (uuid.UUID, error)

	LockAssignmentAt(ctx context.Context, tenantID uuid.UUID, assignmentID uuid.UUID, asOf time.Time) (AssignmentRow, error)
	TruncateAssignment(ctx context.Context, tenantID uuid.UUID, assignmentID uuid.UUID, endDate time.Time) error
	NextAssignmentEffectiveDate(ctx context.Context, tenantID uuid.UUID, assignmentID uuid.UUID, after time.Time) (time.Time, bool, error)

	InsertAssignment(ctx context.Context, tenantID uuid.UUID, assignment AssignmentInsert) (uuid.UUID, error)
	ListAssignmentsTimeline(ctx context.Context, tenantID uuid.UUID, subjectID uuid.UUID) ([]AssignmentViewRow, error)
	ListAssignmentsAsOf(ctx context.Context, tenantID uuid.UUID, subjectID uuid.UUID, asOf time.Time) ([]AssignmentViewRow, error)
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
	EffectiveDate  time.Time
	EndDate        time.Time
}

type AssignmentViewRow struct {
	ID             uuid.UUID  `json:"id"`
	PositionID     uuid.UUID  `json:"position_id"`
	OrgNodeID      uuid.UUID  `json:"org_node_id"`
	AssignmentType string     `json:"assignment_type"`
	IsPrimary      bool       `json:"is_primary"`
	EffectiveDate  time.Time  `json:"effective_date"`
	EndDate        time.Time  `json:"end_date"`
	PositionCode   *string    `json:"position_code,omitempty"`
	Pernr          *string    `json:"pernr,omitempty"`
	SubjectID      *uuid.UUID `json:"subject_id,omitempty"`
}

type OrgService struct {
	repo OrgRepository
}

func NewOrgService(repo OrgRepository) *OrgService {
	return &OrgService{repo: repo}
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

	nodes, err := inTx(ctx, tenantID, func(txCtx context.Context) ([]HierarchyNode, error) {
		return s.repo.ListHierarchyAsOf(txCtx, tenantID, hierarchyType, asOf)
	})
	if err != nil {
		return nil, time.Time{}, err
	}
	return nodes, asOf, nil
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
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	if in.Code == "" || in.Name == "" || in.EffectiveDate.IsZero() {
		return nil, newServiceError(400, "ORG_INVALID_BODY", "code/name/effective_date are required", nil)
	}
	if in.Status == "" {
		in.Status = "active"
	}

	type out struct {
		nodeID uuid.UUID
		edgeID uuid.UUID
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (out, error) {
		hierarchyType := "OrgUnit"
		if in.ParentID == nil {
			hasRoot, err := s.repo.HasRoot(txCtx, tenantID)
			if err != nil {
				return out{}, err
			}
			if hasRoot {
				return out{}, newServiceError(409, "ORG_OVERLAP", "root already exists", nil)
			}
		} else {
			exists, err := s.repo.NodeExistsAt(txCtx, tenantID, *in.ParentID, hierarchyType, in.EffectiveDate)
			if err != nil {
				return out{}, err
			}
			if !exists {
				return out{}, newServiceError(422, "ORG_PARENT_NOT_FOUND", "parent not found at effective_date", nil)
			}
		}

		nodeID, err := s.repo.InsertNode(txCtx, tenantID, hierarchyType, in.Code, in.ParentID == nil)
		if err != nil {
			return out{}, mapPgError(err)
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
			return out{}, mapPgError(err)
		}

		edgeID, err := s.repo.InsertEdge(txCtx, tenantID, hierarchyType, in.ParentID, nodeID, in.EffectiveDate, endOfTime)
		if err != nil {
			return out{}, mapPgError(err)
		}

		return out{nodeID: nodeID, edgeID: edgeID}, nil
	})
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	evNode := buildEventV1(requestID, tenantID, initiatorID, now, "node.created", "org_node", written.nodeID, in.EffectiveDate, endOfTime)
	evEdge := buildEventV1(requestID, tenantID, initiatorID, now, "edge.created", "org_edge", written.edgeID, in.EffectiveDate, endOfTime)

	return &CreateNodeResult{
		NodeID:        written.nodeID,
		EdgeID:        written.edgeID,
		EffectiveDate: in.EffectiveDate,
		EndDate:       endOfTime,
		GeneratedEvents: []events.OrgEventV1{
			evNode,
			evEdge,
		},
	}, nil
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
	if in.NodeID == uuid.Nil || in.EffectiveDate.IsZero() {
		return nil, newServiceError(400, "ORG_INVALID_BODY", "id/effective_date are required", nil)
	}

	windowEnd, err := inTx(ctx, tenantID, func(txCtx context.Context) (time.Time, error) {
		current, err := s.repo.LockNodeSliceAt(txCtx, tenantID, in.NodeID, in.EffectiveDate)
		if err != nil {
			return time.Time{}, mapPgError(err)
		}
		if current.EffectiveDate.Equal(in.EffectiveDate) {
			return time.Time{}, newServiceError(422, "ORG_USE_CORRECT", "use correct for in-place updates", nil)
		}

		next, hasNext, err := s.repo.NextNodeSliceEffectiveDate(txCtx, tenantID, in.NodeID, in.EffectiveDate)
		if err != nil {
			return time.Time{}, err
		}

		newEnd := current.EndDate
		if hasNext && next.Before(newEnd) {
			newEnd = next
		}

		if err := s.repo.TruncateNodeSlice(txCtx, tenantID, current.ID, in.EffectiveDate); err != nil {
			return time.Time{}, mapPgError(err)
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
			return time.Time{}, mapPgError(err)
		}

		return newEnd, nil
	})
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	ev := buildEventV1(requestID, tenantID, initiatorID, now, "node.updated", "org_node", in.NodeID, in.EffectiveDate, windowEnd)
	return &UpdateNodeResult{
		NodeID:        in.NodeID,
		EffectiveDate: in.EffectiveDate,
		EndDate:       windowEnd,
		GeneratedEvents: []events.OrgEventV1{
			ev,
		},
	}, nil
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
	if in.NodeID == uuid.Nil || in.NewParentID == uuid.Nil || in.EffectiveDate.IsZero() {
		return nil, newServiceError(400, "ORG_INVALID_BODY", "id/new_parent_id/effective_date are required", nil)
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (EdgeRow, error) {
		isRoot, err := s.repo.GetNodeIsRoot(txCtx, tenantID, in.NodeID)
		if err != nil {
			return EdgeRow{}, err
		}
		if isRoot {
			return EdgeRow{}, newServiceError(422, "ORG_CANNOT_MOVE_ROOT", "cannot move root", nil)
		}

		hierarchyType := "OrgUnit"
		parentExists, err := s.repo.NodeExistsAt(txCtx, tenantID, in.NewParentID, hierarchyType, in.EffectiveDate)
		if err != nil {
			return EdgeRow{}, err
		}
		if !parentExists {
			return EdgeRow{}, newServiceError(422, "ORG_PARENT_NOT_FOUND", "new_parent_id not found at effective_date", nil)
		}

		movedEdge, err := s.repo.LockEdgeAt(txCtx, tenantID, hierarchyType, in.NodeID, in.EffectiveDate)
		if err != nil {
			return EdgeRow{}, mapPgError(err)
		}
		if movedEdge.EffectiveDate.Equal(in.EffectiveDate) {
			return EdgeRow{}, newServiceError(422, "ORG_USE_CORRECT_MOVE", "use correct-move for in-place updates", nil)
		}

		subtree, err := s.repo.LockEdgesInSubtree(txCtx, tenantID, hierarchyType, in.EffectiveDate, movedEdge.Path)
		if err != nil {
			return EdgeRow{}, err
		}
		for _, e := range subtree {
			if e.EffectiveDate.Equal(in.EffectiveDate) {
				return EdgeRow{}, newServiceError(422, "ORG_USE_CORRECT_MOVE", "subtree contains edges requiring correct-move at effective_date", nil)
			}
		}

		if err := s.repo.TruncateEdge(txCtx, tenantID, movedEdge.ID, in.EffectiveDate); err != nil {
			return EdgeRow{}, mapPgError(err)
		}
		newEdgeID, err := s.repo.InsertEdge(txCtx, tenantID, hierarchyType, &in.NewParentID, in.NodeID, in.EffectiveDate, movedEdge.EndDate)
		if err != nil {
			return EdgeRow{}, mapPgError(err)
		}

		currentSlice, err := s.repo.LockNodeSliceAt(txCtx, tenantID, in.NodeID, in.EffectiveDate)
		if err != nil {
			return EdgeRow{}, mapPgError(err)
		}
		if currentSlice.EffectiveDate.Equal(in.EffectiveDate) {
			return EdgeRow{}, newServiceError(422, "ORG_USE_CORRECT_MOVE", "use correct for in-place parent_hint updates", nil)
		}
		next, hasNext, err := s.repo.NextNodeSliceEffectiveDate(txCtx, tenantID, in.NodeID, in.EffectiveDate)
		if err != nil {
			return EdgeRow{}, err
		}
		newEnd := currentSlice.EndDate
		if hasNext && next.Before(newEnd) {
			newEnd = next
		}
		if err := s.repo.TruncateNodeSlice(txCtx, tenantID, currentSlice.ID, in.EffectiveDate); err != nil {
			return EdgeRow{}, mapPgError(err)
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
			return EdgeRow{}, mapPgError(err)
		}

		for _, e := range subtree {
			if e.ChildNodeID == in.NodeID {
				continue
			}
			if err := s.repo.TruncateEdge(txCtx, tenantID, e.ID, in.EffectiveDate); err != nil {
				return EdgeRow{}, mapPgError(err)
			}
			if _, err := s.repo.InsertEdge(txCtx, tenantID, hierarchyType, e.ParentNodeID, e.ChildNodeID, in.EffectiveDate, e.EndDate); err != nil {
				return EdgeRow{}, mapPgError(err)
			}
		}

		return EdgeRow{
			ID:            newEdgeID,
			ParentNodeID:  &in.NewParentID,
			ChildNodeID:   in.NodeID,
			EffectiveDate: in.EffectiveDate,
			EndDate:       movedEdge.EndDate,
		}, nil
	})
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	ev := buildEventV1(requestID, tenantID, initiatorID, now, "edge.updated", "org_edge", written.ID, written.EffectiveDate, written.EndDate)
	return &MoveNodeResult{
		EdgeID:        written.ID,
		EffectiveDate: written.EffectiveDate,
		EndDate:       written.EndDate,
		GeneratedEvents: []events.OrgEventV1{
			ev,
		},
	}, nil
}

type CreateAssignmentInput struct {
	Pernr          string
	EffectiveDate  time.Time
	AssignmentType string
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
	in.Pernr = strings.TrimSpace(in.Pernr)
	if in.Pernr == "" || in.EffectiveDate.IsZero() {
		return nil, newServiceError(400, "ORG_INVALID_BODY", "pernr/effective_date are required", nil)
	}

	assignmentType := strings.TrimSpace(in.AssignmentType)
	if assignmentType == "" {
		assignmentType = "primary"
	}
	if assignmentType != "primary" && !configuration.Use().EnableOrgExtendedAssignmentTypes {
		return nil, newServiceError(422, "ORG_ASSIGNMENT_TYPE_DISABLED", "assignment_type is disabled", nil)
	}
	if assignmentType != "primary" {
		return nil, newServiceError(422, "ORG_ASSIGNMENT_TYPE_DISABLED", "assignment_type is disabled", nil)
	}

	derivedSubjectID, err := subjectid.NormalizedSubjectID(tenantID, "person", in.Pernr)
	if err != nil {
		return nil, newServiceError(400, "ORG_INVALID_BODY", err.Error(), err)
	}
	if in.SubjectID != nil && *in.SubjectID != derivedSubjectID {
		return nil, newServiceError(422, "ORG_SUBJECT_MISMATCH", "subject_id does not match SSOT mapping", nil)
	}

	type out struct {
		assignmentID uuid.UUID
		positionID   uuid.UUID
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (out, error) {
		var positionID uuid.UUID
		if in.PositionID != nil {
			positionID = *in.PositionID
			exists, err := s.repo.PositionExistsAt(txCtx, tenantID, positionID, in.EffectiveDate)
			if err != nil {
				return out{}, err
			}
			if !exists {
				return out{}, newServiceError(422, "ORG_POSITION_NOT_FOUND_AT_DATE", "position_id not found at effective_date", nil)
			}
		} else {
			if in.OrgNodeID == nil || *in.OrgNodeID == uuid.Nil {
				return out{}, newServiceError(400, "ORG_INVALID_BODY", "position_id or org_node_id is required", nil)
			}
			if !configuration.Use().EnableOrgAutoPositions {
				return out{}, newServiceError(422, "ORG_AUTO_POSITION_DISABLED", "auto position creation is disabled", nil)
			}
			hierarchyType := "OrgUnit"
			exists, err := s.repo.NodeExistsAt(txCtx, tenantID, *in.OrgNodeID, hierarchyType, in.EffectiveDate)
			if err != nil {
				return out{}, err
			}
			if !exists {
				return out{}, newServiceError(422, "ORG_NODE_NOT_FOUND_AT_DATE", "org_node_id not found at effective_date", nil)
			}
			positionID, err = autoPositionID(tenantID, *in.OrgNodeID, derivedSubjectID)
			if err != nil {
				return out{}, err
			}
			code := autoPositionCode(positionID)
			if err := s.repo.InsertAutoPosition(txCtx, tenantID, positionID, *in.OrgNodeID, code, in.EffectiveDate); err != nil {
				return out{}, mapPgError(err)
			}
		}

		assignmentID, err := s.repo.InsertAssignment(txCtx, tenantID, AssignmentInsert{
			PositionID:     positionID,
			SubjectType:    "person",
			SubjectID:      derivedSubjectID,
			Pernr:          in.Pernr,
			AssignmentType: assignmentType,
			IsPrimary:      true,
			EffectiveDate:  in.EffectiveDate,
			EndDate:        endOfTime,
		})
		if err != nil {
			return out{}, mapPgError(err)
		}
		return out{assignmentID: assignmentID, positionID: positionID}, nil
	})
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	ev := buildEventV1(requestID, tenantID, initiatorID, now, "assignment.created", "org_assignment", written.assignmentID, in.EffectiveDate, endOfTime)
	return &CreateAssignmentResult{
		AssignmentID:  written.assignmentID,
		PositionID:    written.positionID,
		SubjectID:     derivedSubjectID,
		EffectiveDate: in.EffectiveDate,
		EndDate:       endOfTime,
		GeneratedEvents: []events.OrgEventV1{
			ev,
		},
	}, nil
}

type UpdateAssignmentInput struct {
	AssignmentID  uuid.UUID
	EffectiveDate time.Time
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
	if in.AssignmentID == uuid.Nil || in.EffectiveDate.IsZero() {
		return nil, newServiceError(400, "ORG_INVALID_BODY", "id/effective_date are required", nil)
	}

	type out struct {
		assignmentID uuid.UUID
		positionID   uuid.UUID
		endDate      time.Time
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (out, error) {
		current, err := s.repo.LockAssignmentAt(txCtx, tenantID, in.AssignmentID, in.EffectiveDate)
		if err != nil {
			return out{}, mapPgError(err)
		}
		if current.EffectiveDate.Equal(in.EffectiveDate) {
			return out{}, newServiceError(422, "ORG_USE_CORRECT", "use correct for in-place updates", nil)
		}

		next, hasNext, err := s.repo.NextAssignmentEffectiveDate(txCtx, tenantID, current.ID, in.EffectiveDate)
		if err != nil {
			return out{}, err
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
				return out{}, err
			}
			if !exists {
				return out{}, newServiceError(422, "ORG_POSITION_NOT_FOUND_AT_DATE", "position_id not found at effective_date", nil)
			}
		} else {
			if in.OrgNodeID == nil || *in.OrgNodeID == uuid.Nil {
				return out{}, newServiceError(400, "ORG_INVALID_BODY", "position_id or org_node_id is required", nil)
			}
			if !configuration.Use().EnableOrgAutoPositions {
				return out{}, newServiceError(422, "ORG_AUTO_POSITION_DISABLED", "auto position creation is disabled", nil)
			}
			hierarchyType := "OrgUnit"
			exists, err := s.repo.NodeExistsAt(txCtx, tenantID, *in.OrgNodeID, hierarchyType, in.EffectiveDate)
			if err != nil {
				return out{}, err
			}
			if !exists {
				return out{}, newServiceError(422, "ORG_NODE_NOT_FOUND_AT_DATE", "org_node_id not found at effective_date", nil)
			}
			positionID, err = autoPositionID(tenantID, *in.OrgNodeID, current.SubjectID)
			if err != nil {
				return out{}, err
			}
			code := autoPositionCode(positionID)
			if err := s.repo.InsertAutoPosition(txCtx, tenantID, positionID, *in.OrgNodeID, code, in.EffectiveDate); err != nil {
				return out{}, mapPgError(err)
			}
		}

		if err := s.repo.TruncateAssignment(txCtx, tenantID, current.ID, in.EffectiveDate); err != nil {
			return out{}, mapPgError(err)
		}

		newID, err := s.repo.InsertAssignment(txCtx, tenantID, AssignmentInsert{
			PositionID:     positionID,
			SubjectType:    current.SubjectType,
			SubjectID:      current.SubjectID,
			Pernr:          current.Pernr,
			AssignmentType: current.AssignmentType,
			IsPrimary:      current.IsPrimary,
			EffectiveDate:  in.EffectiveDate,
			EndDate:        newEnd,
		})
		if err != nil {
			return out{}, mapPgError(err)
		}

		return out{assignmentID: newID, positionID: positionID, endDate: newEnd}, nil
	})
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	ev := buildEventV1(requestID, tenantID, initiatorID, now, "assignment.updated", "org_assignment", written.assignmentID, in.EffectiveDate, written.endDate)
	return &UpdateAssignmentResult{
		AssignmentID:  written.assignmentID,
		PositionID:    written.positionID,
		EffectiveDate: in.EffectiveDate,
		EndDate:       written.endDate,
		GeneratedEvents: []events.OrgEventV1{
			ev,
		},
	}, nil
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
	subjectUUID, err := subjectid.NormalizedSubjectID(tenantID, "person", pernr)
	if err != nil {
		return uuid.Nil, nil, time.Time{}, newServiceError(400, "ORG_INVALID_QUERY", err.Error(), err)
	}

	if asOf == nil || (*asOf).IsZero() {
		rows, err := inTx(ctx, tenantID, func(txCtx context.Context) ([]AssignmentViewRow, error) {
			return s.repo.ListAssignmentsTimeline(txCtx, tenantID, subjectUUID)
		})
		if err != nil {
			return uuid.Nil, nil, time.Time{}, err
		}
		return subjectUUID, rows, time.Time{}, nil
	}

	rows, err := inTx(ctx, tenantID, func(txCtx context.Context) ([]AssignmentViewRow, error) {
		return s.repo.ListAssignmentsAsOf(txCtx, tenantID, subjectUUID, *asOf)
	})
	if err != nil {
		return uuid.Nil, nil, time.Time{}, err
	}
	return subjectUUID, rows, *asOf, nil
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

	pool, err := composables.UsePool(ctx)
	if err != nil {
		return zero, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return zero, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txCtx := composables.WithTx(ctx, tx)
	txCtx = composables.WithTenantID(txCtx, tenantID)
	if err := composables.ApplyTenantRLS(txCtx, tx); err != nil {
		return zero, err
	}

	out, err := fn(txCtx)
	if err != nil {
		return zero, err
	}
	if err := tx.Commit(ctx); err != nil {
		return zero, err
	}
	return out, nil
}
