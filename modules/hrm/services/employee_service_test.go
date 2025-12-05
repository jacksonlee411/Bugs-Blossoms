package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/modules/hrm/domain/aggregates/employee"
	"github.com/iota-uz/iota-sdk/pkg/authz"
)

type mockEmployeeRepo struct {
	called bool
}

func (m *mockEmployeeRepo) mark() { m.called = true }
func (m *mockEmployeeRepo) Count(ctx context.Context) (int64, error) {
	m.mark()
	return 0, nil
}
func (m *mockEmployeeRepo) GetAll(ctx context.Context) ([]employee.Employee, error) {
	m.mark()
	return nil, nil
}
func (m *mockEmployeeRepo) GetPaginated(ctx context.Context, params *employee.FindParams) ([]employee.Employee, error) {
	m.mark()
	return nil, nil
}
func (m *mockEmployeeRepo) GetByID(ctx context.Context, id uint) (employee.Employee, error) {
	m.mark()
	return nil, nil
}
func (m *mockEmployeeRepo) Create(ctx context.Context, data employee.Employee) (employee.Employee, error) {
	m.mark()
	return nil, nil
}
func (m *mockEmployeeRepo) Update(ctx context.Context, data employee.Employee) error {
	m.mark()
	return nil
}
func (m *mockEmployeeRepo) Delete(ctx context.Context, id uint) error {
	m.mark()
	return nil
}

type stubPublisher struct{}

func (s *stubPublisher) Publish(args ...interface{})     {}
func (s *stubPublisher) Subscribe(handler interface{})   {}
func (s *stubPublisher) Unsubscribe(handler interface{}) {}
func (s *stubPublisher) Clear()                          {}
func (s *stubPublisher) SubscribersCount() int           { return 0 }

func TestEmployeeService_AuthorizeCreateDenied(t *testing.T) {
	t.Cleanup(func() { authorizeHRMFn = defaultAuthorizeHRM })

	repo := &mockEmployeeRepo{}
	svc := NewEmployeeService(repo, &stubPublisher{})

	authorizeHRMFn = func(ctx context.Context, object, action string, opts ...authz.RequestOption) error {
		require.Equal(t, EmployeesAuthzObject, object)
		require.Equal(t, "create", action)
		return errors.New("forbidden")
	}

	err := svc.Create(context.Background(), &employee.CreateDTO{})
	require.Error(t, err)
	require.False(t, repo.called, "repository should not be called when authorization fails")
}

func TestEmployeeService_AuthorizeUpdateDenied(t *testing.T) {
	t.Cleanup(func() { authorizeHRMFn = defaultAuthorizeHRM })

	repo := &mockEmployeeRepo{}
	svc := NewEmployeeService(repo, &stubPublisher{})

	authorizeHRMFn = func(ctx context.Context, object, action string, opts ...authz.RequestOption) error {
		require.Equal(t, EmployeesAuthzObject, object)
		require.Equal(t, "update", action)
		return errors.New("forbidden")
	}

	err := svc.Update(context.Background(), 1, &employee.UpdateDTO{})
	require.Error(t, err)
	require.False(t, repo.called, "repository should not be called when authorization fails")
}

func TestEmployeeService_AuthorizeDeleteDenied(t *testing.T) {
	t.Cleanup(func() { authorizeHRMFn = defaultAuthorizeHRM })

	repo := &mockEmployeeRepo{}
	svc := NewEmployeeService(repo, &stubPublisher{})

	authorizeHRMFn = func(ctx context.Context, object, action string, opts ...authz.RequestOption) error {
		require.Equal(t, EmployeesAuthzObject, object)
		require.Equal(t, "delete", action)
		return errors.New("forbidden")
	}

	_, err := svc.Delete(context.Background(), 1)
	require.Error(t, err)
	require.False(t, repo.called, "repository should not be called when authorization fails")
}
