package controllers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/modules/core"
	"github.com/iota-uz/iota-sdk/modules/core/authzutil"
	"github.com/iota-uz/iota-sdk/modules/core/permissions"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/controllers"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/controllers/dtos"
	authz "github.com/iota-uz/iota-sdk/pkg/authz"
	authzVersion "github.com/iota-uz/iota-sdk/pkg/authz/version"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/pkg/defaults"
	"github.com/iota-uz/iota-sdk/pkg/itf"
)

func TestAuthzAPIController_Apply_FromStage(t *testing.T) {
	suite := setupAuthzAPISuiteWithTempPolicy(t)
	user := itf.User(
		permissions.AuthzRequestsWrite,
		permissions.AuthzDebug,
	)
	suite.AsUser(user)

	rev := currentPolicyRevision(t, configuration.Use().Authz.PolicyPath+".rev")

	stagePayload := dtos.StagePolicyRequest{
		Type:    "p",
		Subject: "role:test.apply",
		Domain:  "global",
		Object:  "core.users",
		Action:  "read",
		Effect:  "allow",
	}
	stageResp := suite.POST("/core/api/authz/policies/stage").
		JSON(stagePayload).
		Expect(t).
		Status(http.StatusCreated)

	var staged dtos.StagePolicyResponse
	require.NoError(t, json.Unmarshal([]byte(stageResp.Body()), &staged))
	require.Equal(t, 1, staged.Total)

	applyResp := suite.POST("/core/api/authz/policies/apply").
		Form(url.Values{
			"base_revision": []string{rev},
			"subject":       []string{"role:test.apply"},
			"domain":        []string{"global"},
			"reason":        []string{"e2e-test"},
		}).
		Expect(t).
		Status(http.StatusOK)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(applyResp.Body()), &result))
	require.Equal(t, rev, result["base_revision"])
	require.NotEmpty(t, result["revision"])
	require.NotEqual(t, rev, result["revision"])

	policyBytes, err := os.ReadFile(configuration.Use().Authz.PolicyPath)
	require.NoError(t, err)
	require.Contains(t, string(policyBytes), "p, role:test.apply, core.users, read, global, allow\n")
}

func TestAuthzAPIController_Apply_BaseRevisionMismatch(t *testing.T) {
	suite := setupAuthzAPISuiteWithTempPolicy(t)
	user := itf.User(
		permissions.AuthzRequestsWrite,
		permissions.AuthzDebug,
	)
	suite.AsUser(user)

	resp := suite.POST("/core/api/authz/policies/apply").
		JSON(map[string]any{
			"base_revision": "bogus",
			"subject":       "role:test.apply",
			"domain":        "global",
			"changes": []map[string]any{{
				"stage_kind": "add",
				"type":       "p",
				"subject":    "role:test.apply",
				"object":     "core.users",
				"action":     "read",
				"domain":     "global",
				"effect":     "allow",
			}},
		}).
		Expect(t).
		Status(http.StatusConflict)

	var payload dtos.APIError
	require.NoError(t, json.Unmarshal([]byte(resp.Body()), &payload))
	require.Equal(t, "AUTHZ_BASE_REVISION_MISMATCH", payload.Code)
	require.NotEmpty(t, payload.Meta["base_revision"])
}

func TestAuthzAPIController_StagePolicy_RejectsDeny(t *testing.T) {
	suite := setupAuthzAPISuiteWithTempPolicy(t)
	user := itf.User(
		permissions.AuthzRequestsWrite,
		permissions.AuthzDebug,
	)
	suite.AsUser(user)

	payload := dtos.StagePolicyRequest{
		Type:    "p",
		Subject: "role:test.deny",
		Domain:  "global",
		Object:  "core.users",
		Action:  "read",
		Effect:  "deny",
	}

	suite.POST("/core/api/authz/policies/stage").
		JSON(payload).
		Expect(t).
		Status(http.StatusBadRequest)
}

func TestAuthzAPIController_Debug(t *testing.T) {
	suite := setupAuthzAPISuiteWithTempPolicy(t)
	user := itf.User(permissions.AuthzDebug)
	suite.AsUser(user)

	resp := suite.GET("/core/api/authz/debug?subject=role:core.superadmin&domain=global&object=core.users&action=list").
		Expect(t).
		Status(http.StatusOK)

	var payload dtos.DebugResponse
	require.NoError(t, json.Unmarshal([]byte(resp.Body()), &payload))
	require.True(t, payload.Allowed)
	require.NotEmpty(t, payload.Trace.MatchedPolicy)
	require.GreaterOrEqual(t, payload.LatencyMillis, int64(0))
	require.Equal(t, "role:core.superadmin", payload.Request.Subject)
}

func setupAuthzAPISuiteWithTempPolicy(t *testing.T) *itf.Suite {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "../../../../"))
	srcPolicy := filepath.Join(root, "config/access/policy.csv")
	srcRev := filepath.Join(root, "config/access/policy.csv.rev")
	srcModel := filepath.Join(root, "config/access/model.conf")

	tmpDir := t.TempDir()
	dstPolicy := filepath.Join(tmpDir, "policy.csv")
	dstRev := filepath.Join(tmpDir, "policy.csv.rev")

	require.NoError(t, copyFile(srcPolicy, dstPolicy))
	require.NoError(t, copyFile(srcRev, dstRev))

	cfg := configuration.Use()
	origPolicy := cfg.Authz.PolicyPath
	origModel := cfg.Authz.ModelPath
	cfg.Authz.PolicyPath = dstPolicy
	cfg.Authz.ModelPath = srcModel

	authz.Reset()
	authzutil.ResetRevisionProvider()

	t.Cleanup(func() {
		cfg.Authz.PolicyPath = origPolicy
		cfg.Authz.ModelPath = origModel
		authz.Reset()
		authzutil.ResetRevisionProvider()
	})

	suite := itf.HTTP(t, core.NewModule(&core.ModuleOptions{
		PermissionSchema: defaults.PermissionSchema(),
	}))
	suite.Register(controllers.NewAuthzAPIController(suite.Env().App))
	return suite
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func currentPolicyRevision(t *testing.T, revPath string) string {
	t.Helper()
	provider := authzVersion.NewFileProvider(revPath)
	meta, err := provider.Current(context.Background())
	require.NoError(t, err)
	return meta.Revision
}
