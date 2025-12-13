package controllers_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/modules/core"
	"github.com/iota-uz/iota-sdk/modules/core/permissions"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/controllers"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/controllers/dtos"
	"github.com/iota-uz/iota-sdk/pkg/defaults"
	"github.com/iota-uz/iota-sdk/pkg/itf"
)

func TestAuthzRequestController_Get(t *testing.T) {
	suite := itf.HTTP(t, core.NewModule(&core.ModuleOptions{
		PermissionSchema: defaults.PermissionSchema(),
	}))
	suite.Register(controllers.NewAuthzAPIController(suite.Env().App))
	suite.Register(controllers.NewAuthzRequestController(suite.Env().App))

	user := itf.User(
		permissions.AuthzRequestsWrite,
		permissions.AuthzRequestsRead,
		permissions.AuthzDebug,
	)
	suite.AsUser(user)

	revision := currentPolicyRevision(t)
	payload := dtos.PolicyDraftRequest{
		Object:       "core.users",
		Action:       "read",
		Reason:       "ui integration test",
		Diff:         json.RawMessage(`[{"op":"add","path":"/p/-","value":["role:test","global","core.users","read","allow"]}]`),
		BaseRevision: revision,
	}
	resp := suite.POST("/core/api/authz/requests").
		JSON(payload).
		Expect(t).
		Status(201)

	var created dtos.PolicyDraftResponse
	require.NoError(t, json.Unmarshal([]byte(resp.Body()), &created))
	require.NotEmpty(t, created.ID)

	detail := suite.GET(fmt.Sprintf("/core/authz/requests/%s", created.ID)).
		Expect(t).
		Status(200)

	body := detail.Body()
	require.Contains(t, body, "Policy Change Request")
	require.Contains(t, body, created.ID.String())
	require.Contains(t, body, "Awaiting review")
}

func TestAuthzRequestController_List(t *testing.T) {
	suite := itf.HTTP(t, core.NewModule(&core.ModuleOptions{
		PermissionSchema: defaults.PermissionSchema(),
	}))
	suite.Register(controllers.NewAuthzAPIController(suite.Env().App))
	suite.Register(controllers.NewAuthzRequestController(suite.Env().App))

	user := itf.User(
		permissions.AuthzRequestsWrite,
		permissions.AuthzRequestsRead,
		permissions.AuthzDebug,
	)
	suite.AsUser(user)

	revision := currentPolicyRevision(t)
	payload := dtos.PolicyDraftRequest{
		Object:       "core.users",
		Action:       "read",
		Reason:       "ui integration test list",
		Diff:         json.RawMessage(`[{"op":"add","path":"/p/-","value":["role:test","global","core.users","read","allow"]}]`),
		BaseRevision: revision,
	}
	resp := suite.POST("/core/api/authz/requests").
		JSON(payload).
		Expect(t).
		Status(201)

	var created dtos.PolicyDraftResponse
	require.NoError(t, json.Unmarshal([]byte(resp.Body()), &created))
	require.NotEmpty(t, created.ID)

	list := suite.GET("/core/authz/requests").
		Expect(t).
		Status(200)

	body := list.Body()
	require.Contains(t, body, "Policy change requests")
	require.Contains(t, body, created.ID.String())
}
