package dtos

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/core/services"
)

// APIError standardizes JSON error responses.
type APIError struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

// PolicyDraftRequest captures incoming payloads for draft creation.
type PolicyDraftRequest struct {
	Object       string          `json:"object"`
	Action       string          `json:"action"`
	Reason       string          `json:"reason"`
	Diff         json.RawMessage `json:"diff"`
	BaseRevision string          `json:"base_revision"`
	Domain       string          `json:"domain"`
}

// PolicyDraftResponse serializes drafts for API responses.
type PolicyDraftResponse struct {
	ID                    uuid.UUID       `json:"id"`
	Status                string          `json:"status"`
	RequesterID           uuid.UUID       `json:"requester_id"`
	ApproverID            *uuid.UUID      `json:"approver_id,omitempty"`
	TenantID              uuid.UUID       `json:"tenant_id"`
	Subject               string          `json:"subject"`
	Domain                string          `json:"domain"`
	Action                string          `json:"action"`
	Object                string          `json:"object"`
	Reason                string          `json:"reason"`
	Diff                  json.RawMessage `json:"diff"`
	BasePolicyRevision    string          `json:"base_policy_revision"`
	AppliedPolicyRevision *string         `json:"applied_policy_revision,omitempty"`
	AppliedPolicySnapshot json.RawMessage `json:"applied_policy_snapshot,omitempty"`
	PRLink                *string         `json:"pr_link,omitempty"`
	BotJobID              *string         `json:"bot_job_id,omitempty"`
	BotLock               *string         `json:"bot_lock,omitempty"`
	BotLockedAt           *time.Time      `json:"bot_locked_at,omitempty"`
	BotAttempts           int             `json:"bot_attempts"`
	ErrorLog              *string         `json:"error_log,omitempty"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
	ReviewedAt            *time.Time      `json:"reviewed_at,omitempty"`
}

// PolicyDraftListResponse wraps paginated drafts.
type PolicyDraftListResponse struct {
	Data  []PolicyDraftResponse `json:"data"`
	Total int64                 `json:"total"`
}

// PolicyEntryResponse mirrors services.PolicyEntry for JSON responses.
type PolicyEntryResponse struct {
	Type    string `json:"type"`
	Subject string `json:"subject"`
	Domain  string `json:"domain"`
	Object  string `json:"object"`
	Action  string `json:"action"`
	Effect  string `json:"effect"`
}

// DebugResponse provides a minimal Authz.Debug output.
type DebugResponse struct {
	Allowed bool            `json:"allowed"`
	Mode    string          `json:"mode"`
	Request DebugRequestDTO `json:"request"`
}

// DebugRequestDTO echoes the evaluated request payload.
type DebugRequestDTO struct {
	Subject string `json:"subject"`
	Domain  string `json:"domain"`
	Object  string `json:"object"`
	Action  string `json:"action"`
}

// NewPolicyDraftResponse converts a service draft into a DTO.
func NewPolicyDraftResponse(d services.PolicyDraft) PolicyDraftResponse {
	return PolicyDraftResponse{
		ID:                    d.ID,
		Status:                string(d.Status),
		RequesterID:           d.RequesterID,
		ApproverID:            d.ApproverID,
		TenantID:              d.TenantID,
		Subject:               d.Subject,
		Domain:                d.Domain,
		Action:                d.Action,
		Object:                d.Object,
		Reason:                d.Reason,
		Diff:                  d.Diff,
		BasePolicyRevision:    d.BasePolicyRevision,
		AppliedPolicyRevision: d.AppliedPolicyRevision,
		AppliedPolicySnapshot: d.AppliedPolicySnapshot,
		PRLink:                d.PRLink,
		BotJobID:              d.BotJobID,
		BotLock:               d.BotLock,
		BotLockedAt:           d.BotLockedAt,
		BotAttempts:           d.BotAttempts,
		ErrorLog:              d.ErrorLog,
		CreatedAt:             d.CreatedAt,
		UpdatedAt:             d.UpdatedAt,
		ReviewedAt:            d.ReviewedAt,
	}
}
