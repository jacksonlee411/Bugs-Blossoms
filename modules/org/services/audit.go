package services

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type OrgSettings struct {
	FreezeMode                         string
	FreezeGraceDays                    int
	PositionCatalogValidationMode      string
	PositionRestrictionsValidationMode string
}

type AuditLogInsert struct {
	RequestID        string
	TransactionTime  time.Time
	InitiatorID      uuid.UUID
	ChangeType       string
	EntityType       string
	EntityID         uuid.UUID
	EffectiveDate    time.Time
	EndDate          time.Time
	OldValues        any
	NewValues        any
	Meta             map[string]any
	FreezeMode       string
	FreezeViolation  bool
	FreezeCutoffUTC  time.Time
	AffectedAtUTC    time.Time
	Operation        string
	OperationDetails map[string]any
}

func (a AuditLogInsert) marshalJSON(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (a AuditLogInsert) MarshalOldValues() (string, bool, error) {
	s, err := a.marshalJSON(a.OldValues)
	if err != nil {
		return "", false, err
	}
	if s == "" {
		return "", false, nil
	}
	return s, true, nil
}

func (a AuditLogInsert) MarshalNewValues() (string, error) {
	if a.NewValues == nil {
		return "{}", nil
	}
	s, err := a.marshalJSON(a.NewValues)
	if err != nil {
		return "", err
	}
	if s == "" {
		return "{}", nil
	}
	return s, nil
}

func (a AuditLogInsert) MarshalMeta() (string, error) {
	meta := map[string]any{}
	for k, v := range a.Meta {
		meta[k] = v
	}
	if a.Operation != "" {
		meta["operation"] = a.Operation
	}
	if a.OperationDetails != nil {
		meta["operation_details"] = a.OperationDetails
	}
	if a.FreezeMode != "" {
		meta["freeze_mode"] = a.FreezeMode
		meta["freeze_cutoff_utc"] = a.FreezeCutoffUTC.UTC().Format(time.RFC3339)
		meta["freeze_violation"] = a.FreezeViolation
		meta["affected_at_utc"] = a.AffectedAtUTC.UTC().Format(time.RFC3339)
	}

	s, err := a.marshalJSON(meta)
	if err != nil {
		return "", err
	}
	if s == "" {
		return "{}", nil
	}
	return s, nil
}

func (a AuditLogInsert) Validate() error {
	if a.RequestID == "" {
		return fmt.Errorf("request_id is required")
	}
	if a.TransactionTime.IsZero() {
		return fmt.Errorf("transaction_time is required")
	}
	if a.InitiatorID == uuid.Nil {
		return fmt.Errorf("initiator_id is required")
	}
	if a.ChangeType == "" {
		return fmt.Errorf("change_type is required")
	}
	if a.EntityType == "" {
		return fmt.Errorf("entity_type is required")
	}
	if a.EntityID == uuid.Nil {
		return fmt.Errorf("entity_id is required")
	}
	if a.EffectiveDate.IsZero() || a.EndDate.IsZero() || !a.EffectiveDate.Before(a.EndDate) {
		return fmt.Errorf("effective window is invalid")
	}
	return nil
}
