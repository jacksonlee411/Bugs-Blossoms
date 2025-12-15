package outbox

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Message is the unit stored in <module>_outbox.
type Message struct {
	TenantID uuid.UUID
	Topic    string
	EventID  uuid.UUID
	Payload  json.RawMessage
}

// Meta is the stable dispatch metadata (idempotency + tracing + ops).
type Meta struct {
	Table    pgx.Identifier
	TenantID uuid.UUID
	Topic    string
	EventID  uuid.UUID
	Sequence int64
	Attempts int

	// Optional tracing context (W3C Trace Context). See DEV-PLAN-017 §5.4.
	TraceParent string
	TraceState  string
}

// DispatchedMessage is the unit delivered by Relay to Dispatcher.
type DispatchedMessage struct {
	Meta    Meta
	Payload json.RawMessage
}
