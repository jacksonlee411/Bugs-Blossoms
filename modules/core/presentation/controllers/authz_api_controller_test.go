package controllers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/modules/core"
	"github.com/iota-uz/iota-sdk/modules/core/permissions"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/controllers"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/controllers/dtos"
	authzVersion "github.com/iota-uz/iota-sdk/pkg/authz/version"
	"github.com/iota-uz/iota-sdk/pkg/defaults"
	"github.com/iota-uz/iota-sdk/pkg/itf"
)

func TestAuthzAPIController_CreateApprove(t *testing.T) {
	suite := setupAuthzAPISuite(t)
	user := itf.User(
		permissions.AuthzRequestsWrite,
		permissions.AuthzRequestsRead,
		permissions.AuthzRequestsReview,
		permissions.AuthzDebug,
	)
	suite.AsUser(user)

	revision := currentPolicyRevision(t)
	payload := dtos.PolicyDraftRequest{
		Object:       "core.users",
		Action:       "read",
		Reason:       "integration test",
		Diff:         json.RawMessage(`[{"op":"add","path":"/p","value":["role:test","core.users","read","global","allow"]}]`),
		BaseRevision: revision,
	}
	resp := suite.POST("/core/api/authz/requests").JSON(payload).Expect(t).Status(http.StatusCreated)

	var created dtos.PolicyDraftResponse
	require.NoError(t, json.Unmarshal([]byte(resp.Body()), &created))
	require.NotEmpty(t, created.ID)

	suite.POST(fmt.Sprintf("/core/api/authz/requests/%s/approve", created.ID)).
		Expect(t).
		Status(http.StatusOK)

	listResp := suite.GET("/core/api/authz/requests").
		Expect(t).
		Status(http.StatusOK)
	require.Contains(t, listResp.Body(), "\"total\"")

	suite.GET("/core/api/authz/policies").
		Expect(t).
		Status(http.StatusOK)
}

func TestAuthzAPIController_Debug(t *testing.T) {
	suite := setupAuthzAPISuite(t)
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

func TestAuthzAPIController_StagePolicy(t *testing.T) {
	suite := setupAuthzAPISuite(t)
	user := itf.User(
		permissions.AuthzRequestsWrite,
		permissions.AuthzRequestsRead,
		permissions.AuthzDebug,
	)
	suite.AsUser(user)

	payload := dtos.StagePolicyRequest{
		Type:    "p",
		Subject: "role:test",
		Domain:  "global",
		Object:  "core.users",
		Action:  "read",
		Effect:  "allow",
	}

	resp := suite.POST("/core/api/authz/policies/stage").
		JSON(payload).
		Expect(t).
		Status(http.StatusCreated)

	require.Contains(t, resp.Body(), "\"total\":1")

	resp = suite.DELETE("/core/api/authz/policies/stage?id=" + extractStageID(t, resp.Body())).
		Expect(t).
		Status(http.StatusOK)
	require.Contains(t, resp.Body(), "\"total\":0")
}

func extractStageID(t *testing.T, body string) string {
	t.Helper()
	var parsed dtos.StagePolicyResponse
	require.NoError(t, json.Unmarshal([]byte(body), &parsed))
	require.NotEmpty(t, parsed.Data)
	return parsed.Data[0].ID
}

func setupAuthzAPISuite(t *testing.T) *itf.Suite {
	t.Helper()
	suite := itf.HTTP(t, core.NewModule(&core.ModuleOptions{
		PermissionSchema: defaults.PermissionSchema(),
	}))
	suite.Register(controllers.NewAuthzAPIController(suite.Env().App))
	return suite
}

func currentPolicyRevision(t *testing.T) string {
	t.Helper()
	provider := authzVersion.NewFileProvider("config/access/policy.csv.rev")
	meta, err := provider.Current(context.Background())
	require.NoError(t, err)
	return meta.Revision
}
