package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/modules/core/authzutil"
	"github.com/iota-uz/iota-sdk/pkg/authz"
)

func TestEnsureAuthz_ForbiddenJSONContract(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	allowed := ensureAuthz(rr, req, authz.ObjectName("core", "users"), "list", nil)

	require.False(t, allowed)
	require.Equal(t, http.StatusForbidden, rr.Code)

	var payload authzutil.ForbiddenPayload
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &payload))
	require.Equal(t, "core.users", payload.Object)
	require.Equal(t, "list", payload.Action)
	require.Equal(t, "/core/api/authz/requests", payload.RequestURL)
	require.NotEmpty(t, payload.DebugURL)
	require.Equal(t, "global", payload.Domain)
	require.NotEmpty(t, payload.Subject)
	require.NotEmpty(t, payload.MissingPolicies)

	state := authz.ViewStateFromContext(req.Context())
	require.NotNil(t, state)
	require.Equal(t, payload.Subject, state.Subject)
	require.Equal(t, payload.Domain, state.Tenant)
}
