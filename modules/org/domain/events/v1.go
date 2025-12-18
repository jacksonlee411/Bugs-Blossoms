package events

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	TopicOrgChangedV1           = "org.changed.v1"
	TopicOrgAssignmentChangedV1 = "org.assignment.changed.v1"
	EventVersionV1              = 1
)

type EffectiveWindowV1 struct {
	EffectiveDate time.Time `json:"effective_date"`
	EndDate       time.Time `json:"end_date"`
}

type OrgEventV1 struct {
	EventID         uuid.UUID         `json:"event_id"`
	EventVersion    int               `json:"event_version"`
	RequestID       string            `json:"request_id"`
	TenantID        uuid.UUID         `json:"tenant_id"`
	TransactionTime time.Time         `json:"transaction_time"`
	InitiatorID     uuid.UUID         `json:"initiator_id"`
	ChangeType      string            `json:"change_type"`
	EntityType      string            `json:"entity_type"`
	EntityID        uuid.UUID         `json:"entity_id"`
	EntityVersion   int64             `json:"entity_version"`
	EffectiveWindow EffectiveWindowV1 `json:"effective_window"`
	Sequence        *int64            `json:"sequence,omitempty"`
	OldValues       json.RawMessage   `json:"old_values,omitempty"`
	NewValues       json.RawMessage   `json:"new_values"`
}
