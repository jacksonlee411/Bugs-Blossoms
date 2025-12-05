package services

import (
	"context"
	"testing"

	"github.com/iota-uz/iota-sdk/modules/core/infrastructure/query"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/viewmodels"
	"github.com/stretchr/testify/require"
)

type stubUserQueryRepo struct {
	called bool
}

func (s *stubUserQueryRepo) FindUsers(ctx context.Context, params *query.FindParams) ([]*viewmodels.User, int, error) {
	s.called = true
	return []*viewmodels.User{}, 0, nil
}

func (s *stubUserQueryRepo) FindUserByID(ctx context.Context, userID int) (*viewmodels.User, error) {
	s.called = true
	return nil, nil
}

func (s *stubUserQueryRepo) SearchUsers(ctx context.Context, params *query.FindParams) ([]*viewmodels.User, int, error) {
	s.called = true
	return []*viewmodels.User{}, 0, nil
}

func (s *stubUserQueryRepo) FindUsersWithRoles(ctx context.Context, params *query.FindParams) ([]*viewmodels.User, int, error) {
	s.called = true
	return []*viewmodels.User{}, 0, nil
}

func TestUserQueryService_FindUsers_NoUserInContext(t *testing.T) {
	t.Parallel()

	repo := &stubUserQueryRepo{}
	svc := NewUserQueryService(repo)

	_, _, err := svc.FindUsers(context.Background(), &query.FindParams{})
	require.NoError(t, err)
	require.True(t, repo.called, "repository should still be invoked without authenticated user")
}
