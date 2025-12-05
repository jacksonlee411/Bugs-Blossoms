package services

import (
	"context"

	"github.com/iota-uz/iota-sdk/modules/core/infrastructure/query"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/viewmodels"
)

type GroupQueryService struct {
	repo query.GroupQueryRepository
}

func NewGroupQueryService(repo query.GroupQueryRepository) *GroupQueryService {
	return &GroupQueryService{repo: repo}
}

func (s *GroupQueryService) FindGroups(ctx context.Context, params *query.GroupFindParams) ([]*viewmodels.Group, int, error) {
	if err := authorizeGroups(ctx, "list"); err != nil {
		return nil, 0, err
	}
	return s.repo.FindGroups(ctx, params)
}

func (s *GroupQueryService) FindGroupByID(ctx context.Context, groupID string) (*viewmodels.Group, error) {
	if err := authorizeGroups(ctx, "view"); err != nil {
		return nil, err
	}
	return s.repo.FindGroupByID(ctx, groupID)
}

func (s *GroupQueryService) SearchGroups(ctx context.Context, params *query.GroupFindParams) ([]*viewmodels.Group, int, error) {
	if err := authorizeGroups(ctx, "list"); err != nil {
		return nil, 0, err
	}
	return s.repo.SearchGroups(ctx, params)
}
