package persistence

import (
	"context"
	"errors"

	"github.com/iota-uz/iota-sdk/modules/hrm/domain/entities/position"
	positionsqlc "github.com/iota-uz/iota-sdk/modules/hrm/infrastructure/sqlc/position"
	"github.com/iota-uz/iota-sdk/pkg/composables"

	"github.com/jackc/pgx/v5"
)

var (
	ErrPositionNotFound = errors.New("position not found")
)

type GormPositionRepository struct{}

func NewPositionRepository() position.Repository {
	return &GormPositionRepository{}
}

func (g *GormPositionRepository) GetPaginated(ctx context.Context, params *position.FindParams) ([]*position.Position, error) {
	if params == nil {
		params = &position.FindParams{}
	}

	_, pgTenantID, err := tenantIDs(ctx)
	if err != nil {
		return nil, err
	}

	queries, err := g.positionQueries(ctx)
	if err != nil {
		return nil, err
	}

	if params.ID != 0 {
		row, err := queries.GetPositionByID(ctx, positionsqlc.GetPositionByIDParams{
			ID:       int32(params.ID),
			TenantID: pgTenantID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrPositionNotFound
			}
			return nil, err
		}
		entity, err := toDomainPositionFromSQLC(row)
		if err != nil {
			return nil, err
		}
		return []*position.Position{entity}, nil
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := params.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := queries.ListPositionsPaginated(ctx, positionsqlc.ListPositionsPaginatedParams{
		TenantID:  pgTenantID,
		RowOffset: int32(offset),
		RowLimit:  int32(limit),
	})
	if err != nil {
		return nil, err
	}

	return toDomainPositionsFromSQLC(rows)
}

func (g *GormPositionRepository) Count(ctx context.Context) (int64, error) {
	_, pgTenantID, err := tenantIDs(ctx)
	if err != nil {
		return 0, err
	}

	queries, err := g.positionQueries(ctx)
	if err != nil {
		return 0, err
	}

	return queries.CountPositions(ctx, pgTenantID)
}

func (g *GormPositionRepository) GetAll(ctx context.Context) ([]*position.Position, error) {
	_, pgTenantID, err := tenantIDs(ctx)
	if err != nil {
		return nil, err
	}

	queries, err := g.positionQueries(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListPositionsByTenant(ctx, pgTenantID)
	if err != nil {
		return nil, err
	}

	return toDomainPositionsFromSQLC(rows)
}

func (g *GormPositionRepository) GetByID(ctx context.Context, id int64) (*position.Position, error) {
	_, pgTenantID, err := tenantIDs(ctx)
	if err != nil {
		return nil, err
	}

	queries, err := g.positionQueries(ctx)
	if err != nil {
		return nil, err
	}

	row, err := queries.GetPositionByID(ctx, positionsqlc.GetPositionByIDParams{
		ID:       int32(id),
		TenantID: pgTenantID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPositionNotFound
		}
		return nil, err
	}

	return toDomainPositionFromSQLC(row)
}

func (g *GormPositionRepository) Create(ctx context.Context, data *position.Position) error {
	tenantUUID, _, err := tenantIDs(ctx)
	if err != nil {
		return err
	}

	queries, err := g.positionQueries(ctx)
	if err != nil {
		return err
	}

	params := buildPositionCreateParams(data, tenantUUID)
	id, err := queries.CreatePosition(ctx, params)
	if err != nil {
		return err
	}

	data.ID = uint(id)
	data.TenantID = tenantUUID.String()
	return nil
}

func (g *GormPositionRepository) Update(ctx context.Context, data *position.Position) error {
	tenantUUID, _, err := tenantIDs(ctx)
	if err != nil {
		return err
	}

	queries, err := g.positionQueries(ctx)
	if err != nil {
		return err
	}

	params := buildPositionUpdateParams(data, tenantUUID)
	return queries.UpdatePosition(ctx, params)
}

func (g *GormPositionRepository) Delete(ctx context.Context, id int64) error {
	_, pgTenantID, err := tenantIDs(ctx)
	if err != nil {
		return err
	}

	queries, err := g.positionQueries(ctx)
	if err != nil {
		return err
	}

	return queries.DeletePosition(ctx, positionsqlc.DeletePositionParams{
		ID:       int32(id),
		TenantID: pgTenantID,
	})
}

func (g *GormPositionRepository) positionQueries(ctx context.Context) (*positionsqlc.Queries, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	return positionsqlc.New(tx), nil
}
