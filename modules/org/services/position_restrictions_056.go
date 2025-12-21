package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/org/domain/events"
)

type SetPositionRestrictionsInput struct {
	PositionID           uuid.UUID
	EffectiveDate        time.Time
	PositionRestrictions json.RawMessage
	ReasonCode           string
	ReasonNote           *string
}

type SetPositionRestrictionsResult struct {
	PositionID      uuid.UUID
	SliceID         uuid.UUID
	EffectiveDate   time.Time
	EndDate         time.Time
	GeneratedEvents []events.OrgEventV1
}

func (s *OrgService) GetPositionRestrictions(ctx context.Context, tenantID uuid.UUID, positionID uuid.UUID, asOf *time.Time) (json.RawMessage, time.Time, error) {
	if tenantID == uuid.Nil {
		return nil, time.Time{}, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if positionID == uuid.Nil {
		return nil, time.Time{}, newServiceError(http.StatusBadRequest, "ORG_INVALID_QUERY", "position_id is required", nil)
	}
	t := time.Now().UTC()
	if asOf != nil && !asOf.IsZero() {
		t = (*asOf).UTC()
	}

	restrictions, err := inTx(ctx, tenantID, func(txCtx context.Context) (json.RawMessage, error) {
		slice, err := s.repo.GetPositionSliceAt(txCtx, tenantID, positionID, t)
		if err != nil {
			return nil, mapPgError(err)
		}
		return extractRestrictionsFromProfile(slice.Profile)
	})
	if err != nil {
		return nil, time.Time{}, err
	}
	return restrictions, t, nil
}

func (s *OrgService) SetPositionRestrictions(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in SetPositionRestrictionsInput) (*SetPositionRestrictionsResult, error) {
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

	normalizedRestrictions, err := normalizeJSONObject(in.PositionRestrictions)
	if err != nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "position_restrictions must be a JSON object", nil)
	}
	parsed, err := parsePositionRestrictions(normalizedRestrictions)
	if err != nil {
		return nil, err
	}

	written, err := inTx(ctx, tenantID, func(txCtx context.Context) (*SetPositionRestrictionsResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}
		reasonCode, reasonInfo, svcErr := normalizeReasonCode(settings, in.ReasonCode)
		if svcErr != nil {
			logReasonCodeRejected(txCtx, tenantID, requestID, "position.restrictions.updated", reasonInfo, svcErr)
			return nil, svcErr
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

		isAutoCreated, err := s.repo.GetPositionIsAutoCreated(txCtx, tenantID, in.PositionID)
		if err != nil {
			return nil, mapPgError(err)
		}

		if err := s.validatePositionRestrictionsAgainstSlice(txCtx, tenantID, isAutoCreated, current, parsed); err != nil {
			return nil, err
		}

		nextProfile, err := mergeProfileWithRestrictions(current.Profile, normalizedRestrictions)
		if err != nil {
			return nil, err
		}

		if err := s.repo.TruncatePositionSlice(txCtx, tenantID, current.ID, in.EffectiveDate); err != nil {
			return nil, mapPgError(err)
		}
		sliceID, err := s.repo.InsertPositionSlice(txCtx, tenantID, in.PositionID, PositionSliceInsert{
			OrgNodeID:           current.OrgNodeID,
			Title:               current.Title,
			LifecycleStatus:     current.LifecycleStatus,
			PositionType:        current.PositionType,
			EmploymentType:      current.EmploymentType,
			CapacityFTE:         current.CapacityFTE,
			ReportsToPositionID: current.ReportsToPositionID,
			JobFamilyGroupCode:  current.JobFamilyGroupCode,
			JobFamilyCode:       current.JobFamilyCode,
			JobRoleCode:         current.JobRoleCode,
			JobLevelCode:        current.JobLevelCode,
			JobProfileID:        current.JobProfileID,
			CostCenterCode:      current.CostCenterCode,
			Profile:             nextProfile,
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
				"position_id":    in.PositionID.String(),
				"effective_date": current.EffectiveDate.UTC().Format(time.RFC3339),
				"end_date":       current.EndDate.UTC().Format(time.RFC3339),
			},
			NewValues: map[string]any{
				"position_id":    in.PositionID.String(),
				"effective_date": in.EffectiveDate.UTC().Format(time.RFC3339),
				"end_date":       newEnd.UTC().Format(time.RFC3339),
			},
			Meta: func() map[string]any {
				meta := map[string]any{
					"reason_code": reasonCode,
					"reason_note": in.ReasonNote,
				}
				addReasonCodeMeta(meta, reasonInfo)
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

		res := &SetPositionRestrictionsResult{
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

type positionRestrictions struct {
	AllowedJobProfileIDs []uuid.UUID
	AllowedJobRoleIDs    []uuid.UUID
	AllowedJobLevelIDs   []uuid.UUID
}

func extractRestrictionsFromProfile(profile json.RawMessage) (json.RawMessage, error) {
	if len(profile) == 0 {
		return json.RawMessage(`{}`), nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(profile, &obj); err != nil {
		return nil, newServiceError(http.StatusInternalServerError, "ORG_INTERNAL", "invalid profile json", err)
	}
	raw, ok := obj["position_restrictions"]
	if !ok || len(strings.TrimSpace(string(raw))) == 0 {
		return json.RawMessage(`{}`), nil
	}
	normalized, err := normalizeJSONObject(raw)
	if err != nil {
		return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_POSITION_RESTRICTIONS_PAYLOAD_INVALID", "position_restrictions must be a JSON object", nil)
	}
	return normalized, nil
}

func parsePositionRestrictions(raw json.RawMessage) (positionRestrictions, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return positionRestrictions{}, nil
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return positionRestrictions{}, newServiceError(http.StatusUnprocessableEntity, "ORG_POSITION_RESTRICTIONS_PAYLOAD_INVALID", "position_restrictions is invalid", err)
	}

	parseUUIDList := func(key string) ([]uuid.UUID, bool, error) {
		v, ok := obj[key]
		if !ok {
			return nil, false, nil
		}
		rawList, ok := v.([]any)
		if !ok {
			return nil, true, newServiceError(http.StatusUnprocessableEntity, "ORG_POSITION_RESTRICTIONS_PAYLOAD_INVALID", "position_restrictions is invalid", nil)
		}
		out := make([]uuid.UUID, 0, len(rawList))
		seen := make(map[uuid.UUID]struct{}, len(rawList))
		for _, item := range rawList {
			s, ok := item.(string)
			if !ok {
				return nil, true, newServiceError(http.StatusUnprocessableEntity, "ORG_POSITION_RESTRICTIONS_PAYLOAD_INVALID", "position_restrictions is invalid", nil)
			}
			id, err := uuid.Parse(s)
			if err != nil || id == uuid.Nil {
				return nil, true, newServiceError(http.StatusUnprocessableEntity, "ORG_POSITION_RESTRICTIONS_PAYLOAD_INVALID", "position_restrictions is invalid", nil)
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
		return out, true, nil
	}

	profileIDs, _, err := parseUUIDList("allowed_job_profile_ids")
	if err != nil {
		return positionRestrictions{}, err
	}
	roleIDs, _, err := parseUUIDList("allowed_job_role_ids")
	if err != nil {
		return positionRestrictions{}, err
	}
	levelIDs, _, err := parseUUIDList("allowed_job_level_ids")
	if err != nil {
		return positionRestrictions{}, err
	}
	return positionRestrictions{
		AllowedJobProfileIDs: profileIDs,
		AllowedJobRoleIDs:    roleIDs,
		AllowedJobLevelIDs:   levelIDs,
	}, nil
}

func mergeProfileWithRestrictions(profile json.RawMessage, restrictions json.RawMessage) (json.RawMessage, error) {
	base := map[string]any{}
	if len(profile) != 0 {
		if err := json.Unmarshal(profile, &base); err != nil {
			return nil, newServiceError(http.StatusInternalServerError, "ORG_INTERNAL", "invalid profile json", err)
		}
	}
	var restrictionsObj map[string]any
	if err := json.Unmarshal(restrictions, &restrictionsObj); err != nil {
		return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_POSITION_RESTRICTIONS_PAYLOAD_INVALID", "position_restrictions is invalid", err)
	}
	base["position_restrictions"] = restrictionsObj
	out, err := json.Marshal(base)
	if err != nil {
		return nil, newServiceError(http.StatusInternalServerError, "ORG_INTERNAL", "failed to build profile json", err)
	}
	return normalizeJSONObject(out)
}

func containsUUID(list []uuid.UUID, id uuid.UUID) bool {
	for _, v := range list {
		if v == id {
			return true
		}
	}
	return false
}

func (s *OrgService) validatePositionRestrictionsAgainstSlice(ctx context.Context, tenantID uuid.UUID, isAutoCreated bool, slice PositionSliceRow, r positionRestrictions) error {
	if len(r.AllowedJobProfileIDs) == 0 && len(r.AllowedJobRoleIDs) == 0 && len(r.AllowedJobLevelIDs) == 0 {
		return nil
	}
	if isAutoCreated {
		return newServiceError(http.StatusUnprocessableEntity, "ORG_POSITION_RESTRICTIONS_CATALOG_MISMATCH", "system positions cannot be restricted", nil)
	}

	if len(r.AllowedJobProfileIDs) != 0 {
		if slice.JobProfileID == nil || *slice.JobProfileID == uuid.Nil {
			return newServiceError(http.StatusUnprocessableEntity, "ORG_POSITION_RESTRICTIONS_PROFILE_MISMATCH", "job_profile_id is required", nil)
		}
		if !containsUUID(r.AllowedJobProfileIDs, *slice.JobProfileID) {
			return newServiceError(http.StatusUnprocessableEntity, "ORG_POSITION_RESTRICTIONS_PROFILE_MISMATCH", "job_profile_id is not allowed", nil)
		}
	}

	if len(r.AllowedJobRoleIDs) == 0 && len(r.AllowedJobLevelIDs) == 0 {
		return nil
	}

	if slice.JobFamilyGroupCode == nil || slice.JobFamilyCode == nil || slice.JobRoleCode == nil || slice.JobLevelCode == nil {
		return newServiceError(http.StatusUnprocessableEntity, "ORG_POSITION_RESTRICTIONS_CATALOG_MISMATCH", "job catalog codes are required", nil)
	}
	path, err := s.repo.ResolveJobCatalogPathByCodes(ctx, tenantID, JobCatalogCodes{
		JobFamilyGroupCode: *slice.JobFamilyGroupCode,
		JobFamilyCode:      *slice.JobFamilyCode,
		JobRoleCode:        *slice.JobRoleCode,
		JobLevelCode:       *slice.JobLevelCode,
	})
	if err != nil {
		if errors.Is(err, ErrJobCatalogInactiveOrMissing) || errors.Is(err, ErrJobCatalogInvalidHierarchy) {
			return newServiceError(http.StatusUnprocessableEntity, "ORG_POSITION_RESTRICTIONS_CATALOG_MISMATCH", "job catalog mismatch", nil)
		}
		return mapPgError(err)
	}

	if len(r.AllowedJobRoleIDs) != 0 && !containsUUID(r.AllowedJobRoleIDs, path.JobRoleID) {
		return newServiceError(http.StatusUnprocessableEntity, "ORG_POSITION_RESTRICTIONS_CATALOG_MISMATCH", "job role is not allowed", nil)
	}
	if len(r.AllowedJobLevelIDs) != 0 && !containsUUID(r.AllowedJobLevelIDs, path.JobLevelID) {
		return newServiceError(http.StatusUnprocessableEntity, "ORG_POSITION_RESTRICTIONS_CATALOG_MISMATCH", "job level is not allowed", nil)
	}
	return nil
}
