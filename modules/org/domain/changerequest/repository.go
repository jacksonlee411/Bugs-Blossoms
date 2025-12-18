package changerequest

import "context"

type Repository interface {
	Upsert(ctx context.Context, cr *ChangeRequest) (*ChangeRequest, error)
	GetByRequestID(ctx context.Context, requestID string) (*ChangeRequest, error)
}
