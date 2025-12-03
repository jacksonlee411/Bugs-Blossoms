package services_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	authzPersistence "github.com/iota-uz/iota-sdk/pkg/authz/persistence"
	authzVersion "github.com/iota-uz/iota-sdk/pkg/authz/version"
	"github.com/iota-uz/iota-sdk/pkg/itf"

	"github.com/iota-uz/iota-sdk/modules/core/services"
)

func TestPolicyDraftService_CreateApprove(t *testing.T) {
	t.Parallel()
	env := setupTest(t)
	service := newPolicyDraftService(t, env)

	ctx := env.Ctx
	tenantID := env.TenantID()

	draft, err := service.Create(ctx, tenantID, services.CreatePolicyDraftParams{
		RequesterID: uuid.New(),
		Object:      "core.users",
		Action:      "read",
		Reason:      "test request",
		Diff:        json.RawMessage(`[{"op":"add","path":"/p","value":["role:test","core.users","read","global","allow"]}]`),
	})
	require.NoError(t, err)
	require.Equal(t, authzPersistence.PolicyChangeStatusPendingReview, draft.Status)

	approved, err := service.Approve(ctx, tenantID, draft.ID, uuid.New())
	require.NoError(t, err)
	require.Equal(t, authzPersistence.PolicyChangeStatusApproved, approved.Status)

	list, total, err := service.List(ctx, tenantID, services.ListPolicyDraftsParams{})
	require.NoError(t, err)
	require.NotZero(t, total)
	require.NotEmpty(t, list)
}

func TestPolicyDraftService_RevisionMismatch(t *testing.T) {
	t.Parallel()
	env := setupTest(t)
	service := newPolicyDraftService(t, env)

	ctx := env.Ctx
	tenantID := env.TenantID()

	_, err := service.Create(ctx, tenantID, services.CreatePolicyDraftParams{
		RequesterID:  uuid.New(),
		Object:       "core.roles",
		Action:       "update",
		Reason:       "mismatch",
		Diff:         json.RawMessage(`{"op":"replace"}`),
		BaseRevision: "outdated",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, services.ErrRevisionMismatch)
}

func TestPolicyDraftService_Cancel(t *testing.T) {
	t.Parallel()
	env := setupTest(t)
	service := newPolicyDraftService(t, env)

	ctx := env.Ctx
	tenantID := env.TenantID()

	draft, err := service.Create(ctx, tenantID, services.CreatePolicyDraftParams{
		RequesterID: uuid.New(),
		Object:      "core.groups",
		Action:      "create",
		Diff:        json.RawMessage(`[{"op":"add"}]`),
	})
	require.NoError(t, err)

	cancelled, err := service.Cancel(ctx, tenantID, draft.ID)
	require.NoError(t, err)
	require.Equal(t, authzPersistence.PolicyChangeStatusCanceled, cancelled.Status)
}

func TestPolicyDraftService_Policies(t *testing.T) {
	t.Parallel()
	env := setupTest(t)
	service := newPolicyDraftService(t, env)

	entries, err := service.Policies(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, entries)
}

func newPolicyDraftService(t *testing.T, env *itf.TestEnvironment) *services.PolicyDraftService {
	t.Helper()
	repo := authzPersistence.NewPolicyChangeRequestRepository()
	provider := authzVersion.NewFileProvider("config/access/policy.csv.rev")
	return services.NewPolicyDraftService(
		repo,
		provider,
		"config/access/policy.csv",
		env.App.EventPublisher(),
	)
}
