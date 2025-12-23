package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/modules/core/testhelpers"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/pkg/eventbus"
)

func TestOrgAPIController_GetOpsHealth_HealthyOrDegraded(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeDisabled)

	pool, tenantID := setupOrgTestDB(t, []string{
		"00001_org_baseline.sql",
		"20251218150000_org_outbox.sql",
	})

	app := application.New(&application.ApplicationOptions{
		Pool:     pool,
		Bundle:   application.LoadBundle(),
		EventBus: eventbus.NewEventPublisher(configuration.Use().Logger()),
	})

	withOrgRolloutEnabled(t, tenantID)
	u := newTestOrgUser(tenantID)

	req := newOrgAPIRequest(t, http.MethodGet, "/org/api/ops/health", tenantID, u)
	req.Header.Set("X-Request-ID", "req-org-ops-health")

	rr := httptest.NewRecorder()
	c := &OrgAPIController{app: app}
	c.GetOpsHealth(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var resp struct {
		Status string                     `json:"status"`
		Checks map[string]json.RawMessage `json:"checks"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.NotEqual(t, string(orgHealthStatusDown), resp.Status)
	require.Contains(t, resp.Checks, "database")
	require.Contains(t, resp.Checks, "outbox")
	require.Contains(t, resp.Checks, "deep_read")
	require.Contains(t, resp.Checks, "cache")
}

func TestOrgAPIController_GetOpsHealth_DegradedWhenOutboxIsStuck(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeDisabled)

	pool, tenantID := setupOrgTestDB(t, []string{
		"00001_org_baseline.sql",
		"20251218150000_org_outbox.sql",
	})

	_, err := pool.Exec(context.Background(), `
INSERT INTO org_outbox (tenant_id, topic, payload, event_id, available_at)
VALUES ($1,'org.changed.v1','{}'::jsonb, gen_random_uuid(), $2)
`, tenantID, time.Now().UTC().Add(-10*time.Minute))
	require.NoError(t, err)

	app := application.New(&application.ApplicationOptions{
		Pool:     pool,
		Bundle:   application.LoadBundle(),
		EventBus: eventbus.NewEventPublisher(configuration.Use().Logger()),
	})

	withOrgRolloutEnabled(t, tenantID)
	u := newTestOrgUser(tenantID)

	req := newOrgAPIRequest(t, http.MethodGet, "/org/api/ops/health", tenantID, u)
	req.Header.Set("X-Request-ID", "req-org-ops-health-degraded")

	rr := httptest.NewRecorder()
	c := &OrgAPIController{app: app}
	c.GetOpsHealth(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var resp struct {
		Status string                     `json:"status"`
		Checks map[string]json.RawMessage `json:"checks"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Equal(t, string(orgHealthStatusDegraded), resp.Status)

	var outboxHealth orgComponentHealth
	require.NoError(t, json.Unmarshal(resp.Checks["outbox"], &outboxHealth))
	require.Equal(t, orgHealthStatusDegraded, outboxHealth.Status)
	require.NotEmpty(t, outboxHealth.Details["oldest_available_age"])
}
