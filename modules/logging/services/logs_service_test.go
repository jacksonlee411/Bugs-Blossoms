package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/modules/logging/domain/entities/actionlog"
	"github.com/iota-uz/iota-sdk/modules/logging/domain/entities/authenticationlog"
	"github.com/iota-uz/iota-sdk/pkg/authz"
)

type mockAuthLogRepo struct {
	calledList bool
}

func (m *mockAuthLogRepo) List(ctx context.Context, params *authenticationlog.FindParams) ([]*authenticationlog.AuthenticationLog, error) {
	m.calledList = true
	return []*authenticationlog.AuthenticationLog{}, nil
}

func (m *mockAuthLogRepo) Count(ctx context.Context, params *authenticationlog.FindParams) (int64, error) {
	return 0, nil
}

func (m *mockAuthLogRepo) Create(ctx context.Context, log *authenticationlog.AuthenticationLog) error {
	m.calledList = true
	return nil
}

type mockActionLogRepo struct {
	calledList bool
}

func (m *mockActionLogRepo) List(ctx context.Context, params *actionlog.FindParams) ([]*actionlog.ActionLog, error) {
	m.calledList = true
	return []*actionlog.ActionLog{}, nil
}

func (m *mockActionLogRepo) Count(ctx context.Context, params *actionlog.FindParams) (int64, error) {
	return 0, nil
}

func (m *mockActionLogRepo) Create(ctx context.Context, log *actionlog.ActionLog) error {
	m.calledList = true
	return nil
}

func TestLogsService_ListAuthenticationLogs_AuthorizeDenied(t *testing.T) {
	t.Cleanup(func() { authorizeLoggingFn = defaultAuthorizeLogging })

	authRepo := &mockAuthLogRepo{}
	actionRepo := &mockActionLogRepo{}
	svc := NewLogsService(authRepo, actionRepo)

	authorizeLoggingFn = func(ctx context.Context, action string, opts ...authz.RequestOption) error {
		require.Equal(t, "view", action)
		return errors.New("forbidden")
	}

	_, _, err := svc.ListAuthenticationLogs(context.Background(), &authenticationlog.FindParams{})
	require.Error(t, err)
	require.False(t, authRepo.calledList, "repository should not be invoked when authorization fails")
}

func TestLogsService_ListActionLogs_AuthorizeDenied(t *testing.T) {
	t.Cleanup(func() { authorizeLoggingFn = defaultAuthorizeLogging })

	authRepo := &mockAuthLogRepo{}
	actionRepo := &mockActionLogRepo{}
	svc := NewLogsService(authRepo, actionRepo)

	authorizeLoggingFn = func(ctx context.Context, action string, opts ...authz.RequestOption) error {
		require.Equal(t, "view", action)
		return errors.New("forbidden")
	}

	_, _, err := svc.ListActionLogs(context.Background(), &actionlog.FindParams{})
	require.Error(t, err)
	require.False(t, actionRepo.calledList, "repository should not be invoked when authorization fails")
}

func TestLogsService_CreateAuthenticationLog_ValidatesInput(t *testing.T) {
	svc := NewLogsService(&mockAuthLogRepo{}, &mockActionLogRepo{})
	err := svc.CreateAuthenticationLog(context.Background(), nil)
	require.Error(t, err)
}

func TestLogsService_CreateActionLog_ValidatesInput(t *testing.T) {
	svc := NewLogsService(&mockAuthLogRepo{}, &mockActionLogRepo{})
	err := svc.CreateActionLog(context.Background(), nil)
	require.Error(t, err)
}
