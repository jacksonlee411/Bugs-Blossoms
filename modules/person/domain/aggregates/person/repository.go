package person

import (
	"context"

	"github.com/google/uuid"
)

type FindParams struct {
	Q      string
	Limit  int
	Offset int
}

type Repository interface {
	GetPaginated(ctx context.Context, params *FindParams) ([]Person, int64, error)
	GetByUUID(ctx context.Context, personUUID uuid.UUID) (Person, error)
	GetByPernr(ctx context.Context, pernr string) (Person, error)
	Create(ctx context.Context, p Person) (Person, error)
}
