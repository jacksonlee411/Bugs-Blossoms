package changerequest

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Repository interface {
	Upsert(ctx context.Context, cr *ChangeRequest) (*ChangeRequest, error)
	UpdateDraftByID(ctx context.Context, id uuid.UUID, payload []byte, notes *string) (*ChangeRequest, error)
	UpdateStatusByID(ctx context.Context, id uuid.UUID, status string) (*ChangeRequest, error)
	GetByRequestID(ctx context.Context, requestID string) (*ChangeRequest, error)
	GetByID(ctx context.Context, id uuid.UUID) (*ChangeRequest, error)
	List(ctx context.Context, status string, limit int, cursorUpdatedAt *time.Time, cursorID *uuid.UUID) ([]*ChangeRequest, error)
}
