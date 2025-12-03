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
	"github.com/iota-uz/iota-sdk/pkg/defaults"
	authzVersion "github.com/iota-uz/iota-sdk/pkg/authz/version"
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
	require.NotEqual(t, created.ID.String(), "")

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

func setupAuthzAPISuite(t *testing.T) *itf.Suite {
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
