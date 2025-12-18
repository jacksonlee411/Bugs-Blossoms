package changerequest

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	StatusDraft     = "draft"
	StatusSubmitted = "submitted"
	StatusApproved  = "approved"
	StatusRejected  = "rejected"
	StatusCancelled = "cancelled"
)

type ChangeRequest struct {
	TenantID             uuid.UUID       `json:"tenant_id"`
	ID                   uuid.UUID       `json:"id"`
	RequestID            string          `json:"request_id"`
	RequesterID          uuid.UUID       `json:"requester_id"`
	Status               string          `json:"status"`
	PayloadSchemaVersion int32           `json:"payload_schema_version"`
	Payload              json.RawMessage `json:"payload"`
	Notes                *string         `json:"notes,omitempty"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}
