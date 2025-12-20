package services

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/org/domain/events"
)

type CreatePositionInput struct {
	Code            string
	OrgNodeID       uuid.UUID
	EffectiveDate   time.Time
	Title           *string
	LifecycleStatus string
	CapacityFTE     float64
	ReportsToID     *uuid.UUID
	ReasonCode      string
	ReasonNote      *string
}

type CreatePositionResult struct {
	PositionID      uuid.UUID
	SliceID         uuid.UUID
	EffectiveDate   time.Time
	EndDate         time.Time
	GeneratedEvents []events.OrgEventV1
}

func (s *OrgService) CreatePosition(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in CreatePositionInput) (*CreatePositionResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	txTime := time.Now().UTC()

	code := strings.TrimSpace(in.Code)
	if code == "" || in.OrgNodeID == uuid.Nil || in.EffectiveDate.IsZero() {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "code/org_node_id/effective_date are required", nil)
	}
	if in.CapacityFTE <= 0 {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "capacity_fte must be > 0", nil)
	}

	lifecycle := strings.TrimSpace(in.LifecycleStatus)
	if lifecycle == "" {
		lifecycle = "active"
	}
	switch lifecycle {
	case "planned", "active", "inactive", "rescinded":
	default:
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "invalid lifecycle_status", nil)
	}

	reasonCode := strings.TrimSpace(in.ReasonCode)
	if reasonCode == "" {
		reasonCode = "legacy"
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*CreatePositionResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}
		freeze, err := s.freezeCheck(settings, txTime, in.EffectiveDate)
		if err != nil {
			return nil, err
		}

		hierarchyType := "OrgUnit"
		exists, err := s.repo.NodeExistsAt(txCtx, tenantID, in.OrgNodeID, hierarchyType, in.EffectiveDate)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_NODE_NOT_FOUND_AT_DATE", "org_node_id not found at effective_date", nil)
		}

		positionID := uuid.New()
		legacyStatus := "active"
		if lifecycle == "inactive" {
			legacyStatus = "retired"
		}
		if lifecycle == "rescinded" {
			legacyStatus = "rescinded"
		}

		if _, err := s.repo.InsertPosition(txCtx, tenantID, PositionInsert{
			PositionID:    positionID,
			OrgNodeID:     in.OrgNodeID,
			Code:          code,
			Title:         in.Title,
			LegacyStatus:  legacyStatus,
			IsAutoCreated: false,
			EffectiveDate: in.EffectiveDate,
			EndDate:       endOfTime,
		}); err != nil {
			return nil, mapPgError(err)
		}

		sliceID, err := s.repo.InsertPositionSlice(txCtx, tenantID, positionID, PositionSliceInsert{
			OrgNodeID:           in.OrgNodeID,
			Title:               in.Title,
			LifecycleStatus:     lifecycle,
			CapacityFTE:         in.CapacityFTE,
			ReportsToPositionID: in.ReportsToID,
			EffectiveDate:       in.EffectiveDate,
			EndDate:             endOfTime,
		})
		if err != nil {
			return nil, mapPgError(err)
		}

		newValues := map[string]any{
			"position_id":       positionID.String(),
			"code":              code,
			"org_node_id":       in.OrgNodeID.String(),
			"lifecycle_status":  lifecycle,
			"is_auto_created":   false,
			"capacity_fte":      in.CapacityFTE,
			"effective_date":    in.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":          endOfTime.UTC().Format(time.RFC3339),
			"reports_to_id":     in.ReportsToID,
			"position_slice_id": sliceID.String(),
		}

		_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
			RequestID:       requestID,
			TransactionTime: txTime,
			InitiatorID:     initiatorID,
			ChangeType:      "position.created",
			EntityType:      "org_position",
			EntityID:        positionID,
			EffectiveDate:   in.EffectiveDate,
			EndDate:         endOfTime,
			OldValues:       nil,
			NewValues:       newValues,
			Meta: map[string]any{
				"reason_code": reasonCode,
				"reason_note": in.ReasonNote,
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

		ev := buildEventV1(requestID, tenantID, initiatorID, txTime, "position.created", "org_position", positionID, in.EffectiveDate, endOfTime)
		if payload, err := json.Marshal(map[string]any{
			"position_id":      positionID.String(),
			"code":             code,
			"org_node_id":      in.OrgNodeID.String(),
			"lifecycle_status": lifecycle,
			"is_auto_created":  false,
			"capacity_fte":     in.CapacityFTE,
			"effective_date":   in.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":         endOfTime.UTC().Format(time.RFC3339),
		}); err == nil {
			ev.NewValues = payload
		}

		res := &CreatePositionResult{
			PositionID:    positionID,
			SliceID:       sliceID,
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

type GetPositionsInput struct {
	AsOf            *time.Time
	OrgNodeID       *uuid.UUID
	Q               *string
	LifecycleStatus *string
	IsAutoCreated   *bool
	Limit           int
	Offset          int
}

func (s *OrgService) GetPositions(ctx context.Context, tenantID uuid.UUID, in GetPositionsInput) ([]PositionViewRow, time.Time, error) {
	if tenantID == uuid.Nil {
		return nil, time.Time{}, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	asOf := time.Now().UTC()
	if in.AsOf != nil && !in.AsOf.IsZero() {
		asOf = (*in.AsOf).UTC()
	}

	rows, err := inTx(ctx, tenantID, func(txCtx context.Context) ([]PositionViewRow, error) {
		filter := PositionListFilter{
			OrgNodeID:       in.OrgNodeID,
			Q:               in.Q,
			LifecycleStatus: in.LifecycleStatus,
			IsAutoCreated:   in.IsAutoCreated,
			Limit:           in.Limit,
			Offset:          in.Offset,
		}
		return s.repo.ListPositionsAsOf(txCtx, tenantID, asOf, filter)
	})
	if err != nil {
		return nil, time.Time{}, err
	}
	return rows, asOf, nil
}

func (s *OrgService) GetPosition(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, asOf *time.Time) (PositionViewRow, time.Time, error) {
	if tenantID == uuid.Nil {
		return PositionViewRow{}, time.Time{}, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if positionID == uuid.Nil {
		return PositionViewRow{}, time.Time{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "position_id is required", nil)
	}
	t := time.Now().UTC()
	if asOf != nil && !asOf.IsZero() {
		t = (*asOf).UTC()
	}

	row, err := inTx(ctx, tenantID, func(txCtx context.Context) (PositionViewRow, error) {
		return s.repo.GetPositionAsOf(txCtx, tenantID, positionID, t)
	})
	if err != nil {
		return PositionViewRow{}, time.Time{}, mapPgError(err)
	}
	return row, t, nil
}

func (s *OrgService) GetPositionTimeline(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID) ([]PositionSliceRow, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if positionID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "position_id is required", nil)
	}
	return inTx(ctx, tenantID, func(txCtx context.Context) ([]PositionSliceRow, error) {
		return s.repo.ListPositionSlicesTimeline(txCtx, tenantID, positionID)
	})
}

type UpdatePositionInput struct {
	PositionID    uuid.UUID
	EffectiveDate time.Time
	ReasonCode    string
	ReasonNote    *string

	OrgNodeID       *uuid.UUID
	Title           *string
	LifecycleStatus *string
	CapacityFTE     *float64
	ReportsToID     *uuid.UUID
}

type UpdatePositionResult struct {
	PositionID      uuid.UUID
	SliceID         uuid.UUID
	EffectiveDate   time.Time
	EndDate         time.Time
	GeneratedEvents []events.OrgEventV1
}

func (s *OrgService) UpdatePosition(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in UpdatePositionInput) (*UpdatePositionResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	txTime := time.Now().UTC()
	if in.PositionID == uuid.Nil || in.EffectiveDate.IsZero() {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "position_id/effective_date are required", nil)
	}
	reasonCode := strings.TrimSpace(in.ReasonCode)
	if reasonCode == "" {
		reasonCode = "legacy"
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*UpdatePositionResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}
		freeze, err := s.freezeCheck(settings, txTime, in.EffectiveDate)
		if err != nil {
			return nil, err
		}

		current, err := s.repo.LockPositionSliceAt(txCtx, tenantID, in.PositionID, in.EffectiveDate)
		if err != nil {
			return nil, mapPgError(err)
		}
		if current.EffectiveDate.Equal(in.EffectiveDate) {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_USE_CORRECT", "use correct for in-place updates", nil)
		}

		next, hasNext, err := s.repo.NextPositionSliceEffectiveDate(txCtx, tenantID, in.PositionID, in.EffectiveDate)
		if err != nil {
			return nil, err
		}
		newEnd := current.EndDate
		if hasNext && next.Before(newEnd) {
			newEnd = next
		}

		orgNodeID := current.OrgNodeID
		if in.OrgNodeID != nil && *in.OrgNodeID != uuid.Nil {
			orgNodeID = *in.OrgNodeID
		}
		title := current.Title
		if in.Title != nil {
			title = in.Title
		}
		lifecycle := current.LifecycleStatus
		if in.LifecycleStatus != nil {
			lifecycle = strings.TrimSpace(*in.LifecycleStatus)
		}
		switch lifecycle {
		case "planned", "active", "inactive", "rescinded":
		default:
			return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "invalid lifecycle_status", nil)
		}
		capacity := current.CapacityFTE
		if in.CapacityFTE != nil {
			capacity = *in.CapacityFTE
		}
		if capacity <= 0 {
			return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "capacity_fte must be > 0", nil)
		}
		reportsTo := current.ReportsToPositionID
		if in.ReportsToID != nil {
			reportsTo = in.ReportsToID
		}

		hierarchyType := "OrgUnit"
		nodeExists, err := s.repo.NodeExistsAt(txCtx, tenantID, orgNodeID, hierarchyType, in.EffectiveDate)
		if err != nil {
			return nil, err
		}
		if !nodeExists {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_NODE_NOT_FOUND_AT_DATE", "org_node_id not found at effective_date", nil)
		}

		occupied, err := s.repo.SumAllocatedFTEAt(txCtx, tenantID, in.PositionID, in.EffectiveDate)
		if err != nil {
			return nil, err
		}
		if occupied > capacity {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_POSITION_OVER_CAPACITY", "position capacity would be below occupied_fte", nil)
		}

		if err := s.repo.TruncatePositionSlice(txCtx, tenantID, current.ID, in.EffectiveDate); err != nil {
			return nil, mapPgError(err)
		}

		sliceID, err := s.repo.InsertPositionSlice(txCtx, tenantID, in.PositionID, PositionSliceInsert{
			OrgNodeID:           orgNodeID,
			Title:               title,
			LifecycleStatus:     lifecycle,
			CapacityFTE:         capacity,
			ReportsToPositionID: reportsTo,
			EffectiveDate:       in.EffectiveDate,
			EndDate:             newEnd,
		})
		if err != nil {
			return nil, mapPgError(err)
		}

		_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
			RequestID:       requestID,
			TransactionTime: txTime,
			InitiatorID:     initiatorID,
			ChangeType:      "position.updated",
			EntityType:      "org_position",
			EntityID:        in.PositionID,
			EffectiveDate:   in.EffectiveDate,
			EndDate:         newEnd,
			OldValues: map[string]any{
				"position_id":      in.PositionID.String(),
				"org_node_id":      current.OrgNodeID.String(),
				"lifecycle_status": current.LifecycleStatus,
				"capacity_fte":     current.CapacityFTE,
				"effective_date":   current.EffectiveDate.UTC().Format(time.RFC3339),
				"end_date":         current.EndDate.UTC().Format(time.RFC3339),
			},
			NewValues: map[string]any{
				"position_id":      in.PositionID.String(),
				"org_node_id":      orgNodeID.String(),
				"lifecycle_status": lifecycle,
				"capacity_fte":     capacity,
				"effective_date":   in.EffectiveDate.UTC().Format(time.RFC3339),
				"end_date":         newEnd.UTC().Format(time.RFC3339),
			},
			Meta: map[string]any{
				"reason_code": reasonCode,
				"reason_note": in.ReasonNote,
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

		view, err := s.repo.GetPositionAsOf(txCtx, tenantID, in.PositionID, in.EffectiveDate)
		if err != nil {
			return nil, mapPgError(err)
		}

		ev := buildEventV1(requestID, tenantID, initiatorID, txTime, "position.updated", "org_position", in.PositionID, in.EffectiveDate, newEnd)
		if payload, err := json.Marshal(map[string]any{
			"position_id":      in.PositionID.String(),
			"code":             view.Code,
			"org_node_id":      view.OrgNodeID.String(),
			"lifecycle_status": view.LifecycleStatus,
			"is_auto_created":  view.IsAutoCreated,
			"capacity_fte":     view.CapacityFTE,
			"effective_date":   in.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":         newEnd.UTC().Format(time.RFC3339),
		}); err == nil {
			ev.NewValues = payload
		}

		res := &UpdatePositionResult{
			PositionID:    in.PositionID,
			SliceID:       sliceID,
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

type CorrectPositionInput struct {
	PositionID  uuid.UUID
	AsOf        time.Time
	ReasonCode  string
	ReasonNote  *string
	OrgNodeID   *uuid.UUID
	Title       *string
	Lifecycle   *string
	CapacityFTE *float64
	ReportsToID *uuid.UUID
}

type CorrectPositionResult struct {
	PositionID      uuid.UUID
	EffectiveDate   time.Time
	EndDate         time.Time
	GeneratedEvents []events.OrgEventV1
}

func (s *OrgService) CorrectPosition(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in CorrectPositionInput) (*CorrectPositionResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	txTime := time.Now().UTC()
	if in.PositionID == uuid.Nil || in.AsOf.IsZero() {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "position_id/effective_date are required", nil)
	}
	reasonCode := strings.TrimSpace(in.ReasonCode)
	if reasonCode == "" {
		reasonCode = "legacy"
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*CorrectPositionResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}

		target, err := s.repo.LockPositionSliceAt(txCtx, tenantID, in.PositionID, in.AsOf)
		if err != nil {
			return nil, mapPgError(err)
		}

		freeze, err := s.freezeCheck(settings, txTime, target.EffectiveDate)
		if err != nil {
			return nil, err
		}

		oldValues := map[string]any{
			"slice_id":         target.ID.String(),
			"position_id":      in.PositionID.String(),
			"org_node_id":      target.OrgNodeID.String(),
			"title":            target.Title,
			"lifecycle_status": target.LifecycleStatus,
			"capacity_fte":     target.CapacityFTE,
			"effective_date":   target.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":         target.EndDate.UTC().Format(time.RFC3339),
		}

		if in.CapacityFTE != nil && *in.CapacityFTE <= 0 {
			return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "capacity_fte must be > 0", nil)
		}
		lifecycle := in.Lifecycle
		if lifecycle != nil {
			v := strings.TrimSpace(*lifecycle)
			switch v {
			case "planned", "active", "inactive", "rescinded":
			default:
				return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "invalid lifecycle_status", nil)
			}
			lifecycle = &v
		}

		affectedOrgNodeID := target.OrgNodeID
		if in.OrgNodeID != nil && *in.OrgNodeID != uuid.Nil {
			affectedOrgNodeID = *in.OrgNodeID
		}
		hierarchyType := "OrgUnit"
		nodeExists, err := s.repo.NodeExistsAt(txCtx, tenantID, affectedOrgNodeID, hierarchyType, target.EffectiveDate)
		if err != nil {
			return nil, err
		}
		if !nodeExists {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_NODE_NOT_FOUND_AT_DATE", "org_node_id not found at effective_date", nil)
		}

		if in.CapacityFTE != nil {
			occupied, err := s.repo.SumAllocatedFTEAt(txCtx, tenantID, in.PositionID, target.EffectiveDate)
			if err != nil {
				return nil, err
			}
			if occupied > *in.CapacityFTE {
				return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_POSITION_OVER_CAPACITY", "position capacity would be below occupied_fte", nil)
			}
		}

		patch := PositionSliceInPlacePatch{
			OrgNodeID:           in.OrgNodeID,
			Title:               in.Title,
			LifecycleStatus:     lifecycle,
			CapacityFTE:         in.CapacityFTE,
			ReportsToPositionID: in.ReportsToID,
		}
		if err := s.repo.UpdatePositionSliceInPlace(txCtx, tenantID, target.ID, patch); err != nil {
			return nil, mapPgError(err)
		}

		updated, err := s.repo.GetPositionSliceAt(txCtx, tenantID, in.PositionID, in.AsOf)
		if err != nil {
			return nil, mapPgError(err)
		}

		newValues := map[string]any{
			"slice_id":         updated.ID.String(),
			"position_id":      in.PositionID.String(),
			"org_node_id":      updated.OrgNodeID.String(),
			"title":            updated.Title,
			"lifecycle_status": updated.LifecycleStatus,
			"capacity_fte":     updated.CapacityFTE,
			"effective_date":   updated.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":         updated.EndDate.UTC().Format(time.RFC3339),
		}

		_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
			RequestID:       requestID,
			TransactionTime: txTime,
			InitiatorID:     initiatorID,
			ChangeType:      "position.corrected",
			EntityType:      "org_position",
			EntityID:        in.PositionID,
			EffectiveDate:   updated.EffectiveDate,
			EndDate:         updated.EndDate,
			OldValues:       oldValues,
			NewValues:       newValues,
			Meta: map[string]any{
				"reason_code": reasonCode,
				"reason_note": in.ReasonNote,
			},
			Operation:       "Correct",
			FreezeMode:      freeze.Mode,
			FreezeViolation: freeze.Violation,
			FreezeCutoffUTC: freeze.CutoffUTC,
			AffectedAtUTC:   updated.EffectiveDate,
		})
		if err != nil {
			return nil, err
		}

		view, err := s.repo.GetPositionAsOf(txCtx, tenantID, in.PositionID, updated.EffectiveDate)
		if err != nil {
			return nil, mapPgError(err)
		}

		ev := buildEventV1(requestID, tenantID, initiatorID, txTime, "position.corrected", "org_position", in.PositionID, updated.EffectiveDate, updated.EndDate)
		if payload, err := json.Marshal(map[string]any{
			"position_id":      in.PositionID.String(),
			"code":             view.Code,
			"org_node_id":      view.OrgNodeID.String(),
			"lifecycle_status": view.LifecycleStatus,
			"is_auto_created":  view.IsAutoCreated,
			"capacity_fte":     view.CapacityFTE,
			"effective_date":   updated.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":         updated.EndDate.UTC().Format(time.RFC3339),
		}); err == nil {
			ev.NewValues = payload
		}

		res := &CorrectPositionResult{
			PositionID:    in.PositionID,
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
		s.InvalidateTenantCacheWithReason(tenantID, "write_commit")
	}
	return written, nil
}

type RescindPositionInput struct {
	PositionID    uuid.UUID
	EffectiveDate time.Time
	ReasonCode    string
	ReasonNote    *string
}

type RescindPositionResult struct {
	PositionID      uuid.UUID
	EffectiveDate   time.Time
	EndDate         time.Time
	GeneratedEvents []events.OrgEventV1
}

func (s *OrgService) RescindPosition(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in RescindPositionInput) (*RescindPositionResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	txTime := time.Now().UTC()
	if in.PositionID == uuid.Nil || in.EffectiveDate.IsZero() {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "position_id/effective_date are required", nil)
	}
	reasonCode := strings.TrimSpace(in.ReasonCode)
	if reasonCode == "" {
		reasonCode = "legacy"
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*RescindPositionResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}
		freeze, err := s.freezeCheck(settings, txTime, in.EffectiveDate)
		if err != nil {
			return nil, err
		}

		target, err := s.repo.LockPositionSliceAt(txCtx, tenantID, in.PositionID, in.EffectiveDate)
		if err != nil {
			return nil, mapPgError(err)
		}

		occupied, err := s.repo.SumAllocatedFTEAt(txCtx, tenantID, in.PositionID, in.EffectiveDate)
		if err != nil {
			return nil, err
		}
		if occupied > 0 {
			return nil, newServiceError(http.StatusConflict, "ORG_POSITION_NOT_EMPTY", "position has occupied_fte at effective_date", nil)
		}

		hasSubs, err := s.repo.HasPositionSubordinatesAt(txCtx, tenantID, in.PositionID, in.EffectiveDate)
		if err != nil {
			return nil, err
		}
		if hasSubs {
			return nil, newServiceError(http.StatusConflict, "ORG_POSITION_HAS_SUBORDINATES", "position has subordinates at effective_date", nil)
		}

		oldValues := map[string]any{
			"slice_id":         target.ID.String(),
			"position_id":      in.PositionID.String(),
			"org_node_id":      target.OrgNodeID.String(),
			"lifecycle_status": target.LifecycleStatus,
			"capacity_fte":     target.CapacityFTE,
			"effective_date":   target.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":         target.EndDate.UTC().Format(time.RFC3339),
		}

		if err := s.repo.DeletePositionSlicesFrom(txCtx, tenantID, in.PositionID, in.EffectiveDate); err != nil {
			return nil, err
		}
		if target.EffectiveDate.Before(in.EffectiveDate) {
			if err := s.repo.TruncatePositionSlice(txCtx, tenantID, target.ID, in.EffectiveDate); err != nil {
				return nil, mapPgError(err)
			}
		}

		sliceID, err := s.repo.InsertPositionSlice(txCtx, tenantID, in.PositionID, PositionSliceInsert{
			OrgNodeID:           target.OrgNodeID,
			Title:               target.Title,
			LifecycleStatus:     "rescinded",
			CapacityFTE:         target.CapacityFTE,
			ReportsToPositionID: target.ReportsToPositionID,
			EffectiveDate:       in.EffectiveDate,
			EndDate:             endOfTime,
		})
		if err != nil {
			return nil, mapPgError(err)
		}

		newValues := map[string]any{
			"slice_id":         sliceID.String(),
			"position_id":      in.PositionID.String(),
			"org_node_id":      target.OrgNodeID.String(),
			"lifecycle_status": "rescinded",
			"capacity_fte":     target.CapacityFTE,
			"effective_date":   in.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":         endOfTime.UTC().Format(time.RFC3339),
		}

		_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
			RequestID:       requestID,
			TransactionTime: txTime,
			InitiatorID:     initiatorID,
			ChangeType:      "position.rescinded",
			EntityType:      "org_position",
			EntityID:        in.PositionID,
			EffectiveDate:   in.EffectiveDate,
			EndDate:         endOfTime,
			OldValues:       oldValues,
			NewValues:       newValues,
			Meta: map[string]any{
				"reason_code": reasonCode,
				"reason_note": in.ReasonNote,
			},
			Operation:       "Rescind",
			FreezeMode:      freeze.Mode,
			FreezeViolation: freeze.Violation,
			FreezeCutoffUTC: freeze.CutoffUTC,
			AffectedAtUTC:   in.EffectiveDate,
		})
		if err != nil {
			return nil, err
		}

		view, err := s.repo.GetPositionAsOf(txCtx, tenantID, in.PositionID, in.EffectiveDate)
		if err != nil {
			return nil, mapPgError(err)
		}

		ev := buildEventV1(requestID, tenantID, initiatorID, txTime, "position.rescinded", "org_position", in.PositionID, in.EffectiveDate, endOfTime)
		if payload, err := json.Marshal(map[string]any{
			"position_id":      in.PositionID.String(),
			"code":             view.Code,
			"org_node_id":      view.OrgNodeID.String(),
			"lifecycle_status": "rescinded",
			"is_auto_created":  view.IsAutoCreated,
			"capacity_fte":     view.CapacityFTE,
			"effective_date":   in.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":         endOfTime.UTC().Format(time.RFC3339),
		}); err == nil {
			ev.NewValues = payload
		}

		res := &RescindPositionResult{
			PositionID:    in.PositionID,
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
