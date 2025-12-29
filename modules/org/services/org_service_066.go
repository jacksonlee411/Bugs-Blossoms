package services

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sirupsen/logrus"

	"github.com/iota-uz/iota-sdk/modules/org/domain/events"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

type DeleteNodeSliceAndStitchInput struct {
	NodeID              uuid.UUID
	TargetEffectiveDate time.Time
	ReasonCode          string
	ReasonNote          *string
}

type DeleteNodeSliceAndStitchResult struct {
	NodeID          uuid.UUID
	DeletedStart    time.Time
	DeletedEnd      time.Time
	Stitched        bool
	GeneratedEvents []events.OrgEventV1
}

func (s *OrgService) DeleteNodeSliceAndStitch(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in DeleteNodeSliceAndStitchInput) (*DeleteNodeSliceAndStitchResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	txTime := time.Now().UTC()
	if in.NodeID == uuid.Nil || in.TargetEffectiveDate.IsZero() {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "node_id/target_effective_date are required", nil)
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*DeleteNodeSliceAndStitchResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}
		reasonCode, reasonInfo, svcErr := normalizeReasonCode(settings, in.ReasonCode)
		if svcErr != nil {
			logReasonCodeRejected(txCtx, tenantID, requestID, "node.corrected", reasonInfo, svcErr)
			return nil, svcErr
		}

		if err := lockTimeline(txCtx, "org_node_slices", tenantID, in.NodeID.String()); err != nil {
			return nil, err
		}

		target, err := s.repo.LockNodeSliceStartingAt(txCtx, tenantID, in.NodeID, in.TargetEffectiveDate)
		if err != nil {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_NOT_FOUND_AT_DATE", "target slice not found", err)
		}

		freeze, err := s.freezeCheck(settings, txTime, target.EffectiveDate)
		if err != nil {
			maybeLogFrozenWindowRejected(txCtx, tenantID, requestID, initiatorID, "node.corrected", "org_node", in.NodeID, target.EffectiveDate, freeze, err, logrus.Fields{
				"node_id":    in.NodeID.String(),
				"operation":  "DeleteSliceAndStitch",
				"slice_id":   target.ID.String(),
				"effective":  target.EffectiveDate.UTC().Format(time.RFC3339),
				"end_date":   target.EndDate.UTC().Format(time.RFC3339),
				"table_name": "org_node_slices",
			})
			return nil, err
		}

		var (
			prev     NodeSliceRow
			hasPrev  bool
			prevOld  map[string]any
			prevNew  map[string]any
			opDetail = map[string]any{
				"target_effective_date": normalizeValidDateUTC(in.TargetEffectiveDate).Format(time.RFC3339),
				"deleted_slice_id":      target.ID.String(),
			}
		)
		prevEnd := truncateEndDateFromNewEffectiveDate(target.EffectiveDate)
		prev, err = s.repo.LockNodeSliceEndingAt(txCtx, tenantID, in.NodeID, prevEnd)
		if err == nil {
			hasPrev = true
		} else if err != nil && err != pgx.ErrNoRows {
			return nil, err
		}

		if hasPrev {
			prevOld = map[string]any{
				"slice_id":       prev.ID.String(),
				"org_node_id":    in.NodeID.String(),
				"effective_date": prev.EffectiveDate.UTC().Format(time.RFC3339),
				"end_date":       prev.EndDate.UTC().Format(time.RFC3339),
			}
			prevNew = map[string]any{
				"slice_id":       prev.ID.String(),
				"org_node_id":    in.NodeID.String(),
				"effective_date": prev.EffectiveDate.UTC().Format(time.RFC3339),
				"end_date":       target.EndDate.UTC().Format(time.RFC3339),
			}
		}

		targetOld := map[string]any{
			"slice_id":       target.ID.String(),
			"org_node_id":    in.NodeID.String(),
			"effective_date": target.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":       target.EndDate.UTC().Format(time.RFC3339),
		}

		if err := s.repo.DeleteNodeSliceByID(txCtx, tenantID, target.ID); err != nil {
			return nil, mapPgError(err)
		}
		if hasPrev {
			if err := s.repo.UpdateNodeSliceEndDate(txCtx, tenantID, prev.ID, target.EndDate); err != nil {
				return nil, mapPgError(err)
			}
		}

		if hasPrev {
			_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
				RequestID:        requestID,
				TransactionTime:  txTime,
				InitiatorID:      initiatorID,
				ChangeType:       "node.corrected",
				EntityType:       "org_node",
				EntityID:         in.NodeID,
				EffectiveDate:    prev.EffectiveDate,
				EndDate:          target.EndDate,
				OldValues:        prevOld,
				NewValues:        prevNew,
				Meta:             buildReasonMeta(reasonCode, in.ReasonNote, reasonInfo),
				Operation:        "DeleteSliceAndStitch",
				OperationDetails: opDetail,
				FreezeMode:       freeze.Mode,
				FreezeViolation:  freeze.Violation,
				FreezeCutoffUTC:  freeze.CutoffUTC,
				AffectedAtUTC:    target.EffectiveDate,
			})
			if err != nil {
				return nil, err
			}
		}

		_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
			RequestID:        requestID,
			TransactionTime:  txTime,
			InitiatorID:      initiatorID,
			ChangeType:       "node.corrected",
			EntityType:       "org_node",
			EntityID:         in.NodeID,
			EffectiveDate:    target.EffectiveDate,
			EndDate:          target.EndDate,
			OldValues:        targetOld,
			NewValues:        map[string]any{"deleted": true},
			Meta:             buildReasonMeta(reasonCode, in.ReasonNote, reasonInfo),
			Operation:        "DeleteSliceAndStitch",
			OperationDetails: opDetail,
			FreezeMode:       freeze.Mode,
			FreezeViolation:  freeze.Violation,
			FreezeCutoffUTC:  freeze.CutoffUTC,
			AffectedAtUTC:    target.EffectiveDate,
		})
		if err != nil {
			return nil, err
		}

		ev := buildEventV1(requestID, tenantID, initiatorID, txTime, "node.corrected", "org_node", in.NodeID, target.EffectiveDate, endOfTime)
		res := &DeleteNodeSliceAndStitchResult{
			NodeID:       in.NodeID,
			DeletedStart: target.EffectiveDate,
			DeletedEnd:   target.EndDate,
			Stitched:     hasPrev,
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

type DeletePositionSliceAndStitchInput struct {
	PositionID          uuid.UUID
	TargetEffectiveDate time.Time
	ReasonCode          string
	ReasonNote          *string
}

type DeletePositionSliceAndStitchResult struct {
	PositionID      uuid.UUID
	DeletedStart    time.Time
	DeletedEnd      time.Time
	Stitched        bool
	GeneratedEvents []events.OrgEventV1
}

func (s *OrgService) DeletePositionSliceAndStitch(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in DeletePositionSliceAndStitchInput) (*DeletePositionSliceAndStitchResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	txTime := time.Now().UTC()
	if in.PositionID == uuid.Nil || in.TargetEffectiveDate.IsZero() {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "position_id/target_effective_date are required", nil)
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*DeletePositionSliceAndStitchResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}
		reasonCode, reasonInfo, svcErr := normalizeReasonCode(settings, in.ReasonCode)
		if svcErr != nil {
			logReasonCodeRejected(txCtx, tenantID, requestID, "position.corrected", reasonInfo, svcErr)
			return nil, svcErr
		}

		if err := lockTimeline(txCtx, "org_position_slices", tenantID, in.PositionID.String()); err != nil {
			return nil, err
		}

		target, err := s.repo.LockPositionSliceStartingAt(txCtx, tenantID, in.PositionID, in.TargetEffectiveDate)
		if err != nil {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_POSITION_NOT_FOUND_AT_DATE", "target slice not found", err)
		}

		freeze, err := s.freezeCheck(settings, txTime, target.EffectiveDate)
		if err != nil {
			maybeLogFrozenWindowRejected(txCtx, tenantID, requestID, initiatorID, "position.corrected", "org_position", in.PositionID, target.EffectiveDate, freeze, err, logrus.Fields{
				"position_id": in.PositionID.String(),
				"operation":   "DeleteSliceAndStitch",
				"slice_id":    target.ID.String(),
			})
			return nil, err
		}

		var (
			prev    PositionSliceRow
			hasPrev bool
		)
		prevEnd := truncateEndDateFromNewEffectiveDate(target.EffectiveDate)
		prev, err = s.repo.LockPositionSliceEndingAt(txCtx, tenantID, in.PositionID, prevEnd)
		if err == nil {
			hasPrev = true
		} else if err != nil && err != pgx.ErrNoRows {
			return nil, err
		}

		opDetail := map[string]any{
			"target_effective_date": normalizeValidDateUTC(in.TargetEffectiveDate).Format(time.RFC3339),
			"deleted_slice_id":      target.ID.String(),
		}

		targetOld := map[string]any{
			"slice_id":       target.ID.String(),
			"position_id":    in.PositionID.String(),
			"effective_date": target.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":       target.EndDate.UTC().Format(time.RFC3339),
		}

		if err := s.repo.DeletePositionSliceByID(txCtx, tenantID, target.ID); err != nil {
			return nil, mapPgError(err)
		}
		if hasPrev {
			if err := s.repo.UpdatePositionSliceEndDate(txCtx, tenantID, prev.ID, target.EndDate); err != nil {
				return nil, mapPgError(err)
			}
		}

		if hasPrev {
			prevOld := map[string]any{
				"slice_id":       prev.ID.String(),
				"position_id":    in.PositionID.String(),
				"effective_date": prev.EffectiveDate.UTC().Format(time.RFC3339),
				"end_date":       prev.EndDate.UTC().Format(time.RFC3339),
			}
			prevNew := map[string]any{
				"slice_id":       prev.ID.String(),
				"position_id":    in.PositionID.String(),
				"effective_date": prev.EffectiveDate.UTC().Format(time.RFC3339),
				"end_date":       target.EndDate.UTC().Format(time.RFC3339),
			}

			_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
				RequestID:        requestID,
				TransactionTime:  txTime,
				InitiatorID:      initiatorID,
				ChangeType:       "position.corrected",
				EntityType:       "org_position",
				EntityID:         in.PositionID,
				EffectiveDate:    prev.EffectiveDate,
				EndDate:          target.EndDate,
				OldValues:        prevOld,
				NewValues:        prevNew,
				Meta:             buildReasonMeta(reasonCode, in.ReasonNote, reasonInfo),
				Operation:        "DeleteSliceAndStitch",
				OperationDetails: opDetail,
				FreezeMode:       freeze.Mode,
				FreezeViolation:  freeze.Violation,
				FreezeCutoffUTC:  freeze.CutoffUTC,
				AffectedAtUTC:    target.EffectiveDate,
			})
			if err != nil {
				return nil, err
			}
		}

		_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
			RequestID:        requestID,
			TransactionTime:  txTime,
			InitiatorID:      initiatorID,
			ChangeType:       "position.corrected",
			EntityType:       "org_position",
			EntityID:         in.PositionID,
			EffectiveDate:    target.EffectiveDate,
			EndDate:          target.EndDate,
			OldValues:        targetOld,
			NewValues:        map[string]any{"deleted": true},
			Meta:             buildReasonMeta(reasonCode, in.ReasonNote, reasonInfo),
			Operation:        "DeleteSliceAndStitch",
			OperationDetails: opDetail,
			FreezeMode:       freeze.Mode,
			FreezeViolation:  freeze.Violation,
			FreezeCutoffUTC:  freeze.CutoffUTC,
			AffectedAtUTC:    target.EffectiveDate,
		})
		if err != nil {
			return nil, err
		}

		ev := buildEventV1(requestID, tenantID, initiatorID, txTime, "position.corrected", "org_position", in.PositionID, target.EffectiveDate, endOfTime)
		res := &DeletePositionSliceAndStitchResult{
			PositionID:   in.PositionID,
			DeletedStart: target.EffectiveDate,
			DeletedEnd:   target.EndDate,
			Stitched:     hasPrev,
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

type DeleteEdgeSliceAndStitchInput struct {
	HierarchyType       string
	ChildNodeID         uuid.UUID
	TargetEffectiveDate time.Time
	ReasonCode          string
	ReasonNote          *string
}

type DeleteEdgeSliceAndStitchResult struct {
	DeletedEdgeID   uuid.UUID
	DeletedStart    time.Time
	DeletedEnd      time.Time
	StitchedToEdge  *uuid.UUID
	GeneratedEvents []events.OrgEventV1
}

func (s *OrgService) DeleteEdgeSliceAndStitch(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in DeleteEdgeSliceAndStitchInput) (*DeleteEdgeSliceAndStitchResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	txTime := time.Now().UTC()
	if in.ChildNodeID == uuid.Nil || in.TargetEffectiveDate.IsZero() || in.HierarchyType == "" {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "child_node_id/hierarchy_type/target_effective_date are required", nil)
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*DeleteEdgeSliceAndStitchResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}
		reasonCode, reasonInfo, svcErr := normalizeReasonCode(settings, in.ReasonCode)
		if svcErr != nil {
			logReasonCodeRejected(txCtx, tenantID, requestID, "edge.corrected", reasonInfo, svcErr)
			return nil, svcErr
		}

		if err := lockTimeline(txCtx, "org_edges", tenantID, fmt.Sprintf("%s:%s", in.HierarchyType, in.ChildNodeID.String())); err != nil {
			return nil, err
		}

		target, err := s.repo.LockEdgeStartingAt(txCtx, tenantID, in.HierarchyType, in.ChildNodeID, in.TargetEffectiveDate)
		if err != nil {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_EDGE_NOT_FOUND_AT_DATE", "target slice not found", err)
		}

		freeze, err := s.freezeCheck(settings, txTime, target.EffectiveDate)
		if err != nil {
			maybeLogFrozenWindowRejected(txCtx, tenantID, requestID, initiatorID, "edge.corrected", "org_edge", target.ID, target.EffectiveDate, freeze, err, logrus.Fields{
				"edge_id":       target.ID.String(),
				"hierarchyType": in.HierarchyType,
				"child_node_id": in.ChildNodeID.String(),
				"operation":     "DeleteSliceAndStitch",
			})
			return nil, err
		}

		var (
			prev    EdgeRow
			hasPrev bool
		)
		prevEnd := truncateEndDateFromNewEffectiveDate(target.EffectiveDate)
		prev, err = s.repo.LockEdgeEndingAt(txCtx, tenantID, in.HierarchyType, in.ChildNodeID, prevEnd)
		if err == nil {
			hasPrev = true
		} else if err != nil && err != pgx.ErrNoRows {
			return nil, err
		}

		if !hasPrev {
			return nil, newServiceError(
				http.StatusUnprocessableEntity,
				"ORG_CANNOT_DELETE_FIRST_EDGE_SLICE",
				"cannot delete the first edge slice (no previous slice to stitch)",
				nil,
			)
		}

		oldPrefix := target.Path
		affectedEdges, err := s.repo.CountDescendantEdgesNeedingPathRewriteFrom(txCtx, tenantID, in.HierarchyType, target.EffectiveDate, oldPrefix)
		if err != nil {
			return nil, err
		}
		if affectedEdges > maxEdgesPathRewrite {
			return nil, newServiceError(
				http.StatusUnprocessableEntity,
				"ORG_PREFLIGHT_TOO_LARGE",
				fmt.Sprintf(
					"subtree path rewrite impact is too large (affected_edges=%d, limit=%d, effective_date=%s, node_id=%s)",
					affectedEdges,
					maxEdgesPathRewrite,
					target.EffectiveDate.UTC().Format(time.DateOnly),
					in.ChildNodeID.String(),
				),
				nil,
			)
		}

		opDetail := map[string]any{
			"target_effective_date": normalizeValidDateUTC(in.TargetEffectiveDate).Format(time.RFC3339),
			"deleted_edge_id":       target.ID.String(),
			"path_rewrite_affected": affectedEdges,
		}

		targetOld := map[string]any{
			"edge_id":         target.ID.String(),
			"child_node_id":   target.ChildNodeID.String(),
			"parent_node_id":  nil,
			"path":            target.Path,
			"depth":           target.Depth,
			"effective_date":  target.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":        target.EndDate.UTC().Format(time.RFC3339),
			"hierarchy_type":  in.HierarchyType,
			"timeline_key":    fmt.Sprintf("%s:%s", in.HierarchyType, in.ChildNodeID.String()),
			"tenant_id":       tenantID.String(),
			"stitch_prev_end": prevEnd.UTC().Format(time.RFC3339),
		}
		if target.ParentNodeID != nil {
			targetOld["parent_node_id"] = target.ParentNodeID.String()
		}

		if err := s.repo.DeleteEdgeByID(txCtx, tenantID, target.ID); err != nil {
			return nil, mapPgError(err)
		}
		if err := s.repo.TruncateEdge(txCtx, tenantID, prev.ID, target.EndDate); err != nil {
			return nil, mapPgError(err)
		}

		newEdgeAt, err := s.repo.LockEdgeAt(txCtx, tenantID, in.HierarchyType, in.ChildNodeID, target.EffectiveDate)
		if err != nil {
			return nil, mapPgError(err)
		}
		newPrefix := newEdgeAt.Path
		opDetail["path_rewrite_old_prefix"] = oldPrefix
		opDetail["path_rewrite_new_prefix"] = newPrefix

		if affectedEdges > 0 && oldPrefix != newPrefix {
			if _, err := s.repo.RewriteDescendantEdgesPathPrefixFrom(txCtx, tenantID, in.HierarchyType, target.EffectiveDate, oldPrefix, newPrefix); err != nil {
				return nil, err
			}
		}

		var stitchedTo *uuid.UUID
		prevOld := map[string]any{
			"edge_id":        prev.ID.String(),
			"effective_date": prev.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":       prev.EndDate.UTC().Format(time.RFC3339),
		}
		prevNew := map[string]any{
			"edge_id":        prev.ID.String(),
			"effective_date": prev.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":       target.EndDate.UTC().Format(time.RFC3339),
		}
		_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
			RequestID:        requestID,
			TransactionTime:  txTime,
			InitiatorID:      initiatorID,
			ChangeType:       "edge.corrected",
			EntityType:       "org_edge",
			EntityID:         prev.ID,
			EffectiveDate:    prev.EffectiveDate,
			EndDate:          target.EndDate,
			OldValues:        prevOld,
			NewValues:        prevNew,
			Meta:             buildReasonMeta(reasonCode, in.ReasonNote, reasonInfo),
			Operation:        "DeleteSliceAndStitch",
			OperationDetails: opDetail,
			FreezeMode:       freeze.Mode,
			FreezeViolation:  freeze.Violation,
			FreezeCutoffUTC:  freeze.CutoffUTC,
			AffectedAtUTC:    target.EffectiveDate,
		})
		if err != nil {
			return nil, err
		}
		stitchedTo = &prev.ID

		_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
			RequestID:        requestID,
			TransactionTime:  txTime,
			InitiatorID:      initiatorID,
			ChangeType:       "edge.corrected",
			EntityType:       "org_edge",
			EntityID:         target.ID,
			EffectiveDate:    target.EffectiveDate,
			EndDate:          target.EndDate,
			OldValues:        targetOld,
			NewValues:        map[string]any{"deleted": true},
			Meta:             buildReasonMeta(reasonCode, in.ReasonNote, reasonInfo),
			Operation:        "DeleteSliceAndStitch",
			OperationDetails: opDetail,
			FreezeMode:       freeze.Mode,
			FreezeViolation:  freeze.Violation,
			FreezeCutoffUTC:  freeze.CutoffUTC,
			AffectedAtUTC:    target.EffectiveDate,
		})
		if err != nil {
			return nil, err
		}

		evID := prev.ID
		ev := buildEventV1(requestID, tenantID, initiatorID, txTime, "edge.corrected", "org_edge", evID, target.EffectiveDate, endOfTime)
		res := &DeleteEdgeSliceAndStitchResult{
			DeletedEdgeID:  target.ID,
			DeletedStart:   target.EffectiveDate,
			DeletedEnd:     target.EndDate,
			StitchedToEdge: stitchedTo,
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

type DeleteAssignmentAndStitchInput struct {
	AssignmentID uuid.UUID
	ReasonCode   string
	ReasonNote   *string
}

type DeleteAssignmentAndStitchResult struct {
	DeletedAssignmentID uuid.UUID
	DeletedStart        time.Time
	DeletedEnd          time.Time
	StitchedTo          *uuid.UUID
	GeneratedEvents     []events.OrgEventV1
}

func (s *OrgService) DeleteAssignmentAndStitch(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in DeleteAssignmentAndStitchInput) (*DeleteAssignmentAndStitchResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	txTime := time.Now().UTC()
	if in.AssignmentID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "assignment_id is required", nil)
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*DeleteAssignmentAndStitchResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}
		reasonCode, reasonInfo, svcErr := normalizeReasonCode(settings, in.ReasonCode)
		if svcErr != nil {
			logReasonCodeRejected(txCtx, tenantID, requestID, "assignment.corrected", reasonInfo, svcErr)
			return nil, svcErr
		}

		target, err := s.repo.LockAssignmentByID(txCtx, tenantID, in.AssignmentID)
		if err != nil {
			return nil, mapPgError(err)
		}
		if target.AssignmentType != "primary" || !target.IsPrimary {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_ASSIGNMENT_NOT_PRIMARY", "only primary assignment timeline supports delete and stitch", nil)
		}

		if err := lockTimeline(txCtx, "org_assignments", tenantID, fmt.Sprintf("%s:%s:%s", target.SubjectType, target.SubjectID.String(), target.AssignmentType)); err != nil {
			return nil, err
		}

		freeze, err := s.freezeCheck(settings, txTime, target.EffectiveDate)
		if err != nil {
			maybeLogFrozenWindowRejected(txCtx, tenantID, requestID, initiatorID, "assignment.corrected", "org_assignment", target.ID, target.EffectiveDate, freeze, err, logrus.Fields{
				"assignment_id":   target.ID.String(),
				"position_id":     target.PositionID.String(),
				"pernr":           target.Pernr,
				"subject_id":      target.SubjectID.String(),
				"assignment_type": target.AssignmentType,
				"operation":       "DeleteSliceAndStitch",
			})
			return nil, err
		}

		var (
			prev    AssignmentRow
			hasPrev bool
		)
		prevEnd := truncateEndDateFromNewEffectiveDate(target.EffectiveDate)
		prev, err = s.repo.LockAssignmentEndingAtForTimeline(txCtx, tenantID, target.SubjectType, target.SubjectID, target.AssignmentType, prevEnd)
		if err == nil {
			hasPrev = true
		} else if err != nil && err != pgx.ErrNoRows {
			return nil, err
		}

		opDetail := map[string]any{
			"deleted_assignment_id": target.ID.String(),
		}

		targetOld := map[string]any{
			"assignment_id":  target.ID.String(),
			"effective_date": target.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":       target.EndDate.UTC().Format(time.RFC3339),
		}

		if err := s.repo.DeleteAssignmentByID(txCtx, tenantID, target.ID); err != nil {
			return nil, mapPgError(err)
		}
		if hasPrev {
			if err := s.repo.UpdateAssignmentEndDate(txCtx, tenantID, prev.ID, target.EndDate); err != nil {
				return nil, mapPgError(err)
			}
		}

		var stitchedTo *uuid.UUID
		if hasPrev {
			prevOld := map[string]any{
				"assignment_id":  prev.ID.String(),
				"effective_date": prev.EffectiveDate.UTC().Format(time.RFC3339),
				"end_date":       prev.EndDate.UTC().Format(time.RFC3339),
			}
			prevNew := map[string]any{
				"assignment_id":  prev.ID.String(),
				"effective_date": prev.EffectiveDate.UTC().Format(time.RFC3339),
				"end_date":       target.EndDate.UTC().Format(time.RFC3339),
			}

			_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
				RequestID:        requestID,
				TransactionTime:  txTime,
				InitiatorID:      initiatorID,
				ChangeType:       "assignment.corrected",
				EntityType:       "org_assignment",
				EntityID:         prev.ID,
				EffectiveDate:    prev.EffectiveDate,
				EndDate:          target.EndDate,
				OldValues:        prevOld,
				NewValues:        prevNew,
				Meta:             buildReasonMeta(reasonCode, in.ReasonNote, reasonInfo),
				Operation:        "DeleteSliceAndStitch",
				OperationDetails: opDetail,
				FreezeMode:       freeze.Mode,
				FreezeViolation:  freeze.Violation,
				FreezeCutoffUTC:  freeze.CutoffUTC,
				AffectedAtUTC:    target.EffectiveDate,
			})
			if err != nil {
				return nil, err
			}
			stitchedTo = &prev.ID
		}

		_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
			RequestID:        requestID,
			TransactionTime:  txTime,
			InitiatorID:      initiatorID,
			ChangeType:       "assignment.corrected",
			EntityType:       "org_assignment",
			EntityID:         target.ID,
			EffectiveDate:    target.EffectiveDate,
			EndDate:          target.EndDate,
			OldValues:        targetOld,
			NewValues:        map[string]any{"deleted": true},
			Meta:             buildReasonMeta(reasonCode, in.ReasonNote, reasonInfo),
			Operation:        "DeleteSliceAndStitch",
			OperationDetails: opDetail,
			FreezeMode:       freeze.Mode,
			FreezeViolation:  freeze.Violation,
			FreezeCutoffUTC:  freeze.CutoffUTC,
			AffectedAtUTC:    target.EffectiveDate,
		})
		if err != nil {
			return nil, err
		}

		evID := target.ID
		if hasPrev {
			evID = prev.ID
		}
		ev := buildEventV1(requestID, tenantID, initiatorID, txTime, "assignment.corrected", "org_assignment", evID, target.EffectiveDate, endOfTime)
		res := &DeleteAssignmentAndStitchResult{
			DeletedAssignmentID: target.ID,
			DeletedStart:        target.EffectiveDate,
			DeletedEnd:          target.EndDate,
			StitchedTo:          stitchedTo,
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

func lockTimeline(ctx context.Context, tableName string, tenantID uuid.UUID, timelineKeyText string) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	lockKeyText := fmt.Sprintf("%s:%s:%s", tableName, tenantID.String(), timelineKeyText)
	_, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", lockKeyText)
	return err
}

func buildReasonMeta(reasonCode string, reasonNote *string, reasonInfo ReasonCodeInfo) map[string]any {
	meta := map[string]any{
		"reason_code": reasonCode,
		"reason_note": reasonNote,
	}
	addReasonCodeMeta(meta, reasonInfo)
	return meta
}
