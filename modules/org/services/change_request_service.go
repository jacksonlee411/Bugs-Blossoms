package services

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/iota-uz/iota-sdk/modules/org/domain/changerequest"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

type ChangeRequestService struct {
	Repo changerequest.Repository
}

func NewChangeRequestService(repo changerequest.Repository) *ChangeRequestService {
	return &ChangeRequestService{Repo: repo}
}

type SaveDraftChangeRequestParams struct {
	RequestID   string          `json:"request_id"`
	RequesterID uuid.UUID       `json:"requester_id"`
	Payload     json.RawMessage `json:"payload"`
	Notes       *string         `json:"notes,omitempty"`
}

func (s *ChangeRequestService) SaveDraft(ctx context.Context, params SaveDraftChangeRequestParams) (*changerequest.ChangeRequest, error) {
	if params.RequestID == "" {
		return nil, newServiceError(400, "ORG_CHANGE_REQUEST_INVALID_BODY", "request_id is required", nil)
	}
	if params.RequesterID == uuid.Nil {
		return nil, newServiceError(400, "ORG_CHANGE_REQUEST_INVALID_BODY", "requester_id is required", nil)
	}
	if len(params.Payload) == 0 {
		return nil, newServiceError(422, "ORG_CHANGE_REQUEST_INVALID_BODY", "payload is required", nil)
	}

	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		return nil, err
	}

	existing, err := s.Repo.GetByRequestID(ctx, params.RequestID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if existing != nil && existing.Status != changerequest.StatusDraft {
		return nil, newServiceError(409, "ORG_CHANGE_REQUEST_IMMUTABLE", "change request is immutable", nil)
	}

	cr := &changerequest.ChangeRequest{
		TenantID:             tenantID,
		RequestID:            params.RequestID,
		RequesterID:          params.RequesterID,
		Status:               changerequest.StatusDraft,
		PayloadSchemaVersion: 1,
		Payload:              params.Payload,
		Notes:                params.Notes,
	}
	return s.Repo.Upsert(ctx, cr)
}

type UpdateDraftChangeRequestParams struct {
	ID      uuid.UUID       `json:"id"`
	Payload json.RawMessage `json:"payload"`
	Notes   *string         `json:"notes,omitempty"`
}

func (s *ChangeRequestService) UpdateDraft(ctx context.Context, params UpdateDraftChangeRequestParams) (*changerequest.ChangeRequest, error) {
	if params.ID == uuid.Nil {
		return nil, newServiceError(400, "ORG_CHANGE_REQUEST_INVALID_QUERY", "id is required", nil)
	}
	if len(params.Payload) == 0 {
		return nil, newServiceError(422, "ORG_CHANGE_REQUEST_INVALID_BODY", "payload is required", nil)
	}
	if _, err := composables.UseTenantID(ctx); err != nil {
		return nil, err
	}

	existing, err := s.Repo.GetByID(ctx, params.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, newServiceError(404, "ORG_CHANGE_REQUEST_NOT_FOUND", "not found", err)
		}
		return nil, err
	}
	if existing.Status != changerequest.StatusDraft {
		return nil, newServiceError(409, "ORG_CHANGE_REQUEST_NOT_DRAFT", "change request is not draft", nil)
	}

	updated, err := s.Repo.UpdateDraftByID(ctx, params.ID, []byte(params.Payload), params.Notes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, newServiceError(409, "ORG_CHANGE_REQUEST_NOT_DRAFT", "change request is not draft", err)
		}
		return nil, err
	}
	return updated, nil
}

func (s *ChangeRequestService) Get(ctx context.Context, id uuid.UUID) (*changerequest.ChangeRequest, error) {
	if id == uuid.Nil {
		return nil, newServiceError(400, "ORG_CHANGE_REQUEST_INVALID_QUERY", "id is required", nil)
	}
	if _, err := composables.UseTenantID(ctx); err != nil {
		return nil, err
	}
	cr, err := s.Repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, newServiceError(404, "ORG_CHANGE_REQUEST_NOT_FOUND", "not found", err)
		}
		return nil, err
	}
	return cr, nil
}

func (s *ChangeRequestService) List(ctx context.Context, status string, limit int, cursorUpdatedAt *time.Time, cursorID *uuid.UUID) ([]*changerequest.ChangeRequest, error) {
	if _, err := composables.UseTenantID(ctx); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	return s.Repo.List(ctx, status, limit, cursorUpdatedAt, cursorID)
}

func (s *ChangeRequestService) Submit(ctx context.Context, id uuid.UUID) (*changerequest.ChangeRequest, error) {
	if id == uuid.Nil {
		return nil, newServiceError(400, "ORG_CHANGE_REQUEST_INVALID_QUERY", "id is required", nil)
	}
	if _, err := composables.UseTenantID(ctx); err != nil {
		return nil, err
	}

	existing, err := s.Repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, newServiceError(404, "ORG_CHANGE_REQUEST_NOT_FOUND", "not found", err)
		}
		return nil, err
	}
	if existing.Status != changerequest.StatusDraft {
		return nil, newServiceError(409, "ORG_CHANGE_REQUEST_NOT_DRAFT", "change request is not draft", nil)
	}
	return s.Repo.UpdateStatusByID(ctx, id, changerequest.StatusSubmitted)
}

func (s *ChangeRequestService) Cancel(ctx context.Context, id uuid.UUID) (*changerequest.ChangeRequest, error) {
	if id == uuid.Nil {
		return nil, newServiceError(400, "ORG_CHANGE_REQUEST_INVALID_QUERY", "id is required", nil)
	}
	if _, err := composables.UseTenantID(ctx); err != nil {
		return nil, err
	}

	existing, err := s.Repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, newServiceError(404, "ORG_CHANGE_REQUEST_NOT_FOUND", "not found", err)
		}
		return nil, err
	}
	switch existing.Status {
	case changerequest.StatusDraft, changerequest.StatusSubmitted:
		return s.Repo.UpdateStatusByID(ctx, id, changerequest.StatusCancelled)
	default:
		return nil, newServiceError(409, "ORG_CHANGE_REQUEST_NOT_CANCELLABLE", "change request cannot be cancelled", nil)
	}
}
