package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/modules/hrm/domain/entities/position"
	"github.com/iota-uz/iota-sdk/pkg/authz"
)

type mockPositionRepo struct {
	called bool
}

func (m *mockPositionRepo) mark() { m.called = true }
func (m *mockPositionRepo) Count(ctx context.Context) (int64, error) {
	m.mark()
	return 0, nil
}
func (m *mockPositionRepo) GetAll(ctx context.Context) ([]*position.Position, error) {
	m.mark()
	return nil, nil
}
func (m *mockPositionRepo) GetPaginated(ctx context.Context, params *position.FindParams) ([]*position.Position, error) {
	m.mark()
	return nil, nil
}
func (m *mockPositionRepo) GetByID(ctx context.Context, id int64) (*position.Position, error) {
	m.mark()
	return nil, nil
}
func (m *mockPositionRepo) Create(ctx context.Context, upload *position.Position) error {
	m.mark()
	return nil
}
func (m *mockPositionRepo) Update(ctx context.Context, upload *position.Position) error {
	m.mark()
	return nil
}
func (m *mockPositionRepo) Delete(ctx context.Context, id int64) error {
	m.mark()
	return nil
}

func TestPositionService_AuthorizeCreateDenied(t *testing.T) {
	t.Cleanup(func() { authorizeHRMFn = defaultAuthorizeHRM })

	repo := &mockPositionRepo{}
	svc := NewPositionService(repo, &stubPublisher{})

	authorizeHRMFn = func(ctx context.Context, object, action string, opts ...authz.RequestOption) error {
		require.Equal(t, PositionsAuthzObject, object)
		require.Equal(t, "create", action)
		return errors.New("forbidden")
	}

	err := svc.Create(context.Background(), &position.Position{})
	require.Error(t, err)
	require.False(t, repo.called, "repository should not be called when authorization fails")
}

func TestPositionService_AuthorizeUpdateDenied(t *testing.T) {
	t.Cleanup(func() { authorizeHRMFn = defaultAuthorizeHRM })

	repo := &mockPositionRepo{}
	svc := NewPositionService(repo, &stubPublisher{})

	authorizeHRMFn = func(ctx context.Context, object, action string, opts ...authz.RequestOption) error {
		require.Equal(t, PositionsAuthzObject, object)
		require.Equal(t, "update", action)
		return errors.New("forbidden")
	}

	err := svc.Update(context.Background(), &position.Position{})
	require.Error(t, err)
	require.False(t, repo.called, "repository should not be called when authorization fails")
}

func TestPositionService_AuthorizeDeleteDenied(t *testing.T) {
	t.Cleanup(func() { authorizeHRMFn = defaultAuthorizeHRM })

	repo := &mockPositionRepo{}
	svc := NewPositionService(repo, &stubPublisher{})

	authorizeHRMFn = func(ctx context.Context, object, action string, opts ...authz.RequestOption) error {
		require.Equal(t, PositionsAuthzObject, object)
		require.Equal(t, "delete", action)
		return errors.New("forbidden")
	}

	err := svc.Delete(context.Background(), 1)
	require.Error(t, err)
	require.False(t, repo.called, "repository should not be called when authorization fails")
}
