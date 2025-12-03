package persistence

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// PolicyChangeStatus represents the lifecycle state of a policy change request.
type PolicyChangeStatus string

const (
	PolicyChangeStatusDraft         PolicyChangeStatus = "draft"
	PolicyChangeStatusPendingReview PolicyChangeStatus = "pending_review"
	PolicyChangeStatusApproved      PolicyChangeStatus = "approved"
	PolicyChangeStatusRejected      PolicyChangeStatus = "rejected"
	PolicyChangeStatusMerged        PolicyChangeStatus = "merged"
	PolicyChangeStatusFailed        PolicyChangeStatus = "failed"
	PolicyChangeStatusCanceled      PolicyChangeStatus = "canceled"
)

// PolicyChangeRequest models the persistence layer representation of a request.
type PolicyChangeRequest struct {
	ID                    uuid.UUID
	Status                PolicyChangeStatus
	RequesterID           uuid.UUID
	ApproverID            *uuid.UUID
	TenantID              uuid.UUID
	Subject               string
	Domain                string
	Action                string
	Object                string
	Reason                string
	Diff                  json.RawMessage
	BasePolicyRevision    string
	AppliedPolicyRevision *string
	AppliedPolicySnapshot json.RawMessage
	PRLink                *string
	BotJobID              *string
	BotLock               *string
	BotLockedAt           *time.Time
	BotAttempts           int
	ErrorLog              *string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	ReviewedAt            *time.Time
}

// Nullable represents a tri-state value (unset, set to NULL, set to value).
type Nullable[T any] struct {
	Set   bool
	Valid bool
	Value T
}

// NewNullableValue creates a Nullable that holds a non-null value.
func NewNullableValue[T any](value T) Nullable[T] {
	return Nullable[T]{Set: true, Valid: true, Value: value}
}

// NewNullableNull creates a Nullable that explicitly sets NULL.
func NewNullableNull[T any]() Nullable[T] {
	var zero T
	return Nullable[T]{Set: true, Valid: false, Value: zero}
}

// IsUnset reports whether the nullable value should be ignored.
func (n Nullable[T]) IsUnset() bool {
	return !n.Set
}

// FindParams defines filters for listing requests.
type FindParams struct {
	Statuses    []PolicyChangeStatus
	TenantID    *uuid.UUID
	RequesterID *uuid.UUID
	ApproverID  *uuid.UUID
	Subject     string
	Domain      string
	Limit       int
	Offset      int
	SortAsc     bool
}

// UpdateStatusParams describes allowed updates for status transitions.
type UpdateStatusParams struct {
	Status     PolicyChangeStatus
	ApproverID Nullable[uuid.UUID]
	ReviewedAt Nullable[time.Time]
}

// UpdateBotMetadataParams describes bot related fields that can be mutated.
type UpdateBotMetadataParams struct {
	BotJobID              Nullable[string]
	BotAttempts           Nullable[int]
	ErrorLog              Nullable[string]
	PRLink                Nullable[string]
	AppliedPolicyRevision Nullable[string]
	AppliedPolicySnapshot Nullable[json.RawMessage]
}

// BotLockParams controls lock acquisition semantics.
type BotLockParams struct {
	Locker      string
	LockedAt    time.Time
	StaleBefore time.Time
}
