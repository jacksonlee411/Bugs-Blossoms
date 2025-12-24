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

type TransitionAssignmentInput struct {
	AssignmentID  uuid.UUID
	EventType     string
	EffectiveDate time.Time
	OrgNodeID     *uuid.UUID
	PositionID    *uuid.UUID
	ReasonCode    string
	ReasonNote    *string
}

type TransitionAssignmentResult struct {
	Event   PersonnelEventRow
	Created bool
}

func (s *OrgService) TransitionAssignment(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in TransitionAssignmentInput) (*TransitionAssignmentResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	txTime := time.Now().UTC()
	if in.AssignmentID == uuid.Nil || in.EffectiveDate.IsZero() {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "assignment_id/effective_date are required", nil)
	}
	eventType := strings.TrimSpace(in.EventType)
	switch eventType {
	case "transfer", "termination":
	default:
		return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_INVALID_BODY", "event_type must be transfer or termination", nil)
	}

	return inTx(ctx, tenantID, func(txCtx context.Context) (*TransitionAssignmentResult, error) {
		settings, err := s.repo.GetOrgSettings(txCtx, tenantID)
		if err != nil {
			return nil, err
		}

		reasonCode, reasonInfo, svcErr := normalizeReasonCode(settings, in.ReasonCode)
		if svcErr != nil {
			logReasonCodeRejected(txCtx, tenantID, requestID, "assignment.transitioned", reasonInfo, svcErr)
			return nil, svcErr
		}
		if reasonInfo.OriginalMissing && strings.TrimSpace(reasonCode) == "" {
			reasonCode = "legacy"
			reasonInfo.Filled = true
		}

		anchor, err := s.repo.LockAssignmentByID(txCtx, tenantID, in.AssignmentID)
		if err != nil {
			return nil, mapPgError(err)
		}
		if strings.TrimSpace(anchor.SubjectType) != "person" {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_INVALID_BODY", "assignment subject_type must be person", nil)
		}
		if !anchor.IsPrimary {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_INVALID_BODY", "anchor assignment must be primary", nil)
		}

		freeze, err := s.freezeCheck(settings, txTime, in.EffectiveDate)
		if err != nil {
			maybeLogFrozenWindowRejected(txCtx, tenantID, requestID, initiatorID, "assignment.transitioned", "org_assignment", anchor.ID, in.EffectiveDate, freeze, err, nil)
			return nil, err
		}

		if !in.EffectiveDate.After(anchor.EffectiveDate) {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_USE_CORRECT", "use correct for in-place updates", nil)
		}
		if !in.EffectiveDate.Before(anchor.EndDate) {
			return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_INVALID_BODY", "effective_date must be within current window", nil)
		}

		pernr := strings.TrimSpace(anchor.Pernr)
		if pernr == "" {
			return nil, newServiceError(http.StatusInternalServerError, "ORG_INVALID_BODY", "anchor pernr is empty", nil)
		}

		payload := map[string]any{
			"pernr":                pernr,
			"effective_date":       in.EffectiveDate.UTC().Format(time.RFC3339Nano),
			"anchor_assignment_id": anchor.ID.String(),
			"reason_code":          reasonCode,
		}

		switch eventType {
		case "transfer":
			if in.OrgNodeID == nil || *in.OrgNodeID == uuid.Nil {
				return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "org_node_id is required for transfer", nil)
			}
			payload["org_node_id"] = (*in.OrgNodeID).String()
			payload["position_id"] = uuidOrEmpty(in.PositionID)
		case "termination":
			assignments, err := s.repo.ListAssignmentsAsOf(txCtx, tenantID, anchor.SubjectID, in.EffectiveDate)
			if err != nil {
				return nil, err
			}
			if len(assignments) == 0 {
				return nil, newServiceError(http.StatusNotFound, "ORG_ASSIGNMENT_NOT_FOUND", "no assignments found at effective_date", nil)
			}
			terminatedIDs := make([]string, 0, len(assignments))
			for _, a := range assignments {
				if !in.EffectiveDate.After(a.EffectiveDate) {
					return nil, newServiceError(http.StatusUnprocessableEntity, "ORG_USE_CORRECT", "use correct for in-place updates", nil)
				}
				terminatedIDs = append(terminatedIDs, a.ID.String())
			}
			payload["terminated_assignment_ids"] = terminatedIDs
		}

		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			return nil, newServiceError(http.StatusInternalServerError, "ORG_INVALID_BODY", "failed to encode payload", err)
		}

		eventRow, created, err := s.repo.UpsertPersonnelEvent(txCtx, tenantID, PersonnelEventInsert{
			RequestID:     requestID,
			InitiatorID:   initiatorID,
			EventType:     eventType,
			PersonUUID:    anchor.SubjectID,
			Pernr:         pernr,
			EffectiveDate: in.EffectiveDate,
			ReasonCode:    reasonCode,
			Payload:       payloadJSON,
		})
		if err != nil {
			return nil, err
		}
		if !created {
			return &TransitionAssignmentResult{Event: eventRow, Created: false}, nil
		}

		switch eventType {
		case "transfer":
			_, err := s.UpdateAssignment(txCtx, tenantID, requestID, initiatorID, UpdateAssignmentInput{
				AssignmentID:  anchor.ID,
				EffectiveDate: in.EffectiveDate,
				ReasonCode:    reasonCode,
				ReasonNote:    in.ReasonNote,
				PositionID:    in.PositionID,
				OrgNodeID:     in.OrgNodeID,
			})
			if err != nil {
				return nil, err
			}
		case "termination":
			assignments, err := s.repo.ListAssignmentsAsOf(txCtx, tenantID, anchor.SubjectID, in.EffectiveDate)
			if err != nil {
				return nil, err
			}
			for _, a := range assignments {
				if _, err := s.RescindAssignment(txCtx, tenantID, requestID, initiatorID, RescindAssignmentInput{
					AssignmentID:  a.ID,
					EffectiveDate: in.EffectiveDate,
					ReasonCode:    reasonCode,
					ReasonNote:    in.ReasonNote,
				}); err != nil {
					return nil, err
				}
			}
		}

		ev := buildEventV1(requestID, tenantID, initiatorID, txTime, "personnel_event.created", "org_personnel_event", eventRow.ID, in.EffectiveDate, endOfTime)
		ev.NewValues = mustMarshalJSON(map[string]any{
			"personnel_event_id": eventRow.ID.String(),
			"event_type":         eventRow.EventType,
			"person_uuid":        eventRow.PersonUUID.String(),
			"pernr":              eventRow.Pernr,
			"effective_date":     eventRow.EffectiveDate.UTC().Format(time.RFC3339Nano),
			"reason_code":        eventRow.ReasonCode,
		})
		if err := s.enqueueOutboxEvents(txCtx, tenantID, []events.OrgEventV1{ev}); err != nil {
			return nil, err
		}

		return &TransitionAssignmentResult{Event: eventRow, Created: true}, nil
	})
}
