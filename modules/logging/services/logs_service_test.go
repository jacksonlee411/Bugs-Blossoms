package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/modules/core/domain/aggregates/user"
	"github.com/iota-uz/iota-sdk/modules/core/domain/value_objects/internet"
	"github.com/iota-uz/iota-sdk/modules/logging/domain/entities/actionlog"
	"github.com/iota-uz/iota-sdk/modules/logging/domain/entities/authenticationlog"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

type mockAuthLogRepo struct {
	calledList bool
	lastParams *authenticationlog.FindParams
}

func (m *mockAuthLogRepo) List(ctx context.Context, params *authenticationlog.FindParams) ([]*authenticationlog.AuthenticationLog, error) {
	m.calledList = true
	m.lastParams = params
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
	lastParams *actionlog.FindParams
}

func (m *mockActionLogRepo) List(ctx context.Context, params *actionlog.FindParams) ([]*actionlog.ActionLog, error) {
	m.calledList = true
	m.lastParams = params
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

func TestLogsService_ListAuthenticationLogs_Authorized(t *testing.T) {
	t.Cleanup(func() { authorizeLoggingFn = defaultAuthorizeLogging })

	authRepo := &mockAuthLogRepo{}
	actionRepo := &mockActionLogRepo{}
	svc := NewLogsService(authRepo, actionRepo)

	authorizeLoggingFn = func(ctx context.Context, action string, opts ...authz.RequestOption) error {
		require.Equal(t, "view", action)
		return nil
	}

	logs, total, err := svc.ListAuthenticationLogs(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, int64(0), total)
	require.Empty(t, logs)
	require.True(t, authRepo.calledList, "repository should be invoked when authorized")
	require.NotNil(t, authRepo.lastParams, "params should default to non-nil value")
}

func TestLogsService_ListActionLogs_Authorized(t *testing.T) {
	t.Cleanup(func() { authorizeLoggingFn = defaultAuthorizeLogging })

	authRepo := &mockAuthLogRepo{}
	actionRepo := &mockActionLogRepo{}
	svc := NewLogsService(authRepo, actionRepo)

	authorizeLoggingFn = func(ctx context.Context, action string, opts ...authz.RequestOption) error {
		require.Equal(t, "view", action)
		return nil
	}

	logs, total, err := svc.ListActionLogs(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, int64(0), total)
	require.Empty(t, logs)
	require.True(t, actionRepo.calledList, "repository should be invoked when authorized")
	require.NotNil(t, actionRepo.lastParams, "params should default to non-nil value")
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

func TestLogsService_ListAuthenticationLogs_MissingTenantOrUser(t *testing.T) {
	t.Cleanup(func() { authorizeLoggingFn = defaultAuthorizeLogging })

	authRepo := &mockAuthLogRepo{}
	actionRepo := &mockActionLogRepo{}
	svc := NewLogsService(authRepo, actionRepo)

	// Missing tenant
	_, _, err := svc.ListAuthenticationLogs(context.Background(), nil)
	require.Error(t, err)
	require.False(t, authRepo.calledList)

	// Tenant provided, but no user in context
	ctx := composables.WithTenantID(context.Background(), uuid.New())
	_, _, err = svc.ListAuthenticationLogs(ctx, nil)
	require.Error(t, err)
	require.False(t, authRepo.calledList)
}

func TestLogsService_ListActionLogs_StopsWithoutUser(t *testing.T) {
	t.Cleanup(func() { authorizeLoggingFn = defaultAuthorizeLogging })

	authRepo := &mockAuthLogRepo{}
	actionRepo := &mockActionLogRepo{}
	svc := NewLogsService(authRepo, actionRepo)

	ctx := composables.WithTenantID(context.Background(), uuid.New())
	_, _, err := svc.ListActionLogs(ctx, nil)
	require.Error(t, err)
	require.False(t, actionRepo.calledList)
}

func TestLogsService_ListActionLogs_AllowsWithUserAndTenant(t *testing.T) {
	t.Cleanup(func() { authorizeLoggingFn = defaultAuthorizeLogging })

	authRepo := &mockAuthLogRepo{}
	actionRepo := &mockActionLogRepo{}
	svc := NewLogsService(authRepo, actionRepo)

	tenantID := uuid.New()
	ctx := composables.WithTenantID(context.Background(), tenantID)
	u := user.New("Test", "User", internet.MustParseEmail("logs@example.com"), user.UILanguageEN, user.WithTenantID(tenantID))
	ctx = composables.WithUser(ctx, u)

	authorizeLoggingFn = func(ctx context.Context, action string, opts ...authz.RequestOption) error {
		require.Equal(t, "view", action)
		return nil
	}

	_, _, err := svc.ListActionLogs(ctx, nil)
	require.NoError(t, err)
	require.True(t, actionRepo.calledList)
}
