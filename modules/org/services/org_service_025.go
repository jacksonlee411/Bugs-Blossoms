package services

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/org/domain/events"
	"github.com/iota-uz/iota-sdk/modules/org/domain/subjectid"
)

type CorrectNodeInput struct {
	NodeID        uuid.UUID
	AsOf          time.Time
	Name          *string
	I18nNames     map[string]string
	Status        *string
	DisplayOrder  *int
	LegalEntityID **uuid.UUID
	CompanyCode   **string
	LocationID    **uuid.UUID
	ManagerUserID **int64
}

type CorrectNodeResult struct {
	NodeID          uuid.UUID
	EffectiveDate   time.Time
	EndDate         time.Time
	GeneratedEvents []events.OrgEventV1
}

func (s *OrgService) CorrectNode(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in CorrectNodeInput) (*CorrectNodeResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	txTime := time.Now().UTC()
	if in.NodeID == uuid.Nil || in.AsOf.IsZero() {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "id/effective_date are required", nil)
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*CorrectNodeResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}

		target, err := s.repo.LockNodeSliceAt(txCtx, tenantID, in.NodeID, in.AsOf)
		if err != nil {
			return nil, mapPgError(err)
		}

		freeze, err := s.freezeCheck(settings, txTime, target.EffectiveDate)
		if err != nil {
			return nil, err
		}

		oldValues := map[string]any{
			"slice_id":        target.ID.String(),
			"org_node_id":     in.NodeID.String(),
			"name":            target.Name,
			"i18n_names":      target.I18nNames,
			"status":          target.Status,
			"display_order":   target.DisplayOrder,
			"parent_hint":     target.ParentHint,
			"manager_user_id": target.ManagerUserID,
			"effective_date":  target.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":        target.EndDate.UTC().Format(time.RFC3339),
		}

		patch := NodeSliceInPlacePatch{
			Name:          in.Name,
			I18nNames:     in.I18nNames,
			Status:        in.Status,
			DisplayOrder:  in.DisplayOrder,
			LegalEntityID: in.LegalEntityID,
			CompanyCode:   in.CompanyCode,
			LocationID:    in.LocationID,
			ManagerUserID: in.ManagerUserID,
		}
		if err := s.repo.UpdateNodeSliceInPlace(txCtx, tenantID, target.ID, patch); err != nil {
			return nil, mapPgError(err)
		}

		updated, err := s.repo.LockNodeSliceAt(txCtx, tenantID, in.NodeID, in.AsOf)
		if err != nil {
			return nil, mapPgError(err)
		}

		newValues := map[string]any{
			"slice_id":        updated.ID.String(),
			"org_node_id":     in.NodeID.String(),
			"name":            updated.Name,
			"i18n_names":      updated.I18nNames,
			"status":          updated.Status,
			"display_order":   updated.DisplayOrder,
			"parent_hint":     updated.ParentHint,
			"manager_user_id": updated.ManagerUserID,
			"effective_date":  updated.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":        updated.EndDate.UTC().Format(time.RFC3339),
		}

		_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
			RequestID:       requestID,
			TransactionTime: txTime,
			InitiatorID:     initiatorID,
			ChangeType:      "node.corrected",
			EntityType:      "org_node",
			EntityID:        in.NodeID,
			EffectiveDate:   updated.EffectiveDate,
			EndDate:         updated.EndDate,
			OldValues:       oldValues,
			NewValues:       newValues,
			Operation:       "Correct",
			FreezeMode:      freeze.Mode,
			FreezeViolation: freeze.Violation,
			FreezeCutoffUTC: freeze.CutoffUTC,
			AffectedAtUTC:   target.EffectiveDate,
		})
		if err != nil {
			return nil, err
		}

		ev := buildEventV1(requestID, tenantID, initiatorID, txTime, "node.corrected", "org_node", in.NodeID, updated.EffectiveDate, updated.EndDate)
		res := &CorrectNodeResult{
			NodeID:        in.NodeID,
			EffectiveDate: updated.EffectiveDate,
			EndDate:       updated.EndDate,
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
		s.InvalidateTenantCache(tenantID)
	}
	return written, nil
}

type RescindNodeInput struct {
	NodeID        uuid.UUID
	EffectiveDate time.Time
	Reason        string
}

type RescindNodeResult struct {
	NodeID          uuid.UUID
	EffectiveDate   time.Time
	EndDate         time.Time
	Status          string
	GeneratedEvents []events.OrgEventV1
}

func (s *OrgService) RescindNode(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in RescindNodeInput) (*RescindNodeResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	txTime := time.Now().UTC()
	if in.NodeID == uuid.Nil || in.EffectiveDate.IsZero() {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "id/effective_date are required", nil)
	}
	in.Reason = strings.TrimSpace(in.Reason)

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*RescindNodeResult, error) {
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
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_CANNOT_RESCIND_ROOT", "cannot rescind root", nil)
		}

		hasChildren, err := s.repo.HasChildrenAt(txCtx, tenantID, in.NodeID, in.EffectiveDate)
		if err != nil {
			return nil, err
		}
		if hasChildren {
			return nil, newServiceError(http.StatusConflict, "ORG_NODE_NOT_EMPTY", "node has children at effective_date", nil)
		}

		hasPositions, err := s.repo.HasPositionsAt(txCtx, tenantID, in.NodeID, in.EffectiveDate)
		if err != nil {
			return nil, err
		}
		if hasPositions {
			return nil, newServiceError(http.StatusConflict, "ORG_NODE_NOT_EMPTY", "node has positions at effective_date", nil)
		}

		target, err := s.repo.LockNodeSliceAt(txCtx, tenantID, in.NodeID, in.EffectiveDate)
		if err != nil {
			return nil, mapPgError(err)
		}

		oldValues := map[string]any{
			"slice_id":       target.ID.String(),
			"org_node_id":    in.NodeID.String(),
			"status":         target.Status,
			"effective_date": target.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":       target.EndDate.UTC().Format(time.RFC3339),
		}

		if err := s.repo.DeleteNodeSlicesFrom(txCtx, tenantID, in.NodeID, in.EffectiveDate); err != nil {
			return nil, err
		}
		hierarchyType := "OrgUnit"
		if err := s.repo.DeleteEdgesFrom(txCtx, tenantID, hierarchyType, in.NodeID, in.EffectiveDate); err != nil {
			return nil, err
		}

		edgeAt, err := s.repo.LockEdgeAt(txCtx, tenantID, hierarchyType, in.NodeID, in.EffectiveDate)
		if err == nil {
			if edgeAt.EffectiveDate.Equal(in.EffectiveDate) {
				if err := s.repo.DeleteEdgeByID(txCtx, tenantID, edgeAt.ID); err != nil {
					return nil, err
				}
			} else {
				if err := s.repo.TruncateEdge(txCtx, tenantID, edgeAt.ID, in.EffectiveDate); err != nil {
					return nil, mapPgError(err)
				}
			}
		}

		if target.EffectiveDate.Before(in.EffectiveDate) {
			if err := s.repo.TruncateNodeSlice(txCtx, tenantID, target.ID, in.EffectiveDate); err != nil {
				return nil, mapPgError(err)
			}
		}

		_, err = s.repo.InsertNodeSlice(txCtx, tenantID, in.NodeID, NodeSliceInsert{
			Name:          target.Name,
			I18nNames:     target.I18nNames,
			Status:        "rescinded",
			LegalEntityID: target.LegalEntityID,
			CompanyCode:   target.CompanyCode,
			LocationID:    target.LocationID,
			DisplayOrder:  target.DisplayOrder,
			ParentHint:    target.ParentHint,
			ManagerUserID: target.ManagerUserID,
			EffectiveDate: in.EffectiveDate,
			EndDate:       endOfTime,
		})
		if err != nil {
			return nil, mapPgError(err)
		}

		newValues := map[string]any{
			"org_node_id":    in.NodeID.String(),
			"status":         "rescinded",
			"effective_date": in.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":       endOfTime.UTC().Format(time.RFC3339),
			"reason":         in.Reason,
		}

		_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
			RequestID:       requestID,
			TransactionTime: txTime,
			InitiatorID:     initiatorID,
			ChangeType:      "node.rescinded",
			EntityType:      "org_node",
			EntityID:        in.NodeID,
			EffectiveDate:   in.EffectiveDate,
			EndDate:         endOfTime,
			OldValues:       oldValues,
			NewValues:       newValues,
			Operation:       "Rescind",
			Meta: map[string]any{
				"reason": in.Reason,
			},
			FreezeMode:      freeze.Mode,
			FreezeViolation: freeze.Violation,
			FreezeCutoffUTC: freeze.CutoffUTC,
			AffectedAtUTC:   in.EffectiveDate,
		})
		if err != nil {
			return nil, err
		}

		ev := buildEventV1(requestID, tenantID, initiatorID, txTime, "node.rescinded", "org_node", in.NodeID, in.EffectiveDate, endOfTime)
		res := &RescindNodeResult{
			NodeID:        in.NodeID,
			EffectiveDate: in.EffectiveDate,
			EndDate:       endOfTime,
			Status:        "rescinded",
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
		s.InvalidateTenantCache(tenantID)
	}
	return written, nil
}

type ShiftBoundaryNodeInput struct {
	NodeID              uuid.UUID
	TargetEffectiveDate time.Time
	NewEffectiveDate    time.Time
}

type ShiftBoundaryNodeResult struct {
	NodeID          uuid.UUID
	TargetStart     time.Time
	NewStart        time.Time
	GeneratedEvents []events.OrgEventV1
}

func (s *OrgService) ShiftBoundaryNode(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in ShiftBoundaryNodeInput) (*ShiftBoundaryNodeResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	txTime := time.Now().UTC()
	if in.NodeID == uuid.Nil || in.TargetEffectiveDate.IsZero() || in.NewEffectiveDate.IsZero() {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "id/target_effective_date/new_effective_date are required", nil)
	}

	if !in.NewEffectiveDate.Before(endOfTime) {
		return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_SHIFTBOUNDARY_INVERTED", "new_effective_date is invalid", nil)
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*ShiftBoundaryNodeResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}

		target, err := s.repo.LockNodeSliceStartingAt(txCtx, tenantID, in.NodeID, in.TargetEffectiveDate)
		if err != nil {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_NOT_FOUND_AT_DATE", "target slice not found", err)
		}
		if !in.NewEffectiveDate.Before(target.EndDate) {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_SHIFTBOUNDARY_INVERTED", "new_effective_date must be before target end_date", nil)
		}

		prev, err := s.repo.LockNodeSliceEndingAt(txCtx, tenantID, in.NodeID, in.TargetEffectiveDate)
		if err != nil {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_SHIFTBOUNDARY_SWALLOW", "previous slice not found", err)
		}
		if !in.NewEffectiveDate.After(prev.EffectiveDate) {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_SHIFTBOUNDARY_SWALLOW", "new_effective_date would swallow previous slice", nil)
		}

		affectedAt := in.TargetEffectiveDate
		if in.NewEffectiveDate.Before(affectedAt) {
			affectedAt = in.NewEffectiveDate
		}
		freeze, err := s.freezeCheck(settings, txTime, affectedAt)
		if err != nil {
			return nil, err
		}

		prevOld := map[string]any{
			"slice_id":       prev.ID.String(),
			"org_node_id":    in.NodeID.String(),
			"effective_date": prev.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":       prev.EndDate.UTC().Format(time.RFC3339),
		}
		targetOld := map[string]any{
			"slice_id":       target.ID.String(),
			"org_node_id":    in.NodeID.String(),
			"effective_date": target.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":       target.EndDate.UTC().Format(time.RFC3339),
		}

		if err := s.repo.UpdateNodeSliceEndDate(txCtx, tenantID, prev.ID, in.NewEffectiveDate); err != nil {
			return nil, mapPgError(err)
		}
		if err := s.repo.UpdateNodeSliceEffectiveDate(txCtx, tenantID, target.ID, in.NewEffectiveDate); err != nil {
			return nil, mapPgError(err)
		}

		prevNew := map[string]any{
			"slice_id":       prev.ID.String(),
			"org_node_id":    in.NodeID.String(),
			"effective_date": prev.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":       in.NewEffectiveDate.UTC().Format(time.RFC3339),
		}
		targetNew := map[string]any{
			"slice_id":       target.ID.String(),
			"org_node_id":    in.NodeID.String(),
			"effective_date": in.NewEffectiveDate.UTC().Format(time.RFC3339),
			"end_date":       target.EndDate.UTC().Format(time.RFC3339),
		}

		opDetails := map[string]any{
			"target_effective_date": in.TargetEffectiveDate.UTC().Format(time.RFC3339),
			"new_effective_date":    in.NewEffectiveDate.UTC().Format(time.RFC3339),
		}

		_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
			RequestID:        requestID,
			TransactionTime:  txTime,
			InitiatorID:      initiatorID,
			ChangeType:       "node.corrected",
			EntityType:       "org_node",
			EntityID:         in.NodeID,
			EffectiveDate:    prev.EffectiveDate,
			EndDate:          in.NewEffectiveDate,
			OldValues:        prevOld,
			NewValues:        prevNew,
			Operation:        "ShiftBoundary",
			OperationDetails: opDetails,
			FreezeMode:       freeze.Mode,
			FreezeViolation:  freeze.Violation,
			FreezeCutoffUTC:  freeze.CutoffUTC,
			AffectedAtUTC:    affectedAt,
		})
		if err != nil {
			return nil, err
		}

		_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
			RequestID:        requestID,
			TransactionTime:  txTime,
			InitiatorID:      initiatorID,
			ChangeType:       "node.corrected",
			EntityType:       "org_node",
			EntityID:         in.NodeID,
			EffectiveDate:    in.NewEffectiveDate,
			EndDate:          target.EndDate,
			OldValues:        targetOld,
			NewValues:        targetNew,
			Operation:        "ShiftBoundary",
			OperationDetails: opDetails,
			FreezeMode:       freeze.Mode,
			FreezeViolation:  freeze.Violation,
			FreezeCutoffUTC:  freeze.CutoffUTC,
			AffectedAtUTC:    affectedAt,
		})
		if err != nil {
			return nil, err
		}

		ev := buildEventV1(requestID, tenantID, initiatorID, txTime, "node.corrected", "org_node", in.NodeID, in.NewEffectiveDate, endOfTime)
		res := &ShiftBoundaryNodeResult{
			NodeID:      in.NodeID,
			TargetStart: in.TargetEffectiveDate,
			NewStart:    in.NewEffectiveDate,
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
		s.InvalidateTenantCache(tenantID)
	}
	return written, nil
}

type CorrectMoveNodeInput struct {
	NodeID        uuid.UUID
	EffectiveDate time.Time
	NewParentID   uuid.UUID
}

type CorrectMoveNodeResult struct {
	NodeID          uuid.UUID
	EffectiveDate   time.Time
	GeneratedEvents []events.OrgEventV1
}

func (s *OrgService) CorrectMoveNode(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in CorrectMoveNodeInput) (*CorrectMoveNodeResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	txTime := time.Now().UTC()
	if in.NodeID == uuid.Nil || in.NewParentID == uuid.Nil || in.EffectiveDate.IsZero() {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "id/new_parent_id/effective_date are required", nil)
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*CorrectMoveNodeResult, error) {
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
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_CANNOT_MOVE_ROOT", "cannot move root", nil)
		}

		hierarchyType := "OrgUnit"
		parentExists, err := s.repo.NodeExistsAt(txCtx, tenantID, in.NewParentID, hierarchyType, in.EffectiveDate)
		if err != nil {
			return nil, err
		}
		if !parentExists {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_PARENT_NOT_FOUND", "new_parent_id not found at effective_date", nil)
		}

		movedEdge, err := s.repo.LockEdgeStartingAt(txCtx, tenantID, hierarchyType, in.NodeID, in.EffectiveDate)
		if err != nil {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_USE_MOVE", "use move (insert) when effective_date is not edge slice start", err)
		}

		subtree, err := s.repo.LockEdgesInSubtree(txCtx, tenantID, hierarchyType, in.EffectiveDate, movedEdge.Path)
		if err != nil {
			return nil, err
		}

		if err := s.repo.DeleteEdgeByID(txCtx, tenantID, movedEdge.ID); err != nil {
			return nil, err
		}
		newEdgeID, err := s.repo.InsertEdge(txCtx, tenantID, hierarchyType, &in.NewParentID, in.NodeID, in.EffectiveDate, movedEdge.EndDate)
		if err != nil {
			return nil, mapPgError(err)
		}

		for _, e := range subtree {
			if e.ChildNodeID == in.NodeID {
				continue
			}
			if e.EffectiveDate.Equal(in.EffectiveDate) {
				if err := s.repo.DeleteEdgeByID(txCtx, tenantID, e.ID); err != nil {
					return nil, err
				}
				if _, err := s.repo.InsertEdge(txCtx, tenantID, hierarchyType, e.ParentNodeID, e.ChildNodeID, in.EffectiveDate, e.EndDate); err != nil {
					return nil, mapPgError(err)
				}
				continue
			}
			if err := s.repo.TruncateEdge(txCtx, tenantID, e.ID, in.EffectiveDate); err != nil {
				return nil, mapPgError(err)
			}
			if _, err := s.repo.InsertEdge(txCtx, tenantID, hierarchyType, e.ParentNodeID, e.ChildNodeID, in.EffectiveDate, e.EndDate); err != nil {
				return nil, mapPgError(err)
			}
		}

		nodeSlice, err := s.repo.LockNodeSliceAt(txCtx, tenantID, in.NodeID, in.EffectiveDate)
		if err == nil {
			if nodeSlice.EffectiveDate.Equal(in.EffectiveDate) {
				parent := in.NewParentID
				parentPtr := &parent
				patch := NodeSliceInPlacePatch{ParentHint: &parentPtr}
				if err := s.repo.UpdateNodeSliceInPlace(txCtx, tenantID, nodeSlice.ID, patch); err != nil {
					return nil, err
				}
			} else {
				if err := s.repo.TruncateNodeSlice(txCtx, tenantID, nodeSlice.ID, in.EffectiveDate); err != nil {
					return nil, err
				}
				_, err := s.repo.InsertNodeSlice(txCtx, tenantID, in.NodeID, NodeSliceInsert{
					Name:          nodeSlice.Name,
					I18nNames:     nodeSlice.I18nNames,
					Status:        nodeSlice.Status,
					LegalEntityID: nodeSlice.LegalEntityID,
					CompanyCode:   nodeSlice.CompanyCode,
					LocationID:    nodeSlice.LocationID,
					DisplayOrder:  nodeSlice.DisplayOrder,
					ParentHint:    &in.NewParentID,
					ManagerUserID: nodeSlice.ManagerUserID,
					EffectiveDate: in.EffectiveDate,
					EndDate:       nodeSlice.EndDate,
				})
				if err != nil {
					return nil, mapPgError(err)
				}
			}
		}

		_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
			RequestID:       requestID,
			TransactionTime: txTime,
			InitiatorID:     initiatorID,
			ChangeType:      "edge.corrected",
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
			Operation:       "CorrectMove",
			FreezeMode:      freeze.Mode,
			FreezeViolation: freeze.Violation,
			FreezeCutoffUTC: freeze.CutoffUTC,
			AffectedAtUTC:   in.EffectiveDate,
		})
		if err != nil {
			return nil, err
		}

		ev := buildEventV1(requestID, tenantID, initiatorID, txTime, "edge.corrected", "org_edge", newEdgeID, in.EffectiveDate, movedEdge.EndDate)
		res := &CorrectMoveNodeResult{
			NodeID:        in.NodeID,
			EffectiveDate: in.EffectiveDate,
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
		s.InvalidateTenantCache(tenantID)
	}
	return written, nil
}

type CorrectAssignmentInput struct {
	AssignmentID uuid.UUID
	Pernr        *string
	PositionID   *uuid.UUID
	SubjectID    *uuid.UUID
}

type CorrectAssignmentResult struct {
	AssignmentID    uuid.UUID
	EffectiveDate   time.Time
	EndDate         time.Time
	GeneratedEvents []events.OrgEventV1
}

func (s *OrgService) CorrectAssignment(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in CorrectAssignmentInput) (*CorrectAssignmentResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	txTime := time.Now().UTC()
	if in.AssignmentID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "id is required", nil)
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*CorrectAssignmentResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}

		current, err := s.repo.LockAssignmentByID(txCtx, tenantID, in.AssignmentID)
		if err != nil {
			return nil, mapPgError(err)
		}

		freeze, err := s.freezeCheck(settings, txTime, current.EffectiveDate)
		if err != nil {
			return nil, err
		}

		oldValues := map[string]any{
			"assignment_id":   current.ID.String(),
			"position_id":     current.PositionID.String(),
			"subject_type":    current.SubjectType,
			"subject_id":      current.SubjectID.String(),
			"pernr":           current.Pernr,
			"assignment_type": current.AssignmentType,
			"is_primary":      current.IsPrimary,
			"effective_date":  current.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":        current.EndDate.UTC().Format(time.RFC3339),
		}

		pernr := current.Pernr
		if in.Pernr != nil {
			pernr = strings.TrimSpace(*in.Pernr)
		}
		if pernr == "" {
			return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "pernr cannot be empty", nil)
		}

		derivedSubjectID, err := subjectid.NormalizedSubjectID(tenantID, "person", pernr)
		if err != nil {
			return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", err.Error(), err)
		}
		if in.SubjectID != nil && *in.SubjectID != derivedSubjectID {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_SUBJECT_MISMATCH", "subject_id does not match SSOT mapping", nil)
		}

		if in.PositionID != nil {
			exists, err := s.repo.PositionExistsAt(txCtx, tenantID, *in.PositionID, current.EffectiveDate)
			if err != nil {
				return nil, err
			}
			if !exists {
				return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_POSITION_NOT_FOUND_AT_DATE", "position_id not found at effective_date", nil)
			}
		}

		patch := AssignmentInPlacePatch{
			PositionID: in.PositionID,
			Pernr:      &pernr,
			SubjectID:  &derivedSubjectID,
		}
		if err := s.repo.UpdateAssignmentInPlace(txCtx, tenantID, current.ID, patch); err != nil {
			return nil, mapPgError(err)
		}

		updated, err := s.repo.LockAssignmentByID(txCtx, tenantID, current.ID)
		if err != nil {
			return nil, mapPgError(err)
		}

		newValues := map[string]any{
			"assignment_id":   updated.ID.String(),
			"position_id":     updated.PositionID.String(),
			"subject_type":    updated.SubjectType,
			"subject_id":      updated.SubjectID.String(),
			"pernr":           updated.Pernr,
			"assignment_type": updated.AssignmentType,
			"is_primary":      updated.IsPrimary,
			"effective_date":  updated.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":        updated.EndDate.UTC().Format(time.RFC3339),
		}

		_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
			RequestID:       requestID,
			TransactionTime: txTime,
			InitiatorID:     initiatorID,
			ChangeType:      "assignment.corrected",
			EntityType:      "org_assignment",
			EntityID:        updated.ID,
			EffectiveDate:   updated.EffectiveDate,
			EndDate:         updated.EndDate,
			OldValues:       oldValues,
			NewValues:       newValues,
			Operation:       "Correct",
			FreezeMode:      freeze.Mode,
			FreezeViolation: freeze.Violation,
			FreezeCutoffUTC: freeze.CutoffUTC,
			AffectedAtUTC:   current.EffectiveDate,
		})
		if err != nil {
			return nil, err
		}

		ev := buildEventV1(requestID, tenantID, initiatorID, txTime, "assignment.corrected", "org_assignment", updated.ID, updated.EffectiveDate, updated.EndDate)
		res := &CorrectAssignmentResult{
			AssignmentID:  updated.ID,
			EffectiveDate: updated.EffectiveDate,
			EndDate:       updated.EndDate,
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
		s.InvalidateTenantCache(tenantID)
	}
	return written, nil
}

type RescindAssignmentInput struct {
	AssignmentID  uuid.UUID
	EffectiveDate time.Time
	Reason        string
}

type RescindAssignmentResult struct {
	AssignmentID    uuid.UUID
	EffectiveDate   time.Time
	EndDate         time.Time
	GeneratedEvents []events.OrgEventV1
}

func (s *OrgService) RescindAssignment(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in RescindAssignmentInput) (*RescindAssignmentResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	txTime := time.Now().UTC()
	if in.AssignmentID == uuid.Nil || in.EffectiveDate.IsZero() {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "id/effective_date are required", nil)
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*RescindAssignmentResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}
		freeze, err := s.freezeCheck(settings, txTime, in.EffectiveDate)
		if err != nil {
			return nil, err
		}

		current, err := s.repo.LockAssignmentByID(txCtx, tenantID, in.AssignmentID)
		if err != nil {
			return nil, mapPgError(err)
		}
		if !in.EffectiveDate.After(current.EffectiveDate) || !in.EffectiveDate.Before(current.EndDate) {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_INVALID_RESCIND_DATE", "effective_date must be within current window", nil)
		}

		oldValues := map[string]any{
			"assignment_id":  current.ID.String(),
			"effective_date": current.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":       current.EndDate.UTC().Format(time.RFC3339),
		}

		if err := s.repo.UpdateAssignmentEndDate(txCtx, tenantID, current.ID, in.EffectiveDate); err != nil {
			return nil, mapPgError(err)
		}

		updated, err := s.repo.LockAssignmentByID(txCtx, tenantID, current.ID)
		if err != nil {
			return nil, mapPgError(err)
		}

		newValues := map[string]any{
			"assignment_id":  updated.ID.String(),
			"effective_date": updated.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":       updated.EndDate.UTC().Format(time.RFC3339),
			"reason":         strings.TrimSpace(in.Reason),
		}

		_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
			RequestID:       requestID,
			TransactionTime: txTime,
			InitiatorID:     initiatorID,
			ChangeType:      "assignment.rescinded",
			EntityType:      "org_assignment",
			EntityID:        updated.ID,
			EffectiveDate:   updated.EffectiveDate,
			EndDate:         updated.EndDate,
			OldValues:       oldValues,
			NewValues:       newValues,
			Operation:       "Rescind",
			Meta: map[string]any{
				"reason": strings.TrimSpace(in.Reason),
			},
			FreezeMode:      freeze.Mode,
			FreezeViolation: freeze.Violation,
			FreezeCutoffUTC: freeze.CutoffUTC,
			AffectedAtUTC:   in.EffectiveDate,
		})
		if err != nil {
			return nil, err
		}

		ev := buildEventV1(requestID, tenantID, initiatorID, txTime, "assignment.rescinded", "org_assignment", updated.ID, updated.EffectiveDate, updated.EndDate)
		res := &RescindAssignmentResult{
			AssignmentID:  updated.ID,
			EffectiveDate: updated.EffectiveDate,
			EndDate:       updated.EndDate,
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
		s.InvalidateTenantCache(tenantID)
	}
	return written, nil
}
