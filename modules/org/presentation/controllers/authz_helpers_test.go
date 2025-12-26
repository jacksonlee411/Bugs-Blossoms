package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/gorilla/mux"
	"github.com/iota-uz/iota-sdk/modules/core/authzutil"
	coreuser "github.com/iota-uz/iota-sdk/modules/core/domain/aggregates/user"
	"github.com/iota-uz/iota-sdk/modules/core/domain/entities/session"
	"github.com/iota-uz/iota-sdk/modules/core/domain/value_objects/internet"
	"github.com/iota-uz/iota-sdk/modules/core/testhelpers"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
)

func TestEnsureOrgAuthz_ForbiddenJSONContract(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000054")
	req := httptest.NewRequest(http.MethodGet, "/org/api/positions", nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Request-ID", "req-org-json")

	rr := httptest.NewRecorder()
	allowed := ensureOrgAuthz(rr, req, tenantID, nil, orgPositionsAuthzObject, "read")

	require.False(t, allowed)
	require.Equal(t, http.StatusForbidden, rr.Code)

	var payload authzutil.ForbiddenPayload
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &payload))
	require.Equal(t, "forbidden", payload.Error)
	require.Equal(t, orgPositionsAuthzObject, payload.Object)
	require.Equal(t, "read", payload.Action)
	require.NotEmpty(t, payload.DebugURL)
	require.Equal(t, authz.DomainFromTenant(tenantID), payload.Domain)
	require.NotEmpty(t, payload.Subject)
	require.NotEmpty(t, payload.MissingPolicies)
	require.Equal(t, "req-org-json", payload.RequestID)

	state := authz.ViewStateFromContext(req.Context())
	require.NotNil(t, state)
	require.Equal(t, payload.Subject, state.Subject)
	require.Equal(t, payload.Domain, state.Tenant)
}

func TestEnsureOrgAuthz_EnforceModeDeniesWithoutPolicy(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeEnforce)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000054")
	u := coreuser.New(
		"Test",
		"User",
		internet.MustParseEmail("test@example.com"),
		coreuser.UILanguageEN,
		coreuser.WithID(1),
		coreuser.WithTenantID(tenantID),
	)

	req := httptest.NewRequest(http.MethodGet, "/org/api/positions", nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Request-ID", "req-org-enforce")

	rr := httptest.NewRecorder()
	allowed := ensureOrgAuthz(rr, req, tenantID, u, orgPositionsAuthzObject, "read")

	require.False(t, allowed)
	require.Equal(t, http.StatusForbidden, rr.Code)

	var payload authzutil.ForbiddenPayload
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &payload))
	require.Equal(t, "forbidden", payload.Error)
	require.Equal(t, orgPositionsAuthzObject, payload.Object)
	require.Equal(t, "read", payload.Action)
	require.Equal(t, authz.DomainFromTenant(tenantID), payload.Domain)
	require.Equal(t, "req-org-enforce", payload.RequestID)
	require.NotEmpty(t, payload.MissingPolicies)
	require.Equal(t, authz.DomainFromTenant(tenantID), payload.MissingPolicies[0].Domain)
	require.Equal(t, orgPositionsAuthzObject, payload.MissingPolicies[0].Object)
	require.Equal(t, "read", payload.MissingPolicies[0].Action)
}

func TestOrgAPIController_UpdatePosition_MapsToPositionsWrite(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeEnforce)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000054")
	withOrgRolloutEnabled(t, tenantID)

	u := coreuser.New(
		"Viewer",
		"User",
		internet.MustParseEmail("viewer@example.com"),
		coreuser.UILanguageEN,
		coreuser.WithID(1),
		coreuser.WithTenantID(tenantID),
	)

	withAuthzPolicy(t, []string{
		"p, role:org.staffing.viewer, org.positions, read, *, allow",
		"g, " + authzutil.SubjectForUser(tenantID, u) + ", role:org.staffing.viewer, " + authz.DomainFromTenant(tenantID),
	})

	req := newOrgAPIRequest(t, http.MethodPatch, "/org/api/positions/"+uuid.NewString(), tenantID, u)
	req = mux.SetURLVars(req, map[string]string{"id": uuid.NewString()})
	req.Header.Set("X-Request-ID", "req-org-update-write-deny")

	rr := httptest.NewRecorder()
	c := &OrgAPIController{}
	c.UpdatePosition(rr, req)

	require.Equal(t, http.StatusForbidden, rr.Code)

	var payload authzutil.ForbiddenPayload
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &payload))
	require.Equal(t, orgPositionsAuthzObject, payload.Object)
	require.Equal(t, "write", payload.Action)
	require.Equal(t, "req-org-update-write-deny", payload.RequestID)
	require.NotEmpty(t, payload.MissingPolicies)
	require.Equal(t, orgPositionsAuthzObject, payload.MissingPolicies[0].Object)
	require.Equal(t, "write", payload.MissingPolicies[0].Action)
}

func TestOrgAPIController_CorrectPosition_RequiresAdminEvenWithWrite(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeEnforce)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000054")
	withOrgRolloutEnabled(t, tenantID)

	u := coreuser.New(
		"Editor",
		"User",
		internet.MustParseEmail("editor@example.com"),
		coreuser.UILanguageEN,
		coreuser.WithID(2),
		coreuser.WithTenantID(tenantID),
	)

	withAuthzPolicy(t, []string{
		"p, role:org.staffing.editor, org.positions, write, *, allow",
		"g, " + authzutil.SubjectForUser(tenantID, u) + ", role:org.staffing.editor, " + authz.DomainFromTenant(tenantID),
	})

	// Sanity: write is allowed.
	rrAllow := httptest.NewRecorder()
	reqAllow := newOrgAPIRequest(t, http.MethodPatch, "/org/api/positions/"+uuid.NewString(), tenantID, u)
	allowed := ensureOrgAuthz(rrAllow, reqAllow, tenantID, u, orgPositionsAuthzObject, "write")
	require.True(t, allowed)
	require.Equal(t, http.StatusOK, rrAllow.Code)

	// But admin is denied for strong-governance endpoints.
	req := newOrgAPIRequest(t, http.MethodPost, "/org/api/positions/"+uuid.NewString()+":correct", tenantID, u)
	req = mux.SetURLVars(req, map[string]string{"id": uuid.NewString()})
	req.Header.Set("X-Request-ID", "req-org-correct-admin-deny")

	rr := httptest.NewRecorder()
	c := &OrgAPIController{}
	c.CorrectPosition(rr, req)

	require.Equal(t, http.StatusForbidden, rr.Code)

	var payload authzutil.ForbiddenPayload
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &payload))
	require.Equal(t, orgPositionsAuthzObject, payload.Object)
	require.Equal(t, "admin", payload.Action)
	require.Equal(t, "req-org-correct-admin-deny", payload.RequestID)
	require.NotEmpty(t, payload.MissingPolicies)
	require.Equal(t, orgPositionsAuthzObject, payload.MissingPolicies[0].Object)
	require.Equal(t, "admin", payload.MissingPolicies[0].Action)
}

func TestOrgAPIController_CorrectAssignment_RequiresAdminEvenWithAssign(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeEnforce)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000054")
	withOrgRolloutEnabled(t, tenantID)

	u := coreuser.New(
		"Assigner",
		"User",
		internet.MustParseEmail("assigner@example.com"),
		coreuser.UILanguageEN,
		coreuser.WithID(3),
		coreuser.WithTenantID(tenantID),
	)

	withAuthzPolicy(t, []string{
		"p, role:org.staffing.editor, org.assignments, assign, *, allow",
		"g, " + authzutil.SubjectForUser(tenantID, u) + ", role:org.staffing.editor, " + authz.DomainFromTenant(tenantID),
	})

	rrAllow := httptest.NewRecorder()
	reqAllow := newOrgAPIRequest(t, http.MethodPost, "/org/api/assignments", tenantID, u)
	allowed := ensureOrgAuthz(rrAllow, reqAllow, tenantID, u, orgAssignmentsAuthzObject, "assign")
	require.True(t, allowed)
	require.Equal(t, http.StatusOK, rrAllow.Code)

	req := newOrgAPIRequest(t, http.MethodPost, "/org/api/assignments/"+uuid.NewString()+":correct", tenantID, u)
	req = mux.SetURLVars(req, map[string]string{"id": uuid.NewString()})
	req.Header.Set("X-Request-ID", "req-org-assignment-admin-deny")

	rr := httptest.NewRecorder()
	c := &OrgAPIController{}
	c.CorrectAssignment(rr, req)

	require.Equal(t, http.StatusForbidden, rr.Code)

	var payload authzutil.ForbiddenPayload
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &payload))
	require.Equal(t, orgAssignmentsAuthzObject, payload.Object)
	require.Equal(t, "admin", payload.Action)
	require.Equal(t, "req-org-assignment-admin-deny", payload.RequestID)
	require.NotEmpty(t, payload.MissingPolicies)
	require.Equal(t, orgAssignmentsAuthzObject, payload.MissingPolicies[0].Object)
	require.Equal(t, "admin", payload.MissingPolicies[0].Action)
}

func TestOrgAPIController_StaffingReports_RequirePositionReportsRead(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeEnforce)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000054")
	withOrgRolloutEnabled(t, tenantID)

	u := coreuser.New(
		"Viewer",
		"User",
		internet.MustParseEmail("viewer@example.com"),
		coreuser.UILanguageEN,
		coreuser.WithID(10),
		coreuser.WithTenantID(tenantID),
	)

	withAuthzPolicy(t, []string{
		"p, role:org.staffing.viewer, org.positions, read, *, allow",
		"g, " + authzutil.SubjectForUser(tenantID, u) + ", role:org.staffing.viewer, " + authz.DomainFromTenant(tenantID),
	})

	cases := []struct {
		name string
		fn   func(rr *httptest.ResponseRecorder, req *http.Request)
		url  string
	}{
		{
			name: "summary",
			fn: func(rr *httptest.ResponseRecorder, req *http.Request) {
				c := &OrgAPIController{}
				c.GetStaffingSummary(rr, req)
			},
			url: "/org/api/reports/staffing:summary",
		},
		{
			name: "vacancies",
			fn: func(rr *httptest.ResponseRecorder, req *http.Request) {
				c := &OrgAPIController{}
				c.GetStaffingVacancies(rr, req)
			},
			url: "/org/api/reports/staffing:vacancies",
		},
		{
			name: "time_to_fill",
			fn: func(rr *httptest.ResponseRecorder, req *http.Request) {
				c := &OrgAPIController{}
				c.GetStaffingTimeToFill(rr, req)
			},
			url: "/org/api/reports/staffing:time-to-fill?from=2025-01-01&to=2025-02-01",
		},
		{
			name: "export",
			fn: func(rr *httptest.ResponseRecorder, req *http.Request) {
				c := &OrgAPIController{}
				c.ExportStaffingReport(rr, req)
			},
			url: "/org/api/reports/staffing:export?kind=summary&format=csv",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := newOrgAPIRequest(t, http.MethodGet, tc.url, tenantID, u)
			req.Header.Set("X-Request-ID", "req-org-reports-deny-"+tc.name)

			rr := httptest.NewRecorder()
			tc.fn(rr, req)

			require.Equal(t, http.StatusForbidden, rr.Code)

			var payload authzutil.ForbiddenPayload
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &payload))
			require.Equal(t, orgPositionReportsAuthzObject, payload.Object)
			require.Equal(t, "read", payload.Action)
			require.Equal(t, "req-org-reports-deny-"+tc.name, payload.RequestID)
			require.NotEmpty(t, payload.MissingPolicies)
			require.Equal(t, orgPositionReportsAuthzObject, payload.MissingPolicies[0].Object)
			require.Equal(t, "read", payload.MissingPolicies[0].Action)
		})
	}
}

func setAuthzEnv(t *testing.T) {
	t.Helper()

	root := filepath.Clean("../../../../")
	modelPath := filepath.Join(root, "config/access/model.conf")
	policyPath := filepath.Join(root, "config/access/policy.csv")

	t.Setenv("AUTHZ_MODEL_PATH", modelPath)
	t.Setenv("AUTHZ_POLICY_PATH", policyPath)

	cfg := configuration.Use()
	origModelPath := cfg.Authz.ModelPath
	origPolicyPath := cfg.Authz.PolicyPath
	cfg.Authz.ModelPath = modelPath
	cfg.Authz.PolicyPath = policyPath

	authz.Reset()
	authzutil.ResetRevisionProvider()

	t.Cleanup(func() {
		cfg.Authz.ModelPath = origModelPath
		cfg.Authz.PolicyPath = origPolicyPath
		authz.Reset()
		authzutil.ResetRevisionProvider()
	})
}

func withOrgRolloutEnabled(t *testing.T, tenantID uuid.UUID) {
	t.Helper()

	cfg := configuration.Use()
	prevMode := cfg.OrgRolloutMode
	prevTenants := cfg.OrgRolloutTenants
	cfg.OrgRolloutMode = "enabled"
	cfg.OrgRolloutTenants = tenantID.String()
	t.Cleanup(func() {
		cfg.OrgRolloutMode = prevMode
		cfg.OrgRolloutTenants = prevTenants
	})
}

func withAuthzPolicy(t *testing.T, lines []string) {
	t.Helper()

	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "policy.csv")
	content := []byte("")
	for _, line := range lines {
		content = append(content, []byte(line+"\n")...)
	}
	require.NoError(t, os.WriteFile(policyPath, content, 0o644))

	cfg := configuration.Use()
	origPolicyPath := cfg.Authz.PolicyPath
	cfg.Authz.PolicyPath = policyPath
	authz.Reset()
	authzutil.ResetRevisionProvider()

	t.Cleanup(func() {
		cfg.Authz.PolicyPath = origPolicyPath
		authz.Reset()
		authzutil.ResetRevisionProvider()
	})
}

func newOrgAPIRequest(t *testing.T, method, path string, tenantID uuid.UUID, u coreuser.User) *http.Request {
	t.Helper()

	ctx := context.Background()
	ctx = composables.WithSession(ctx, &session.Session{})
	ctx = composables.WithTenantID(ctx, tenantID)
	ctx = composables.WithUser(ctx, u)

	req := httptest.NewRequest(method, path, nil).WithContext(ctx)
	req.Header.Set("Accept", "application/json")
	return req
}
