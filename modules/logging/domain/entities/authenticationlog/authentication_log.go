package authenticationlog

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type AuthenticationLog struct {
	ID        uint
	TenantID  uuid.UUID
	UserID    uint
	IP        string
	UserAgent string
	CreatedAt time.Time
}

type FindParams struct {
	UserID    *uint
	IP        string
	UserAgent string
	From      *time.Time
	To        *time.Time
	Limit     int
	Offset    int
	SortBy    []string
}

type Repository interface {
	List(ctx context.Context, params *FindParams) ([]*AuthenticationLog, error)
	Count(ctx context.Context, params *FindParams) (int64, error)
	Create(ctx context.Context, log *AuthenticationLog) error
}
