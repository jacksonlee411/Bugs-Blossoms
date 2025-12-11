package authzutil

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

func TestBuildForbiddenPayload_WithStateAndHeaders(t *testing.T) {
	setAuthzPolicyPath(t)

	req := httptest.NewRequest(http.MethodGet, "/logs", nil)
	req.Header.Set("X-Request-ID", "req-123")
	tenantID := uuid.New()
	req = req.WithContext(composables.WithTenantID(req.Context(), tenantID))

	state := authz.NewViewState(authz.SubjectForUserID(tenantID, "user-1"), "logging")
	state.AddMissingPolicy(authz.MissingPolicy{Domain: "logging", Object: "logging.logs", Action: "view"})

	payload := BuildForbiddenPayload(req, state, "logging.logs", "view")

	require.Equal(t, "logging.logs", payload.Object)
	require.Equal(t, "view", payload.Action)
	require.Equal(t, "logging", payload.Domain)
	require.Equal(t, state.Subject, payload.Subject)
	require.Equal(t, "req-123", payload.RequestID)
	require.NotEmpty(t, payload.BaseRevision)
	require.NotEmpty(t, payload.DebugURL)
	require.Len(t, payload.MissingPolicies, 1)
	require.Len(t, payload.SuggestDiff, 1)
	require.Equal(t, payload.Domain, payload.MissingPolicies[0].Domain)
	require.Equal(t, "allow", payload.SuggestDiff[0].Effect)
}

func TestBuildForbiddenPayload_DefaultsWithoutState(t *testing.T) {
	setAuthzPolicyPath(t)

	tenantID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/core/users", nil)
	req.Header.Set("X-Request-ID", "fallback-req")
	req = req.WithContext(composables.WithTenantID(req.Context(), tenantID))

	payload := BuildForbiddenPayload(req, nil, "core.users", "list")

	require.Equal(t, "core.users", payload.Object)
	require.Equal(t, "list", payload.Action)
	require.Equal(t, authz.DomainFromTenant(tenantID), payload.Domain)
	require.Empty(t, payload.Subject)
	require.NotEmpty(t, payload.MissingPolicies)
	require.Equal(t, payload.Domain, payload.MissingPolicies[0].Domain)
	require.Empty(t, payload.SuggestDiff)
	require.Equal(t, "fallback-req", payload.RequestID)
	require.NotEmpty(t, payload.BaseRevision)
	require.Equal(t, "/core/api/authz/debug", payload.DebugURL)
}

func setAuthzPolicyPath(t *testing.T) {
	t.Helper()
	policyPath := filepath.Clean(filepath.Join("..", "..", "..", "config", "access", "policy.csv"))
	t.Setenv("AUTHZ_POLICY_PATH", policyPath)
}
