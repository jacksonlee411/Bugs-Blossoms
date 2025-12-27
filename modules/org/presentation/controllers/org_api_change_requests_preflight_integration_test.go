package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"

	coredtos "github.com/iota-uz/iota-sdk/modules/core/presentation/controllers/dtos"
	"github.com/iota-uz/iota-sdk/modules/core/testhelpers"
	"github.com/iota-uz/iota-sdk/modules/org/infrastructure/persistence"
	orgsvc "github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
)

func TestOrgAPIController_ChangeRequests_RequestIDIsIdempotent(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeDisabled)
	withOrgChangeRequestsEnabled(t)

	pool, tenantID := setupOrgTestDB(t, []string{
		"00001_org_baseline.sql",
		"20251218005114_org_placeholders_and_event_contracts.sql",
	})

	withOrgRolloutEnabled(t, tenantID)
	u := newTestOrgUser(tenantID)

	c := &OrgAPIController{
		changeRequests: orgsvc.NewChangeRequestService(persistence.NewChangeRequestRepository()),
	}

	payload := map[string]any{
		"effective_date": "2025-01-01T00:00:00Z",
		"commands": []any{
			map[string]any{
				"type":    "node.create",
				"payload": map[string]any{"code": "ROOT", "name": "Company"},
			},
		},
	}
	body := mustJSON(t, map[string]any{
		"payload": payload,
	})

	req1 := newOrgAPIRequestWithBody(t, http.MethodPost, "/org/api/change-requests", tenantID, u, body)
	req1 = req1.WithContext(composables.WithPool(req1.Context(), pool))
	req1.Header.Set("X-Request-ID", "req-org-cr-idempotent")

	rr1 := httptest.NewRecorder()
	c.CreateChangeRequest(rr1, req1)
	require.Equal(t, http.StatusCreated, rr1.Code)

	var created1 changeRequestSummaryResponse
	require.NoError(t, json.Unmarshal(rr1.Body.Bytes(), &created1))
	require.NotEmpty(t, created1.ID)
	require.Equal(t, "req-org-cr-idempotent", created1.RequestID)
	require.Equal(t, "draft", created1.Status)

	req2 := newOrgAPIRequestWithBody(t, http.MethodPost, "/org/api/change-requests", tenantID, u, body)
	req2 = req2.WithContext(composables.WithPool(req2.Context(), pool))
	req2.Header.Set("X-Request-ID", "req-org-cr-idempotent")

	rr2 := httptest.NewRecorder()
	c.CreateChangeRequest(rr2, req2)
	require.Equal(t, http.StatusCreated, rr2.Code)

	var created2 changeRequestSummaryResponse
	require.NoError(t, json.Unmarshal(rr2.Body.Bytes(), &created2))
	require.Equal(t, created1.ID, created2.ID)
	require.Equal(t, "req-org-cr-idempotent", created2.RequestID)
	require.Equal(t, "draft", created2.Status)
}

func TestOrgAPIController_ChangeRequests_ImmutableAfterSubmit(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeDisabled)
	withOrgChangeRequestsEnabled(t)

	pool, tenantID := setupOrgTestDB(t, []string{
		"00001_org_baseline.sql",
		"20251218005114_org_placeholders_and_event_contracts.sql",
	})

	withOrgRolloutEnabled(t, tenantID)
	u := newTestOrgUser(tenantID)

	c := &OrgAPIController{
		changeRequests: orgsvc.NewChangeRequestService(persistence.NewChangeRequestRepository()),
	}

	payload := map[string]any{
		"effective_date": "2025-01-01T00:00:00Z",
		"commands": []any{
			map[string]any{
				"type":    "node.create",
				"payload": map[string]any{"code": "ROOT", "name": "Company"},
			},
		},
	}
	body := mustJSON(t, map[string]any{
		"payload": payload,
	})

	createReq := newOrgAPIRequestWithBody(t, http.MethodPost, "/org/api/change-requests", tenantID, u, body)
	createReq = createReq.WithContext(composables.WithPool(createReq.Context(), pool))
	createReq.Header.Set("X-Request-ID", "req-org-cr-immutable")

	createRR := httptest.NewRecorder()
	c.CreateChangeRequest(createRR, createReq)
	require.Equal(t, http.StatusCreated, createRR.Code)

	var created changeRequestSummaryResponse
	require.NoError(t, json.Unmarshal(createRR.Body.Bytes(), &created))
	crID := uuid.MustParse(created.ID)

	submitReq := newOrgAPIRequest(t, http.MethodPost, "/org/api/change-requests/"+created.ID+":submit", tenantID, u)
	submitReq = submitReq.WithContext(composables.WithPool(submitReq.Context(), pool))
	submitReq = mux.SetURLVars(submitReq, map[string]string{"id": crID.String()})
	submitReq.Header.Set("X-Request-ID", "req-org-cr-submit")

	submitRR := httptest.NewRecorder()
	c.SubmitChangeRequest(submitRR, submitReq)
	require.Equal(t, http.StatusOK, submitRR.Code)

	var submitted changeRequestSummaryResponse
	require.NoError(t, json.Unmarshal(submitRR.Body.Bytes(), &submitted))
	require.Equal(t, "submitted", submitted.Status)

	againReq := newOrgAPIRequestWithBody(t, http.MethodPost, "/org/api/change-requests", tenantID, u, body)
	againReq = againReq.WithContext(composables.WithPool(againReq.Context(), pool))
	againReq.Header.Set("X-Request-ID", "req-org-cr-immutable")

	againRR := httptest.NewRecorder()
	c.CreateChangeRequest(againRR, againReq)
	require.Equal(t, http.StatusConflict, againRR.Code)

	var apiErr coredtos.APIError
	require.NoError(t, json.Unmarshal(againRR.Body.Bytes(), &apiErr))
	require.Equal(t, "ORG_CHANGE_REQUEST_IMMUTABLE", apiErr.Code)
}

func TestOrgAPIController_Preflight_TooManyMovesReturnsTooLarge(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeDisabled)
	withOrgPreflightEnabled(t)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	withOrgRolloutEnabled(t, tenantID)
	u := newTestOrgUser(tenantID)

	cmds := make([]any, 0, 11)
	for i := 0; i < 11; i++ {
		cmds = append(cmds, map[string]any{"type": "node.move"})
	}
	body := mustJSON(t, map[string]any{
		"commands": cmds,
	})

	req := newOrgAPIRequestWithBody(t, http.MethodPost, "/org/api/preflight", tenantID, u, body)
	req.Header.Set("X-Request-ID", "req-org-preflight-too-many-moves")

	rr := httptest.NewRecorder()
	c := &OrgAPIController{}
	c.Preflight(rr, req)
	require.Equal(t, http.StatusUnprocessableEntity, rr.Code)

	var apiErr coredtos.APIError
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &apiErr))
	require.Equal(t, "ORG_PREFLIGHT_TOO_LARGE", apiErr.Code)
}

func TestOrgAPIController_Preflight_InvalidCommandReturnsCommandIndexMeta(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeDisabled)
	withOrgPreflightEnabled(t)

	pool, tenantID := setupOrgTestDB(t, []string{
		"00001_org_baseline.sql",
	})

	withOrgRolloutEnabled(t, tenantID)
	u := newTestOrgUser(tenantID)

	body := mustJSON(t, map[string]any{
		"effective_date": "2025-01-01T00:00:00Z",
		"commands": []any{
			map[string]any{
				"type": "node.create",
			},
		},
	})

	req := newOrgAPIRequestWithBody(t, http.MethodPost, "/org/api/preflight", tenantID, u, body)
	req = req.WithContext(composables.WithPool(req.Context(), pool))
	req.Header.Set("X-Request-ID", "req-org-preflight-invalid")

	rr := httptest.NewRecorder()
	c := &OrgAPIController{}
	c.Preflight(rr, req)
	require.Equal(t, http.StatusUnprocessableEntity, rr.Code)

	var apiErr coredtos.APIError
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &apiErr))
	require.Equal(t, "ORG_PREFLIGHT_INVALID_COMMAND", apiErr.Code)
	require.Equal(t, "0", apiErr.Meta["command_index"])
	require.Equal(t, "node.create", apiErr.Meta["command_type"])
	require.Equal(t, "req-org-preflight-invalid", apiErr.Meta["request_id"])
}

func TestOrgAPIController_Preflight_SuccessHasNoSideEffects(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeDisabled)
	withOrgPreflightEnabled(t)

	pool, tenantID := setupOrgTestDB(t, []string{
		"00001_org_baseline.sql",
		"20251218005114_org_placeholders_and_event_contracts.sql",
		"20251218130000_org_settings_and_audit.sql",
		"20251218150000_org_outbox.sql",
		"20251219090000_org_hierarchy_closure_and_snapshots.sql",
		"20251219195000_org_security_group_mappings_and_links.sql",
		"20251219220000_org_reporting_nodes_and_view.sql",
		"20251220160000_org_position_slices_and_fte.sql",
		"20251220200000_org_job_catalog_profiles_and_validation_modes.sql",
		"20251221090000_org_reason_code_mode.sql",
		"20251222120000_org_personnel_events.sql",
		"20251227090000_org_valid_time_day_granularity.sql",
	})
	ensureOrgSettings(t, pool, tenantID)

	withOrgRolloutEnabled(t, tenantID)
	u := newTestOrgUser(tenantID)

	c := &OrgAPIController{
		org: orgsvc.NewOrgService(persistence.NewOrgRepository()),
	}

	body := mustJSON(t, map[string]any{
		"effective_date": "2025-01-01T00:00:00Z",
		"commands": []any{
			map[string]any{
				"type":    "node.create",
				"payload": map[string]any{"code": "ROOT", "name": "Company"},
			},
		},
	})

	req := newOrgAPIRequestWithBody(t, http.MethodPost, "/org/api/preflight", tenantID, u, body)
	req = req.WithContext(composables.WithPool(req.Context(), pool))
	req.Header.Set("X-Request-ID", "req-org-preflight-ok")

	rr := httptest.NewRecorder()
	c.Preflight(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resp struct {
		CommandsCount int `json:"commands_count"`
		Impact        struct {
			OrgNodes struct {
				Create int `json:"create"`
			} `json:"org_nodes"`
		} `json:"impact"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Equal(t, 1, resp.CommandsCount)
	require.Equal(t, 1, resp.Impact.OrgNodes.Create)

	ctx := context.Background()
	require.Equal(t, int64(0), countRows(t, ctx, pool, "org_nodes", tenantID))
	require.Equal(t, int64(0), countRows(t, ctx, pool, "org_edges", tenantID))
	require.Equal(t, int64(0), countRows(t, ctx, pool, "org_node_slices", tenantID))
	require.Equal(t, int64(0), countRows(t, ctx, pool, "org_audit_logs", tenantID))
}

func withOrgChangeRequestsEnabled(t *testing.T) {
	t.Helper()

	cfg := configuration.Use()
	prev := cfg.OrgChangeRequestsEnabled
	cfg.OrgChangeRequestsEnabled = true
	t.Cleanup(func() {
		cfg.OrgChangeRequestsEnabled = prev
	})
}

func withOrgPreflightEnabled(t *testing.T) {
	t.Helper()

	cfg := configuration.Use()
	prev := cfg.OrgPreflightEnabled
	cfg.OrgPreflightEnabled = true
	t.Cleanup(func() {
		cfg.OrgPreflightEnabled = prev
	})
}
