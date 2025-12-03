package persistence_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	authzPersistence "github.com/iota-uz/iota-sdk/pkg/authz/persistence"
)

func TestPolicyChangeRequestRepository_CreateAndGet(t *testing.T) {
	env := setupTest(t)
	repo := authzPersistence.NewPolicyChangeRequestRepository()

	req := newRequest(env.TenantID())
	require.NoError(t, repo.Create(env.Ctx, req))
	require.NotEqual(t, uuid.Nil, req.ID)

	found, err := repo.GetByID(env.Ctx, req.ID)
	require.NoError(t, err)
	require.Equal(t, req.ID, found.ID)
	require.Equal(t, req.Status, found.Status)
	require.Equal(t, req.Subject, found.Subject)
	require.Equal(t, req.RequesterID, found.RequesterID)
	require.Equal(t, req.TenantID, found.TenantID)
	require.Equal(t, req.BasePolicyRevision, found.BasePolicyRevision)
}

func TestPolicyChangeRequestRepository_ListAndCount(t *testing.T) {
	env := setupTest(t)
	repo := authzPersistence.NewPolicyChangeRequestRepository()

	first := newRequest(env.TenantID())
	second := newRequest(env.TenantID())
	second.Status = authzPersistence.PolicyChangeStatusPendingReview
	require.NoError(t, repo.Create(env.Ctx, first))
	require.NoError(t, repo.Create(env.Ctx, second))

	params := authzPersistence.FindParams{
		Statuses: []authzPersistence.PolicyChangeStatus{
			authzPersistence.PolicyChangeStatusPendingReview,
		},
	}
	list, total, err := repo.List(env.Ctx, params)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, list, 1)
	require.Equal(t, second.ID, list[0].ID)
}

func TestPolicyChangeRequestRepository_UpdateStatus(t *testing.T) {
	env := setupTest(t)
	repo := authzPersistence.NewPolicyChangeRequestRepository()

	req := newRequest(env.TenantID())
	require.NoError(t, repo.Create(env.Ctx, req))

	approver := uuid.New()
	reviewedAt := time.Date(2025, 1, 15, 11, 30, 0, 0, time.UTC)
	err := repo.UpdateStatus(env.Ctx, req.ID, authzPersistence.UpdateStatusParams{
		Status:     authzPersistence.PolicyChangeStatusApproved,
		ApproverID: authzPersistence.NewNullableValue(approver),
		ReviewedAt: authzPersistence.NewNullableValue(reviewedAt),
	})
	require.NoError(t, err)

	updated, err := repo.GetByID(env.Ctx, req.ID)
	require.NoError(t, err)
	require.Equal(t, authzPersistence.PolicyChangeStatusApproved, updated.Status)
	require.NotNil(t, updated.ApproverID)
	require.Equal(t, approver, *updated.ApproverID)
	require.NotNil(t, updated.ReviewedAt)
	require.WithinDuration(t, reviewedAt, *updated.ReviewedAt, time.Second)
}

func TestPolicyChangeRequestRepository_BotLockLifecycle(t *testing.T) {
	env := setupTest(t)
	repo := authzPersistence.NewPolicyChangeRequestRepository()

	req := newRequest(env.TenantID())
	require.NoError(t, repo.Create(env.Ctx, req))

	now := time.Now().UTC()
	acquired, err := repo.AcquireBotLock(env.Ctx, req.ID, authzPersistence.BotLockParams{
		Locker:      "worker-1",
		LockedAt:    now,
		StaleBefore: now.Add(-time.Minute),
	})
	require.NoError(t, err)
	require.True(t, acquired)

	// second acquire should fail while lock is fresh
	acquired, err = repo.AcquireBotLock(env.Ctx, req.ID, authzPersistence.BotLockParams{
		Locker:      "worker-2",
		LockedAt:    now.Add(time.Minute),
		StaleBefore: now,
	})
	require.NoError(t, err)
	require.False(t, acquired)

	require.NoError(t, repo.ReleaseBotLock(env.Ctx, req.ID, "worker-1"))

	acquired, err = repo.AcquireBotLock(env.Ctx, req.ID, authzPersistence.BotLockParams{
		Locker:      "worker-2",
		LockedAt:    now.Add(2 * time.Minute),
		StaleBefore: now,
	})
	require.NoError(t, err)
	require.True(t, acquired)

	require.NoError(t, repo.ForceReleaseBotLock(env.Ctx, req.ID))
}

func TestPolicyChangeRequestRepository_UpdateBotMetadata(t *testing.T) {
	env := setupTest(t)
	repo := authzPersistence.NewPolicyChangeRequestRepository()

	req := newRequest(env.TenantID())
	require.NoError(t, repo.Create(env.Ctx, req))

	prLink := "https://example.com/pr/1"
	jobID := "job-42"
	attempts := 3
	errorLog := "boom"
	appliedRev := "rev-1"
	appliedSnapshot := json.RawMessage(`["p", "foo"]`)

	err := repo.UpdateBotMetadata(env.Ctx, req.ID, authzPersistence.UpdateBotMetadataParams{
		BotJobID:              authzPersistence.NewNullableValue(jobID),
		BotAttempts:           authzPersistence.NewNullableValue(attempts),
		ErrorLog:              authzPersistence.NewNullableValue(errorLog),
		PRLink:                authzPersistence.NewNullableValue(prLink),
		AppliedPolicyRevision: authzPersistence.NewNullableValue(appliedRev),
		AppliedPolicySnapshot: authzPersistence.NewNullableValue(appliedSnapshot),
	})
	require.NoError(t, err)

	after, err := repo.GetByID(env.Ctx, req.ID)
	require.NoError(t, err)
	require.NotNil(t, after.BotJobID)
	require.Equal(t, jobID, *after.BotJobID)
	require.Equal(t, attempts, after.BotAttempts)
	require.NotNil(t, after.ErrorLog)
	require.Equal(t, errorLog, *after.ErrorLog)
	require.NotNil(t, after.PRLink)
	require.Equal(t, prLink, *after.PRLink)
	require.NotNil(t, after.AppliedPolicyRevision)
	require.Equal(t, appliedRev, *after.AppliedPolicyRevision)
	require.Equal(t, appliedSnapshot, after.AppliedPolicySnapshot)
}

func newRequest(tenantID uuid.UUID) *authzPersistence.PolicyChangeRequest {
	diff := json.RawMessage(`[{"op":"add"}]`)
	requester := uuid.New()
	return &authzPersistence.PolicyChangeRequest{
		Status:             authzPersistence.PolicyChangeStatusDraft,
		RequesterID:        requester,
		TenantID:           tenantID,
		Subject:            "role:admin",
		Domain:             "tenant",
		Action:             "update",
		Object:             "core.users",
		Reason:             "testing",
		Diff:               diff,
		BasePolicyRevision: "rev-0",
	}
}
