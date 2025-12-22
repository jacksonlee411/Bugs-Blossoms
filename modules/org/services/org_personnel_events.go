package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/iota-uz/iota-sdk/modules/org/domain/events"
)

type PersonnelEventInsert struct {
	RequestID     string
	InitiatorID   uuid.UUID
	EventType     string
	PersonUUID    uuid.UUID
	Pernr         string
	EffectiveDate time.Time
	ReasonCode    string
	Payload       json.RawMessage
}

type PersonnelEventRow struct {
	ID            uuid.UUID
	RequestID     string
	InitiatorID   uuid.UUID
	EventType     string
	PersonUUID    uuid.UUID
	Pernr         string
	EffectiveDate time.Time
	ReasonCode    string
	Payload       json.RawMessage
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type PersonnelEventApplyResult struct {
	Event   PersonnelEventRow
	Created bool
}

type HirePersonnelEventInput struct {
	Pernr         string
	OrgNodeID     uuid.UUID
	PositionID    *uuid.UUID
	EffectiveDate time.Time
	AllocatedFTE  float64
	ReasonCode    string
}

type TransferPersonnelEventInput struct {
	Pernr         string
	OrgNodeID     uuid.UUID
	PositionID    *uuid.UUID
	EffectiveDate time.Time
	AllocatedFTE  float64
	ReasonCode    string
}

type TerminationPersonnelEventInput struct {
	Pernr         string
	EffectiveDate time.Time
	ReasonCode    string
}

func (s *OrgService) HirePersonnelEvent(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in HirePersonnelEventInput) (*PersonnelEventApplyResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	txTime := time.Now().UTC()

	pernr := strings.TrimSpace(in.Pernr)
	if pernr == "" || in.OrgNodeID == uuid.Nil || in.EffectiveDate.IsZero() || strings.TrimSpace(in.ReasonCode) == "" {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "pernr/org_node_id/effective_date/reason_code are required", nil)
	}
	allocatedFTE := in.AllocatedFTE
	if allocatedFTE <= 0 {
		allocatedFTE = 1.0
	}

	return inTx(ctx, tenantID, func(txCtx context.Context) (*PersonnelEventApplyResult, error) {
		personUUID, err := s.repo.ResolvePersonUUIDByPernr(txCtx, tenantID, pernr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, newServiceError(http.StatusNotFound, "ORG_PERSON_NOT_FOUND", "pernr not found", nil)
			}
			return nil, err
		}

		payload, err := json.Marshal(map[string]any{
			"pernr":          pernr,
			"org_node_id":    in.OrgNodeID.String(),
			"position_id":    uuidOrEmpty(in.PositionID),
			"effective_date": in.EffectiveDate.UTC().Format(time.RFC3339Nano),
			"allocated_fte":  allocatedFTE,
			"reason_code":    strings.TrimSpace(in.ReasonCode),
		})
		if err != nil {
			return nil, newServiceError(http.StatusInternalServerError, "ORG_INVALID_BODY", "failed to encode payload", err)
		}

		row, created, err := s.repo.UpsertPersonnelEvent(txCtx, tenantID, PersonnelEventInsert{
			RequestID:     requestID,
			InitiatorID:   initiatorID,
			EventType:     "hire",
			PersonUUID:    personUUID,
			Pernr:         pernr,
			EffectiveDate: in.EffectiveDate,
			ReasonCode:    strings.TrimSpace(in.ReasonCode),
			Payload:       payload,
		})
		if err != nil {
			return nil, err
		}
		if !created {
			return &PersonnelEventApplyResult{Event: row, Created: false}, nil
		}

		_, err = s.CreateAssignment(txCtx, tenantID, requestID, initiatorID, CreateAssignmentInput{
			Pernr:         pernr,
			EffectiveDate: in.EffectiveDate,
			ReasonCode:    strings.TrimSpace(in.ReasonCode),
			AllocatedFTE:  allocatedFTE,
			PositionID:    in.PositionID,
			OrgNodeID:     &in.OrgNodeID,
		})
		if err != nil {
			return nil, err
		}

		ev := buildEventV1(requestID, tenantID, initiatorID, txTime, "personnel_event.created", "org_personnel_event", row.ID, in.EffectiveDate, endOfTime)
		ev.NewValues = mustMarshalJSON(map[string]any{
			"personnel_event_id": row.ID.String(),
			"event_type":         row.EventType,
			"person_uuid":        row.PersonUUID.String(),
			"pernr":              row.Pernr,
			"effective_date":     row.EffectiveDate.UTC().Format(time.RFC3339Nano),
			"reason_code":        row.ReasonCode,
		})

		if err := s.enqueueOutboxEvents(txCtx, tenantID, []events.OrgEventV1{ev}); err != nil {
			return nil, err
		}
		return &PersonnelEventApplyResult{Event: row, Created: true}, nil
	})
}

func (s *OrgService) TransferPersonnelEvent(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in TransferPersonnelEventInput) (*PersonnelEventApplyResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	txTime := time.Now().UTC()

	pernr := strings.TrimSpace(in.Pernr)
	if pernr == "" || in.OrgNodeID == uuid.Nil || in.EffectiveDate.IsZero() || strings.TrimSpace(in.ReasonCode) == "" {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "pernr/org_node_id/effective_date/reason_code are required", nil)
	}
	allocatedFTE := in.AllocatedFTE
	if allocatedFTE <= 0 {
		allocatedFTE = 1.0
	}

	return inTx(ctx, tenantID, func(txCtx context.Context) (*PersonnelEventApplyResult, error) {
		personUUID, err := s.repo.ResolvePersonUUIDByPernr(txCtx, tenantID, pernr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, newServiceError(http.StatusNotFound, "ORG_PERSON_NOT_FOUND", "pernr not found", nil)
			}
			return nil, err
		}

		payload, err := json.Marshal(map[string]any{
			"pernr":          pernr,
			"org_node_id":    in.OrgNodeID.String(),
			"position_id":    uuidOrEmpty(in.PositionID),
			"effective_date": in.EffectiveDate.UTC().Format(time.RFC3339Nano),
			"allocated_fte":  allocatedFTE,
			"reason_code":    strings.TrimSpace(in.ReasonCode),
		})
		if err != nil {
			return nil, newServiceError(http.StatusInternalServerError, "ORG_INVALID_BODY", "failed to encode payload", err)
		}

		row, created, err := s.repo.UpsertPersonnelEvent(txCtx, tenantID, PersonnelEventInsert{
			RequestID:     requestID,
			InitiatorID:   initiatorID,
			EventType:     "transfer",
			PersonUUID:    personUUID,
			Pernr:         pernr,
			EffectiveDate: in.EffectiveDate,
			ReasonCode:    strings.TrimSpace(in.ReasonCode),
			Payload:       payload,
		})
		if err != nil {
			return nil, err
		}
		if !created {
			return &PersonnelEventApplyResult{Event: row, Created: false}, nil
		}

		primary, err := s.findPrimaryAssignmentAt(txCtx, tenantID, personUUID, in.EffectiveDate)
		if err != nil {
			return nil, err
		}

		fte := allocatedFTE
		_, err = s.UpdateAssignment(txCtx, tenantID, requestID, initiatorID, UpdateAssignmentInput{
			AssignmentID:  primary.ID,
			EffectiveDate: in.EffectiveDate,
			ReasonCode:    strings.TrimSpace(in.ReasonCode),
			AllocatedFTE:  &fte,
			PositionID:    in.PositionID,
			OrgNodeID:     &in.OrgNodeID,
		})
		if err != nil {
			return nil, err
		}

		ev := buildEventV1(requestID, tenantID, initiatorID, txTime, "personnel_event.created", "org_personnel_event", row.ID, in.EffectiveDate, endOfTime)
		ev.NewValues = mustMarshalJSON(map[string]any{
			"personnel_event_id": row.ID.String(),
			"event_type":         row.EventType,
			"person_uuid":        row.PersonUUID.String(),
			"pernr":              row.Pernr,
			"effective_date":     row.EffectiveDate.UTC().Format(time.RFC3339Nano),
			"reason_code":        row.ReasonCode,
		})

		if err := s.enqueueOutboxEvents(txCtx, tenantID, []events.OrgEventV1{ev}); err != nil {
			return nil, err
		}
		return &PersonnelEventApplyResult{Event: row, Created: true}, nil
	})
}

func (s *OrgService) TerminationPersonnelEvent(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, in TerminationPersonnelEventInput) (*PersonnelEventApplyResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(http.StatusBadRequest, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	txTime := time.Now().UTC()

	pernr := strings.TrimSpace(in.Pernr)
	if pernr == "" || in.EffectiveDate.IsZero() || strings.TrimSpace(in.ReasonCode) == "" {
		return nil, newServiceError(http.StatusBadRequest, "ORG_INVALID_BODY", "pernr/effective_date/reason_code are required", nil)
	}

	return inTx(ctx, tenantID, func(txCtx context.Context) (*PersonnelEventApplyResult, error) {
		personUUID, err := s.repo.ResolvePersonUUIDByPernr(txCtx, tenantID, pernr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, newServiceError(http.StatusNotFound, "ORG_PERSON_NOT_FOUND", "pernr not found", nil)
			}
			return nil, err
		}

		payload, err := json.Marshal(map[string]any{
			"pernr":          pernr,
			"effective_date": in.EffectiveDate.UTC().Format(time.RFC3339Nano),
			"reason_code":    strings.TrimSpace(in.ReasonCode),
		})
		if err != nil {
			return nil, newServiceError(http.StatusInternalServerError, "ORG_INVALID_BODY", "failed to encode payload", err)
		}

		row, created, err := s.repo.UpsertPersonnelEvent(txCtx, tenantID, PersonnelEventInsert{
			RequestID:     requestID,
			InitiatorID:   initiatorID,
			EventType:     "termination",
			PersonUUID:    personUUID,
			Pernr:         pernr,
			EffectiveDate: in.EffectiveDate,
			ReasonCode:    strings.TrimSpace(in.ReasonCode),
			Payload:       payload,
		})
		if err != nil {
			return nil, err
		}
		if !created {
			return &PersonnelEventApplyResult{Event: row, Created: false}, nil
		}

		primary, err := s.findPrimaryAssignmentAt(txCtx, tenantID, personUUID, in.EffectiveDate)
		if err != nil {
			return nil, err
		}
		_, err = s.RescindAssignment(txCtx, tenantID, requestID, initiatorID, RescindAssignmentInput{
			AssignmentID:  primary.ID,
			EffectiveDate: in.EffectiveDate,
			ReasonCode:    strings.TrimSpace(in.ReasonCode),
		})
		if err != nil {
			return nil, err
		}

		ev := buildEventV1(requestID, tenantID, initiatorID, txTime, "personnel_event.created", "org_personnel_event", row.ID, in.EffectiveDate, endOfTime)
		ev.NewValues = mustMarshalJSON(map[string]any{
			"personnel_event_id": row.ID.String(),
			"event_type":         row.EventType,
			"person_uuid":        row.PersonUUID.String(),
			"pernr":              row.Pernr,
			"effective_date":     row.EffectiveDate.UTC().Format(time.RFC3339Nano),
			"reason_code":        row.ReasonCode,
		})

		if err := s.enqueueOutboxEvents(txCtx, tenantID, []events.OrgEventV1{ev}); err != nil {
			return nil, err
		}
		return &PersonnelEventApplyResult{Event: row, Created: true}, nil
	})
}

func (s *OrgService) findPrimaryAssignmentAt(ctx context.Context, tenantID uuid.UUID, personUUID uuid.UUID, asOf time.Time) (AssignmentViewRow, error) {
	rows, err := s.repo.ListAssignmentsAsOf(ctx, tenantID, personUUID, asOf)
	if err != nil {
		return AssignmentViewRow{}, err
	}
	for _, row := range rows {
		if row.IsPrimary {
			return row, nil
		}
	}
	return AssignmentViewRow{}, newServiceError(http.StatusNotFound, "ORG_PRIMARY_NOT_FOUND", "primary assignment not found at effective_date", nil)
}

func uuidOrEmpty(id *uuid.UUID) string {
	if id == nil || *id == uuid.Nil {
		return ""
	}
	return (*id).String()
}

func mustMarshalJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}
