package actionlog

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type ActionLog struct {
	ID        uint
	TenantID  uuid.UUID
	UserID    *uint
	Method    string
	Path      string
	Before    json.RawMessage
	After     json.RawMessage
	UserAgent string
	IP        string
	CreatedAt time.Time
}

type FindParams struct {
	UserID    *uint
	Method    string
	Path      string
	IP        string
	UserAgent string
	From      *time.Time
	To        *time.Time
	Limit     int
	Offset    int
	SortBy    []string
}

type Repository interface {
	List(ctx context.Context, params *FindParams) ([]*ActionLog, error)
	Count(ctx context.Context, params *FindParams) (int64, error)
	Create(ctx context.Context, log *ActionLog) error
}
