package entities

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type SuperadminAuditLog struct {
	ID          int64
	ActorUserID *int64
	ActorEmail  string
	ActorName   *string
	TenantID    *uuid.UUID
	Action      string
	Payload     json.RawMessage
	IPAddress   *string
	UserAgent   *string
	CreatedAt   time.Time
}
