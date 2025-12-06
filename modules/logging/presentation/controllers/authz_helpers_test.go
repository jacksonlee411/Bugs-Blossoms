package controllers

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/modules/core/domain/aggregates/user"
	"github.com/iota-uz/iota-sdk/modules/core/domain/value_objects/internet"
	"github.com/iota-uz/iota-sdk/modules/core/testhelpers"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/constants"
)

func TestEnsureLoggingAuthz_AllowsWhenModeDisabled(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeDisabled)

	tenantID := uuid.New()
	u := user.New(
		"Test",
		"User",
		internet.MustParseEmail("allow@example.com"),
		user.UILanguageEN,
		user.WithTenantID(tenantID),
		user.WithID(1),
	)

	ctx := contextWithLogger(t)
	ctx = composables.WithTenantID(ctx, tenantID)
	ctx = composables.WithUser(ctx, u)
	req := httptest.NewRequest("GET", "/logs", nil).WithContext(ctx)

	rr := httptest.NewRecorder()
	allowed := ensureLoggingAuthz(rr, req, "view")

	require.True(t, allowed)
	require.Equal(t, 200, rr.Code)
}

func TestEnsureLoggingAuthz_ForbidsWithoutUser(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeEnforce)

	ctx := contextWithLogger(t)
	ctx = composables.WithTenantID(ctx, uuid.New())
	req := httptest.NewRequest("GET", "/logs", nil).WithContext(ctx)
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	allowed := ensureLoggingAuthz(rr, req, "view")

	require.False(t, allowed)
	require.Equal(t, 403, rr.Code)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &payload))
	require.Equal(t, "forbidden", payload["error"])
	require.Equal(t, "logging.logs", payload["object"])
	require.Equal(t, "view", payload["action"])
	require.Equal(t, "/core/api/authz/requests", payload["request_url"])
	_, hasMissing := payload["missing_policies"]
	require.True(t, hasMissing)
}

func setAuthzEnv(t *testing.T) {
	t.Helper()
	root := filepath.Clean("../../../../")
	t.Setenv("AUTHZ_MODEL_PATH", filepath.Join(root, "config/access/model.conf"))
	t.Setenv("AUTHZ_POLICY_PATH", filepath.Join(root, "config/access/policy.csv"))
}

func contextWithLogger(t *testing.T) context.Context {
	t.Helper()
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)
	return context.WithValue(context.Background(), constants.LoggerKey, logrus.NewEntry(logger))
}
