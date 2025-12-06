package services

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/modules/core/authzutil"
	"github.com/iota-uz/iota-sdk/modules/core/testhelpers"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/serrors"
)

func TestAuthorizeLogging_ReturnsForbiddenWhenTenantMissing(t *testing.T) {
	setAuthzEnv(t)
	err := authorizeLogging(context.Background(), "view")
	require.Error(t, err)

	var serr *serrors.BaseError
	require.True(t, errors.As(err, &serr))
	require.Equal(t, "AUTHZ_FORBIDDEN", serr.Code)
}

func TestAuthorizeLogging_ReturnsForbiddenWhenUserMissing(t *testing.T) {
	setAuthzEnv(t)
	ctx := composables.WithTenantID(context.Background(), uuid.New())

	err := authorizeLogging(ctx, "view")
	require.Error(t, err)

	var serr *serrors.BaseError
	require.True(t, errors.As(err, &serr))
	require.Equal(t, "AUTHZ_FORBIDDEN", serr.Code)
}

func TestAuthorizeLogging_AllowsSystemSubject(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeDisabled)

	ctx := composables.WithTenantID(context.Background(), uuid.New())
	ctx = authzutil.WithSystemSubject(ctx, "system:logger")

	err := authorizeLogging(ctx, "view")
	require.NoError(t, err)
}

func setAuthzEnv(t *testing.T) {
	t.Helper()
	root := filepath.Clean("../../..")
	t.Setenv("AUTHZ_MODEL_PATH", filepath.Join(root, "config/access/model.conf"))
	t.Setenv("AUTHZ_POLICY_PATH", filepath.Join(root, "config/access/policy.csv"))
}
