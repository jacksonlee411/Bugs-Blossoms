package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sirupsen/logrus"

	"github.com/iota-uz/iota-sdk/modules/org/domain/events"
)

const maxReportsToDepth = 128

type CreatePositionInput struct {
	Code               string
	OrgNodeID          uuid.UUID
	EffectiveDate      time.Time
	Title              *string
	LifecycleStatus    string
	PositionType       string
	EmploymentType     string
	CapacityFTE        float64
	ReportsToID        *uuid.UUID
	JobFamilyGroupCode string
	JobFamilyCode      string
	JobRoleCode        string
	JobLevelCode       string
	JobProfileID       *uuid.UUID
	CostCenterCode     *string
	Profile            json.RawMessage
	ReasonCode         string
	ReasonNote         *string
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

	positionType := strings.TrimSpace(in.PositionType)
	employmentType := strings.TrimSpace(in.EmploymentType)
	jobFamilyGroupCode := strings.TrimSpace(in.JobFamilyGroupCode)
	jobFamilyCode := strings.TrimSpace(in.JobFamilyCode)
	jobRoleCode := strings.TrimSpace(in.JobRoleCode)
	jobLevelCode := strings.TrimSpace(in.JobLevelCode)
	if positionType == "" || employmentType == "" || jobFamilyGroupCode == "" || jobFamilyCode == "" || jobRoleCode == "" || jobLevelCode == "" {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "position_type/employment_type/job_*_code are required for managed positions", nil)
	}

	jobProfileID := in.JobProfileID
	if jobProfileID != nil && *jobProfileID == uuid.Nil {
		jobProfileID = nil
	}

	costCenterCode := trimOptionalText(in.CostCenterCode)

	profile, err := normalizeJSONObject(in.Profile)
	if err != nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "profile must be a JSON object", nil)
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

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*CreatePositionResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}
		reasonCode, reasonInfo, svcErr := normalizeReasonCode(settings, in.ReasonCode)
		if svcErr != nil {
			logReasonCodeRejected(txCtx, tenantID, requestID, "position.created", reasonInfo, svcErr)
			return nil, svcErr
		}
		freeze, err := s.freezeCheck(settings, txTime, in.EffectiveDate)
		if err != nil {
			maybeLogFrozenWindowRejected(txCtx, tenantID, requestID, initiatorID, "position.created", "org_position", uuid.Nil, in.EffectiveDate, freeze, err, logrus.Fields{
				"org_node_id": in.OrgNodeID.String(),
				"operation":   "Create",
			})
			return nil, err
		}
		catalogMode := normalizeValidationMode(settings.PositionCatalogValidationMode)

		hierarchyType := "OrgUnit"
		exists, err := s.repo.NodeExistsAt(txCtx, tenantID, in.OrgNodeID, hierarchyType, in.EffectiveDate)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_NODE_NOT_FOUND_AT_DATE", "org_node_id not found at effective_date", nil)
		}

		var catalogShadowErr *ServiceError
		if catalogMode != "disabled" {
			_, err := s.validatePositionCatalogAndProfile(txCtx, tenantID, JobCatalogCodes{
				JobFamilyGroupCode: jobFamilyGroupCode,
				JobFamilyCode:      jobFamilyCode,
				JobRoleCode:        jobRoleCode,
				JobLevelCode:       jobLevelCode,
			}, jobProfileID)
			if err != nil {
				if catalogMode == "enforce" {
					maybeLogModeRejected(txCtx, "org.position_catalog.rejected", tenantID, requestID, initiatorID, "position.created", "org_position", uuid.Nil, in.EffectiveDate, catalogMode, err, logrus.Fields{
						"org_node_id":           in.OrgNodeID.String(),
						"job_family_group_code": jobFamilyGroupCode,
						"job_family_code":       jobFamilyCode,
						"job_role_code":         jobRoleCode,
						"job_level_code":        jobLevelCode,
						"job_profile_id":        jobProfileID,
						"operation":             "Create",
					})
					return nil, err
				}
				var svcErr *ServiceError
				if !errors.As(err, &svcErr) {
					return nil, err
				}
				catalogShadowErr = svcErr
			}
		}

		restrictionsJSON, err := extractRestrictionsFromProfile(profile)
		if err != nil {
			return nil, err
		}
		parsedRestrictions, err := parsePositionRestrictions(restrictionsJSON)
		if err != nil {
			return nil, err
		}
		if err := s.validatePositionRestrictionsAgainstSlice(txCtx, tenantID, false, PositionSliceRow{
			JobFamilyGroupCode: &jobFamilyGroupCode,
			JobFamilyCode:      &jobFamilyCode,
			JobRoleCode:        &jobRoleCode,
			JobLevelCode:       &jobLevelCode,
			JobProfileID:       jobProfileID,
		}, parsedRestrictions); err != nil {
			return nil, err
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

		if err := s.validateReportsToNoCycle(txCtx, tenantID, positionID, in.EffectiveDate, in.ReportsToID); err != nil {
			return nil, err
		}

		sliceID, err := s.repo.InsertPositionSlice(txCtx, tenantID, positionID, PositionSliceInsert{
			OrgNodeID:           in.OrgNodeID,
			Title:               in.Title,
			LifecycleStatus:     lifecycle,
			PositionType:        &positionType,
			EmploymentType:      &employmentType,
			CapacityFTE:         in.CapacityFTE,
			ReportsToPositionID: in.ReportsToID,
			JobFamilyGroupCode:  &jobFamilyGroupCode,
			JobFamilyCode:       &jobFamilyCode,
			JobRoleCode:         &jobRoleCode,
			JobLevelCode:        &jobLevelCode,
			JobProfileID:        jobProfileID,
			CostCenterCode:      costCenterCode,
			Profile:             profile,
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

		meta := map[string]any{
			"reason_code": reasonCode,
			"reason_note": in.ReasonNote,
		}
		addReasonCodeMeta(meta, reasonInfo)
		if catalogShadowErr != nil {
			meta["position_catalog_validation_mode"] = catalogMode
			meta["position_catalog_validation_error_code"] = catalogShadowErr.Code
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
			Meta:            meta,
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

func (s *OrgService) validateReportsToNoCycle(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, asOf time.Time, reportsToID *uuid.UUID) error {
	if reportsToID == nil || *reportsToID == uuid.Nil {
		return nil
	}

	visited := make(map[uuid.UUID]struct{}, 8)
	current := *reportsToID
	for i := 0; i < maxReportsToDepth; i++ {
		if current == uuid.Nil {
			return nil
		}
		if current == positionID {
			return newServiceError(http.StatusUnprocessableEntity, "ORG_POSITION_REPORTS_TO_CYCLE", "reports_to creates a cycle", nil)
		}
		if _, ok := visited[current]; ok {
			return newServiceError(http.StatusUnprocessableEntity, "ORG_POSITION_REPORTS_TO_CYCLE", "reports_to creates a cycle", nil)
		}
		visited[current] = struct{}{}

		row, err := s.repo.GetPositionSliceAt(ctx, tenantID, current, asOf)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return newServiceError(http.StatusUnprocessableEntity, "ORG_POSITION_NOT_FOUND_AT_DATE", "reports_to_position_id not found at effective_date", err)
			}
			return mapPgError(err)
		}
		if row.ReportsToPositionID == nil || *row.ReportsToPositionID == uuid.Nil {
			return nil
		}
		current = *row.ReportsToPositionID
	}
	return newServiceError(http.StatusUnprocessableEntity, "ORG_POSITION_REPORTS_TO_CYCLE", "reports_to creates a cycle", nil)
}

type ShiftBoundaryPositionInput struct {
	PositionID          uuid.UUID
	TargetEffectiveDate time.Time
	NewEffectiveDate    time.Time
	ReasonCode          string
	ReasonNote          *string
}

type ShiftBoundaryPositionResult struct {
	PositionID      uuid.UUID
	TargetStart     time.Time
	NewStart        time.Time
	GeneratedEvents []events.OrgEventV1
}

func (s *OrgService) ShiftBoundaryPosition(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in ShiftBoundaryPositionInput) (*ShiftBoundaryPositionResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	txTime := time.Now().UTC()
	if in.PositionID == uuid.Nil || in.TargetEffectiveDate.IsZero() || in.NewEffectiveDate.IsZero() {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "position_id/target_effective_date/new_effective_date are required", nil)
	}
	if !in.NewEffectiveDate.Before(endOfTime) {
		return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_SHIFTBOUNDARY_INVERTED", "new_effective_date is invalid", nil)
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*ShiftBoundaryPositionResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}
		reasonCode, reasonInfo, svcErr := normalizeReasonCode(settings, in.ReasonCode)
		if svcErr != nil {
			logReasonCodeRejected(txCtx, tenantID, requestID, "position.shifted", reasonInfo, svcErr)
			return nil, svcErr
		}

		target, err := s.repo.LockPositionSliceStartingAt(txCtx, tenantID, in.PositionID, in.TargetEffectiveDate)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_POSITION_NOT_FOUND_AT_DATE", "target slice not found", err)
			}
			return nil, mapPgError(err)
		}
		if in.NewEffectiveDate.After(target.EndDate) {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_SHIFTBOUNDARY_INVERTED", "new_effective_date must be on/before target end_date", nil)
		}

		prev, err := s.repo.LockPositionSliceEndingAt(txCtx, tenantID, in.PositionID, truncateEndDateFromNewEffectiveDate(in.TargetEffectiveDate))
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
			maybeLogFrozenWindowRejected(txCtx, tenantID, requestID, initiatorID, "position.shifted", "org_position", in.PositionID, affectedAt, freeze, err, logrus.Fields{
				"position_id": in.PositionID.String(),
				"operation":   "ShiftBoundary",
			})
			return nil, err
		}

		prevOld := map[string]any{
			"slice_id":       prev.ID.String(),
			"position_id":    in.PositionID.String(),
			"effective_date": prev.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":       prev.EndDate.UTC().Format(time.RFC3339),
		}
		targetOld := map[string]any{
			"slice_id":       target.ID.String(),
			"position_id":    in.PositionID.String(),
			"effective_date": target.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":       target.EndDate.UTC().Format(time.RFC3339),
		}

		if in.NewEffectiveDate.After(in.TargetEffectiveDate) {
			if err := s.repo.UpdatePositionSliceEffectiveDate(txCtx, tenantID, target.ID, in.NewEffectiveDate); err != nil {
				return nil, mapPgError(err)
			}
			if err := s.repo.UpdatePositionSliceEndDate(txCtx, tenantID, prev.ID, truncateEndDateFromNewEffectiveDate(in.NewEffectiveDate)); err != nil {
				return nil, mapPgError(err)
			}
		} else {
			if err := s.repo.UpdatePositionSliceEndDate(txCtx, tenantID, prev.ID, truncateEndDateFromNewEffectiveDate(in.NewEffectiveDate)); err != nil {
				return nil, mapPgError(err)
			}
			if err := s.repo.UpdatePositionSliceEffectiveDate(txCtx, tenantID, target.ID, in.NewEffectiveDate); err != nil {
				return nil, mapPgError(err)
			}
		}

		prevNew := map[string]any{
			"slice_id":       prev.ID.String(),
			"position_id":    in.PositionID.String(),
			"effective_date": prev.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":       truncateEndDateFromNewEffectiveDate(in.NewEffectiveDate).UTC().Format(time.RFC3339),
		}
		targetNew := map[string]any{
			"slice_id":       target.ID.String(),
			"position_id":    in.PositionID.String(),
			"effective_date": in.NewEffectiveDate.UTC().Format(time.RFC3339),
			"end_date":       target.EndDate.UTC().Format(time.RFC3339),
		}

		opDetails := map[string]any{
			"target_effective_date": in.TargetEffectiveDate.UTC().Format(time.RFC3339),
			"new_effective_date":    in.NewEffectiveDate.UTC().Format(time.RFC3339),
		}
		meta := map[string]any{
			"reason_code": reasonCode,
			"reason_note": in.ReasonNote,
		}
		addReasonCodeMeta(meta, reasonInfo)

		_, err = s.repo.InsertAuditLog(txCtx, tenantID, AuditLogInsert{
			RequestID:        requestID,
			TransactionTime:  txTime,
			InitiatorID:      initiatorID,
			ChangeType:       "position.corrected",
			EntityType:       "org_position",
			EntityID:         in.PositionID,
			EffectiveDate:    prev.EffectiveDate,
			EndDate:          in.NewEffectiveDate,
			OldValues:        prevOld,
			NewValues:        prevNew,
			Meta:             meta,
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
			ChangeType:       "position.corrected",
			EntityType:       "org_position",
			EntityID:         in.PositionID,
			EffectiveDate:    in.NewEffectiveDate,
			EndDate:          target.EndDate,
			OldValues:        targetOld,
			NewValues:        targetNew,
			Meta:             meta,
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

		view, err := s.repo.GetPositionAsOf(txCtx, tenantID, in.PositionID, in.NewEffectiveDate)
		if err != nil {
			return nil, mapPgError(err)
		}

		ev := buildEventV1(requestID, tenantID, initiatorID, txTime, "position.corrected", "org_position", in.PositionID, in.NewEffectiveDate, endOfTime)
		if payload, err := json.Marshal(map[string]any{
			"position_id":      in.PositionID.String(),
			"code":             view.Code,
			"org_node_id":      view.OrgNodeID.String(),
			"lifecycle_status": view.LifecycleStatus,
			"is_auto_created":  view.IsAutoCreated,
			"capacity_fte":     view.CapacityFTE,
			"effective_date":   view.EffectiveDate.UTC().Format(time.RFC3339),
			"end_date":         view.EndDate.UTC().Format(time.RFC3339),
		}); err == nil {
			ev.NewValues = payload
		}

		res := &ShiftBoundaryPositionResult{
			PositionID:  in.PositionID,
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
		s.InvalidateTenantCacheWithReason(tenantID, "write_commit")
	}
	return written, nil
}

type GetPositionsInput struct {
	AsOf            *time.Time
	OrgNodeID       *uuid.UUID
	OrgNodeIDs      []uuid.UUID
	Q               *string
	LifecycleStatus *string
	StaffingState   *string
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
			OrgNodeIDs:      in.OrgNodeIDs,
			Q:               in.Q,
			LifecycleStatus: in.LifecycleStatus,
			StaffingState:   in.StaffingState,
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

	OrgNodeID          *uuid.UUID
	Title              *string
	LifecycleStatus    *string
	PositionType       *string
	EmploymentType     *string
	CapacityFTE        *float64
	ReportsToID        *uuid.UUID
	JobFamilyGroupCode *string
	JobFamilyCode      *string
	JobRoleCode        *string
	JobLevelCode       *string
	JobProfileID       *uuid.UUID
	CostCenterCode     *string
	Profile            *json.RawMessage
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

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*UpdatePositionResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}
		reasonCode, reasonInfo, svcErr := normalizeReasonCode(settings, in.ReasonCode)
		if svcErr != nil {
			logReasonCodeRejected(txCtx, tenantID, requestID, "position.updated", reasonInfo, svcErr)
			return nil, svcErr
		}
		freeze, err := s.freezeCheck(settings, txTime, in.EffectiveDate)
		if err != nil {
			maybeLogFrozenWindowRejected(txCtx, tenantID, requestID, initiatorID, "position.updated", "org_position", in.PositionID, in.EffectiveDate, freeze, err, logrus.Fields{
				"position_id": in.PositionID.String(),
				"operation":   "Update",
			})
			return nil, err
		}
		catalogMode := normalizeValidationMode(settings.PositionCatalogValidationMode)

		current, err := s.repo.LockPositionSliceAt(txCtx, tenantID, in.PositionID, in.EffectiveDate)
		if err != nil {
			return nil, mapPgError(err)
		}
		isAutoCreated, err := s.repo.GetPositionIsAutoCreated(txCtx, tenantID, in.PositionID)
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

		positionType := current.PositionType
		if in.PositionType != nil {
			v := strings.TrimSpace(*in.PositionType)
			if v == "" {
				return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "position_type is invalid", nil)
			}
			positionType = &v
		}
		employmentType := current.EmploymentType
		if in.EmploymentType != nil {
			v := strings.TrimSpace(*in.EmploymentType)
			if v == "" {
				return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "employment_type is invalid", nil)
			}
			employmentType = &v
		}
		jobFamilyGroupCode := current.JobFamilyGroupCode
		if in.JobFamilyGroupCode != nil {
			v := strings.TrimSpace(*in.JobFamilyGroupCode)
			if v == "" {
				return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "job_family_group_code is invalid", nil)
			}
			jobFamilyGroupCode = &v
		}
		jobFamilyCode := current.JobFamilyCode
		if in.JobFamilyCode != nil {
			v := strings.TrimSpace(*in.JobFamilyCode)
			if v == "" {
				return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "job_family_code is invalid", nil)
			}
			jobFamilyCode = &v
		}
		jobRoleCode := current.JobRoleCode
		if in.JobRoleCode != nil {
			v := strings.TrimSpace(*in.JobRoleCode)
			if v == "" {
				return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "job_role_code is invalid", nil)
			}
			jobRoleCode = &v
		}
		jobLevelCode := current.JobLevelCode
		if in.JobLevelCode != nil {
			v := strings.TrimSpace(*in.JobLevelCode)
			if v == "" {
				return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "job_level_code is invalid", nil)
			}
			jobLevelCode = &v
		}
		jobProfileID := current.JobProfileID
		if in.JobProfileID != nil {
			if *in.JobProfileID == uuid.Nil {
				jobProfileID = nil
			} else {
				jobProfileID = in.JobProfileID
			}
		}
		costCenterCode := current.CostCenterCode
		if in.CostCenterCode != nil {
			costCenterCode = trimOptionalText(in.CostCenterCode)
		}
		profile := current.Profile
		if in.Profile != nil {
			normalized, err := normalizeJSONObject(*in.Profile)
			if err != nil {
				return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "profile must be a JSON object", nil)
			}
			profile = normalized
		}

		if !isAutoCreated {
			if positionType == nil || strings.TrimSpace(*positionType) == "" ||
				employmentType == nil || strings.TrimSpace(*employmentType) == "" ||
				jobFamilyGroupCode == nil || strings.TrimSpace(*jobFamilyGroupCode) == "" ||
				jobFamilyCode == nil || strings.TrimSpace(*jobFamilyCode) == "" ||
				jobRoleCode == nil || strings.TrimSpace(*jobRoleCode) == "" ||
				jobLevelCode == nil || strings.TrimSpace(*jobLevelCode) == "" {
				return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "position_type/employment_type/job_*_code are required for managed positions", nil)
			}
		}

		var catalogShadowErr *ServiceError
		if !isAutoCreated && catalogMode != "disabled" {
			_, err := s.validatePositionCatalogAndProfile(txCtx, tenantID, JobCatalogCodes{
				JobFamilyGroupCode: derefString(jobFamilyGroupCode),
				JobFamilyCode:      derefString(jobFamilyCode),
				JobRoleCode:        derefString(jobRoleCode),
				JobLevelCode:       derefString(jobLevelCode),
			}, jobProfileID)
			if err != nil {
				if catalogMode == "enforce" {
					maybeLogModeRejected(txCtx, "org.position_catalog.rejected", tenantID, requestID, initiatorID, "position.updated", "org_position", current.ID, in.EffectiveDate, catalogMode, err, logrus.Fields{
						"position_id":           current.ID.String(),
						"is_auto_created":       isAutoCreated,
						"job_family_group_code": derefString(jobFamilyGroupCode),
						"job_family_code":       derefString(jobFamilyCode),
						"job_role_code":         derefString(jobRoleCode),
						"job_level_code":        derefString(jobLevelCode),
						"job_profile_id":        jobProfileID,
						"operation":             "Update",
					})
					return nil, err
				}
				var svcErr *ServiceError
				if !errors.As(err, &svcErr) {
					return nil, err
				}
				catalogShadowErr = svcErr
			}
		}

		restrictionsJSON, err := extractRestrictionsFromProfile(profile)
		if err != nil {
			return nil, err
		}
		parsedRestrictions, err := parsePositionRestrictions(restrictionsJSON)
		if err != nil {
			return nil, err
		}
		if err := s.validatePositionRestrictionsAgainstSlice(txCtx, tenantID, isAutoCreated, PositionSliceRow{
			JobFamilyGroupCode: jobFamilyGroupCode,
			JobFamilyCode:      jobFamilyCode,
			JobRoleCode:        jobRoleCode,
			JobLevelCode:       jobLevelCode,
			JobProfileID:       jobProfileID,
		}, parsedRestrictions); err != nil {
			return nil, err
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

		if err := s.validateReportsToNoCycle(txCtx, tenantID, in.PositionID, in.EffectiveDate, reportsTo); err != nil {
			return nil, err
		}

		if err := s.repo.TruncatePositionSlice(txCtx, tenantID, current.ID, truncateEndDateFromNewEffectiveDate(in.EffectiveDate)); err != nil {
			return nil, mapPgError(err)
		}

		sliceID, err := s.repo.InsertPositionSlice(txCtx, tenantID, in.PositionID, PositionSliceInsert{
			OrgNodeID:           orgNodeID,
			Title:               title,
			LifecycleStatus:     lifecycle,
			PositionType:        positionType,
			EmploymentType:      employmentType,
			CapacityFTE:         capacity,
			ReportsToPositionID: reportsTo,
			JobFamilyGroupCode:  jobFamilyGroupCode,
			JobFamilyCode:       jobFamilyCode,
			JobRoleCode:         jobRoleCode,
			JobLevelCode:        jobLevelCode,
			JobProfileID:        jobProfileID,
			CostCenterCode:      costCenterCode,
			Profile:             profile,
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
			Meta: func() map[string]any {
				meta := map[string]any{
					"reason_code": reasonCode,
					"reason_note": in.ReasonNote,
				}
				addReasonCodeMeta(meta, reasonInfo)
				if catalogShadowErr != nil {
					meta["position_catalog_validation_mode"] = catalogMode
					meta["position_catalog_validation_error_code"] = catalogShadowErr.Code
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
	PositionID         uuid.UUID
	AsOf               time.Time
	ReasonCode         string
	ReasonNote         *string
	OrgNodeID          *uuid.UUID
	Title              *string
	Lifecycle          *string
	PositionType       *string
	EmploymentType     *string
	CapacityFTE        *float64
	ReportsToID        *uuid.UUID
	JobFamilyGroupCode *string
	JobFamilyCode      *string
	JobRoleCode        *string
	JobLevelCode       *string
	JobProfileID       *uuid.UUID
	CostCenterCode     *string
	Profile            *json.RawMessage
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

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*CorrectPositionResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}
		reasonCode, reasonInfo, svcErr := normalizeReasonCode(settings, in.ReasonCode)
		if svcErr != nil {
			logReasonCodeRejected(txCtx, tenantID, requestID, "position.corrected", reasonInfo, svcErr)
			return nil, svcErr
		}
		catalogMode := normalizeValidationMode(settings.PositionCatalogValidationMode)

		target, err := s.repo.LockPositionSliceAt(txCtx, tenantID, in.PositionID, in.AsOf)
		if err != nil {
			return nil, mapPgError(err)
		}
		isAutoCreated, err := s.repo.GetPositionIsAutoCreated(txCtx, tenantID, in.PositionID)
		if err != nil {
			return nil, mapPgError(err)
		}

		freeze, err := s.freezeCheck(settings, txTime, target.EffectiveDate)
		if err != nil {
			maybeLogFrozenWindowRejected(txCtx, tenantID, requestID, initiatorID, "position.corrected", "org_position", in.PositionID, target.EffectiveDate, freeze, err, logrus.Fields{
				"position_id":     in.PositionID.String(),
				"is_auto_created": isAutoCreated,
				"operation":       "Correct",
			})
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

		positionType := target.PositionType
		var positionTypePatch *string
		if in.PositionType != nil {
			v := strings.TrimSpace(*in.PositionType)
			if v == "" {
				return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "position_type is invalid", nil)
			}
			positionType = &v
			positionTypePatch = &v
		}
		employmentType := target.EmploymentType
		var employmentTypePatch *string
		if in.EmploymentType != nil {
			v := strings.TrimSpace(*in.EmploymentType)
			if v == "" {
				return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "employment_type is invalid", nil)
			}
			employmentType = &v
			employmentTypePatch = &v
		}
		jobFamilyGroupCode := target.JobFamilyGroupCode
		var jobFamilyGroupCodePatch *string
		if in.JobFamilyGroupCode != nil {
			v := strings.TrimSpace(*in.JobFamilyGroupCode)
			if v == "" {
				return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "job_family_group_code is invalid", nil)
			}
			jobFamilyGroupCode = &v
			jobFamilyGroupCodePatch = &v
		}
		jobFamilyCode := target.JobFamilyCode
		var jobFamilyCodePatch *string
		if in.JobFamilyCode != nil {
			v := strings.TrimSpace(*in.JobFamilyCode)
			if v == "" {
				return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "job_family_code is invalid", nil)
			}
			jobFamilyCode = &v
			jobFamilyCodePatch = &v
		}
		jobRoleCode := target.JobRoleCode
		var jobRoleCodePatch *string
		if in.JobRoleCode != nil {
			v := strings.TrimSpace(*in.JobRoleCode)
			if v == "" {
				return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "job_role_code is invalid", nil)
			}
			jobRoleCode = &v
			jobRoleCodePatch = &v
		}
		jobLevelCode := target.JobLevelCode
		var jobLevelCodePatch *string
		if in.JobLevelCode != nil {
			v := strings.TrimSpace(*in.JobLevelCode)
			if v == "" {
				return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "job_level_code is invalid", nil)
			}
			jobLevelCode = &v
			jobLevelCodePatch = &v
		}
		var jobProfileIDPatch *uuid.UUID
		if in.JobProfileID != nil {
			if *in.JobProfileID == uuid.Nil {
				jobProfileIDPatch = &uuid.Nil
			} else {
				jobProfileIDPatch = in.JobProfileID
			}
		}
		var costCenterCodePatch *string
		if in.CostCenterCode != nil {
			costCenterCodePatch = trimOptionalText(in.CostCenterCode)
		}
		profile := target.Profile
		var profilePatch *json.RawMessage
		if in.Profile != nil {
			normalized, err := normalizeJSONObject(*in.Profile)
			if err != nil {
				return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "profile must be a JSON object", nil)
			}
			profile = normalized
			tmp := profile
			profilePatch = &tmp
		}

		if !isAutoCreated {
			if positionType == nil || strings.TrimSpace(*positionType) == "" ||
				employmentType == nil || strings.TrimSpace(*employmentType) == "" ||
				jobFamilyGroupCode == nil || strings.TrimSpace(*jobFamilyGroupCode) == "" ||
				jobFamilyCode == nil || strings.TrimSpace(*jobFamilyCode) == "" ||
				jobRoleCode == nil || strings.TrimSpace(*jobRoleCode) == "" ||
				jobLevelCode == nil || strings.TrimSpace(*jobLevelCode) == "" {
				return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "position_type/employment_type/job_*_code are required for managed positions", nil)
			}
		}

		jobProfileID := target.JobProfileID
		if in.JobProfileID != nil {
			if *in.JobProfileID == uuid.Nil {
				jobProfileID = nil
			} else {
				jobProfileID = in.JobProfileID
			}
		}

		var catalogShadowErr *ServiceError
		if !isAutoCreated && catalogMode != "disabled" {
			_, err := s.validatePositionCatalogAndProfile(txCtx, tenantID, JobCatalogCodes{
				JobFamilyGroupCode: derefString(jobFamilyGroupCode),
				JobFamilyCode:      derefString(jobFamilyCode),
				JobRoleCode:        derefString(jobRoleCode),
				JobLevelCode:       derefString(jobLevelCode),
			}, jobProfileID)
			if err != nil {
				if catalogMode == "enforce" {
					maybeLogModeRejected(txCtx, "org.position_catalog.rejected", tenantID, requestID, initiatorID, "position.corrected", "org_position", in.PositionID, target.EffectiveDate, catalogMode, err, logrus.Fields{
						"position_id":           in.PositionID.String(),
						"is_auto_created":       isAutoCreated,
						"job_family_group_code": derefString(jobFamilyGroupCode),
						"job_family_code":       derefString(jobFamilyCode),
						"job_role_code":         derefString(jobRoleCode),
						"job_level_code":        derefString(jobLevelCode),
						"job_profile_id":        jobProfileID,
						"operation":             "Correct",
					})
					return nil, err
				}
				var svcErr *ServiceError
				if !errors.As(err, &svcErr) {
					return nil, err
				}
				catalogShadowErr = svcErr
			}
		}

		restrictionsJSON, err := extractRestrictionsFromProfile(profile)
		if err != nil {
			return nil, err
		}
		parsedRestrictions, err := parsePositionRestrictions(restrictionsJSON)
		if err != nil {
			return nil, err
		}
		if err := s.validatePositionRestrictionsAgainstSlice(txCtx, tenantID, isAutoCreated, PositionSliceRow{
			JobFamilyGroupCode: jobFamilyGroupCode,
			JobFamilyCode:      jobFamilyCode,
			JobRoleCode:        jobRoleCode,
			JobLevelCode:       jobLevelCode,
			JobProfileID:       jobProfileID,
		}, parsedRestrictions); err != nil {
			return nil, err
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

		reportsTo := target.ReportsToPositionID
		if in.ReportsToID != nil {
			reportsTo = in.ReportsToID
		}
		if err := s.validateReportsToNoCycle(txCtx, tenantID, in.PositionID, target.EffectiveDate, reportsTo); err != nil {
			return nil, err
		}

		patch := PositionSliceInPlacePatch{
			OrgNodeID:           in.OrgNodeID,
			Title:               in.Title,
			LifecycleStatus:     lifecycle,
			PositionType:        positionTypePatch,
			EmploymentType:      employmentTypePatch,
			CapacityFTE:         in.CapacityFTE,
			ReportsToPositionID: in.ReportsToID,
			JobFamilyGroupCode:  jobFamilyGroupCodePatch,
			JobFamilyCode:       jobFamilyCodePatch,
			JobRoleCode:         jobRoleCodePatch,
			JobLevelCode:        jobLevelCodePatch,
			JobProfileID:        jobProfileIDPatch,
			CostCenterCode:      costCenterCodePatch,
			Profile:             profilePatch,
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
			Meta: func() map[string]any {
				meta := map[string]any{
					"reason_code": reasonCode,
					"reason_note": in.ReasonNote,
				}
				addReasonCodeMeta(meta, reasonInfo)
				if catalogShadowErr != nil {
					meta["position_catalog_validation_mode"] = catalogMode
					meta["position_catalog_validation_error_code"] = catalogShadowErr.Code
				}
				return meta
			}(),
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

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*RescindPositionResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}
		reasonCode, reasonInfo, svcErr := normalizeReasonCode(settings, in.ReasonCode)
		if svcErr != nil {
			logReasonCodeRejected(txCtx, tenantID, requestID, "position.rescinded", reasonInfo, svcErr)
			return nil, svcErr
		}
		freeze, err := s.freezeCheck(settings, txTime, in.EffectiveDate)
		if err != nil {
			maybeLogFrozenWindowRejected(txCtx, tenantID, requestID, initiatorID, "position.rescinded", "org_position", in.PositionID, in.EffectiveDate, freeze, err, logrus.Fields{
				"position_id": in.PositionID.String(),
				"operation":   "Rescind",
			})
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
			if err := s.repo.TruncatePositionSlice(txCtx, tenantID, target.ID, truncateEndDateFromNewEffectiveDate(in.EffectiveDate)); err != nil {
				return nil, mapPgError(err)
			}
		}

		sliceID, err := s.repo.InsertPositionSlice(txCtx, tenantID, in.PositionID, PositionSliceInsert{
			OrgNodeID:           target.OrgNodeID,
			Title:               target.Title,
			LifecycleStatus:     "rescinded",
			PositionType:        target.PositionType,
			EmploymentType:      target.EmploymentType,
			CapacityFTE:         target.CapacityFTE,
			ReportsToPositionID: target.ReportsToPositionID,
			JobFamilyGroupCode:  target.JobFamilyGroupCode,
			JobFamilyCode:       target.JobFamilyCode,
			JobRoleCode:         target.JobRoleCode,
			JobLevelCode:        target.JobLevelCode,
			JobProfileID:        target.JobProfileID,
			CostCenterCode:      target.CostCenterCode,
			Profile:             target.Profile,
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
			Meta: func() map[string]any {
				meta := map[string]any{
					"reason_code": reasonCode,
					"reason_note": in.ReasonNote,
				}
				addReasonCodeMeta(meta, reasonInfo)
				return meta
			}(),
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

func trimOptionalText(v *string) *string {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(*v)
	if s == "" {
		return nil
	}
	return &s
}

func normalizeJSONObject(raw json.RawMessage) (json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return json.RawMessage(`{}`), nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, errors.New("json null is not allowed")
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	normalized, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}
