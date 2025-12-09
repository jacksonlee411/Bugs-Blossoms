package controllers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
		Diff:         json.RawMessage(`[{"op":"add","path":"/p/-","value":["role:test","global","core.users","read","allow"]}]`),
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

func TestAuthzAPIController_StagePolicy_Bulk(t *testing.T) {
	suite := setupAuthzAPISuite(t)
	user := itf.User(
		permissions.AuthzRequestsWrite,
		permissions.AuthzRequestsRead,
		permissions.AuthzDebug,
	)
	suite.AsUser(user)

	resp := suite.POST("/core/api/authz/policies/stage").
		JSON([]dtos.StagePolicyRequest{
			{
				Type:    "p",
				Subject: "role:test",
				Domain:  "global",
				Object:  "core.users",
				Action:  "read",
				Effect:  "allow",
			},
			{
				Type:      "g",
				Subject:   "tenant:00000000-0000-0000-0000-000000000001:user:bulk",
				Domain:    "00000000-0000-0000-0000-000000000001",
				Object:    "role:core.superadmin",
				Action:    "*",
				Effect:    "allow",
				StageKind: "add",
			},
		}).
		Expect(t).
		Status(http.StatusCreated)
	var staged dtos.StagePolicyResponse
	require.NoError(t, json.Unmarshal([]byte(resp.Body()), &staged))
	require.Len(t, staged.Data, 2)
}

func TestAuthzAPIController_CreateRequestFromStage(t *testing.T) {
	suite := setupAuthzAPISuite(t)
	user := itf.User(
		permissions.AuthzRequestsWrite,
		permissions.AuthzRequestsRead,
		permissions.AuthzDebug,
	)
	suite.AsUser(user)

	stagePayload := dtos.StagePolicyRequest{
		Type:    "p",
		Subject: "role:test",
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
	require.NotEmpty(t, staged.Data)
	require.Equal(t, "core.users", staged.Data[0].Object)

	requestPayload := dtos.PolicyDraftRequest{
		Reason: "from stage",
		Domain: "global",
	}
	resp := suite.POST("/core/api/authz/requests").
		JSON(requestPayload).
		Expect(t).
		Status(http.StatusCreated)
	var draft dtos.PolicyDraftResponse
	require.NoError(t, json.Unmarshal([]byte(resp.Body()), &draft))
	require.NotEmpty(t, draft.Diff)
	var patch []map[string]any
	require.NoError(t, json.Unmarshal(draft.Diff, &patch))
	require.NotEmpty(t, patch)
	require.Equal(t, "add", patch[0]["op"])
	path, _ := patch[0]["path"].(string)
	require.True(t, strings.HasPrefix(path, "/p/"))

	// stage should now be empty; submitting again without diff should fail
	suite.POST("/core/api/authz/requests").
		JSON(dtos.PolicyDraftRequest{}).
		Expect(t).
		Status(http.StatusBadRequest)
}

func TestAuthzAPIController_CreateRequestFromStage_Remove(t *testing.T) {
	suite := setupAuthzAPISuite(t)
	user := itf.User(
		permissions.AuthzRequestsWrite,
		permissions.AuthzRequestsRead,
		permissions.AuthzDebug,
	)
	suite.AsUser(user)

	stagePayload := dtos.StagePolicyRequest{
		Type:      "p",
		Subject:   "role:bot-dev-2",
		Domain:    "core.users",
		Object:    "list",
		Action:    "global",
		Effect:    "allow",
		StageKind: "remove",
	}
	suite.POST("/core/api/authz/policies/stage").
		JSON(stagePayload).
		Expect(t).
		Status(http.StatusCreated)

	resp := suite.POST("/core/api/authz/requests").
		JSON(dtos.PolicyDraftRequest{
			Domain: "core.users",
		}).
		Expect(t).
		Status(http.StatusCreated)

	var draft dtos.PolicyDraftResponse
	require.NoError(t, json.Unmarshal([]byte(resp.Body()), &draft))
	require.NotEmpty(t, draft.Diff)
	var patch []map[string]any
	require.NoError(t, json.Unmarshal(draft.Diff, &patch))
	require.Equal(t, "remove", patch[0]["op"])
	path, _ := patch[0]["path"].(string)
	require.True(t, strings.HasPrefix(path, "/p/"))
}

func TestAuthzAPIController_CreateRequest_HTMXForbiddenToast(t *testing.T) {
	suite := setupAuthzAPISuite(t)
	user := itf.User(permissions.AuthzDebug)
	suite.AsUser(user)

	payload := dtos.PolicyDraftRequest{
		Object: "core.users",
		Action: "read",
		Diff:   json.RawMessage(`[{"op":"add","path":"/p/-","value":["role:test","global","core.users","read","allow"]}]`),
	}
	suite.POST("/core/api/authz/requests").
		HTMX().
		JSON(payload).
		Assert(t).
		ExpectStatus(http.StatusForbidden).
		ExpectHTMXTrigger("notify")
}

func TestAuthzAPIController_StagePolicy_HTMXValidationToast(t *testing.T) {
	suite := setupAuthzAPISuite(t)
	user := itf.User(
		permissions.AuthzRequestsWrite,
		permissions.AuthzRequestsRead,
		permissions.AuthzDebug,
	)
	suite.AsUser(user)

	suite.POST("/core/api/authz/policies/stage").
		HTMX().
		JSON(dtos.StagePolicyRequest{
			Type:    "p",
			Subject: "role:test",
			Domain:  "global",
			Effect:  "allow",
		}).
		Assert(t).
		ExpectBadRequest().
		ExpectHTMXTrigger("notify")
}

func TestAuthzAPIController_CreateRequestFromStage_HTMXEmptyToast(t *testing.T) {
	suite := setupAuthzAPISuite(t)
	user := itf.User(
		permissions.AuthzRequestsWrite,
		permissions.AuthzRequestsRead,
		permissions.AuthzDebug,
	)
	suite.AsUser(user)

	suite.POST("/core/api/authz/requests").
		HTMX().
		JSON(dtos.PolicyDraftRequest{}).
		Assert(t).
		ExpectBadRequest().
		ExpectHTMXTrigger("notify")
}

func TestAuthzAPIController_RequestAccessEmptyDiff(t *testing.T) {
	suite := setupAuthzAPISuite(t)
	user := itf.User(
		permissions.AuthzRequestsWrite,
		permissions.AuthzRequestsRead,
		permissions.AuthzDebug,
	)
	suite.AsUser(user)

	suite.POST("/core/api/authz/requests").
		HTMX().
		FormFields(map[string]interface{}{
			"object":         "core.roles",
			"action":         "update",
			"domain":         "global",
			"diff":           "[]",
			"request_access": "1",
			"reason":         "请求编辑角色策略",
		}).
		Assert(t).
		ExpectCreated().
		ExpectHTMXTrigger("policies:staged").
		ExpectJSON().
		ExpectField("status", "pending_review")
}

func TestAuthzAPIController_RequestAccessMissingObject(t *testing.T) {
	suite := setupAuthzAPISuite(t)
	user := itf.User(
		permissions.AuthzRequestsWrite,
		permissions.AuthzRequestsRead,
		permissions.AuthzDebug,
	)
	suite.AsUser(user)

	suite.POST("/core/api/authz/requests").
		HTMX().
		FormFields(map[string]interface{}{
			"action":         "update",
			"domain":         "global",
			"diff":           "[]",
			"request_access": "1",
		}).
		Assert(t).
		ExpectBadRequest().
		ExpectHTMXTrigger("notify")
}

func TestAuthzAPIController_CreateRequest_BaseRevisionMismatch(t *testing.T) {
	suite := setupAuthzAPISuite(t)
	user := itf.User(
		permissions.AuthzRequestsWrite,
		permissions.AuthzRequestsRead,
		permissions.AuthzDebug,
	)
	suite.AsUser(user)

	suite.POST("/core/api/authz/requests").
		JSON(dtos.PolicyDraftRequest{
			Object:       "core.users",
			Action:       "read",
			Diff:         json.RawMessage(`[{"op":"add","path":"/p/-","value":["role:test","global","core.users","read","allow"]}]`),
			BaseRevision: "bogus",
		}).
		Expect(t).
		Status(http.StatusBadRequest)
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
