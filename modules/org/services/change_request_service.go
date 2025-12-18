package services

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/org/domain/changerequest"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/serrors"
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
		return nil, serrors.NewFieldRequiredError("request_id", "Org.ChangeRequests.Fields.request_id")
	}
	if params.RequesterID == uuid.Nil {
		return nil, serrors.NewFieldRequiredError("requester_id", "Org.ChangeRequests.Fields.requester_id")
	}
	if len(params.Payload) == 0 {
		return nil, serrors.NewFieldRequiredError("payload", "Org.ChangeRequests.Fields.payload")
	}

	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		return nil, err
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
