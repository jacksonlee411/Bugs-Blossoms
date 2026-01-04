package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/modules/core/testhelpers"
	"github.com/iota-uz/iota-sdk/modules/org/domain/events"
	"github.com/iota-uz/iota-sdk/modules/org/infrastructure/persistence"
	orgsvc "github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

func TestOrgAPIController_Nodes_CreateUpdateMove_EmitsOutboxEvents(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeDisabled)

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
		"20251228120000_org_eliminate_effective_on_end_on.sql",
		"20251228140000_org_assignment_employment_status.sql",
		"20251228150000_org_gap_free_constraint_triggers.sql",
	})
	ensureOrgSettings(t, pool, tenantID)

	withOrgRolloutEnabled(t, tenantID)
	u := newTestOrgUser(tenantID)

	c := &OrgAPIController{
		org: orgsvc.NewOrgService(persistence.NewOrgRepository()),
	}

	asOf := "2025-01-01"

	// Create root.
	var rootID uuid.UUID
	{
		body := mustJSON(t, map[string]any{
			"code":           "D0000",
			"name":           "Company",
			"effective_date": asOf,
		})
		req := newOrgAPIRequestWithBody(t, http.MethodPost, "/org/api/nodes", tenantID, u, body)
		req = req.WithContext(composables.WithPool(req.Context(), pool))
		req.Header.Set("X-Request-ID", "req-org-crud-root")

		rr := httptest.NewRecorder()
		c.CreateNode(rr, req)
		require.Equal(t, http.StatusCreated, rr.Code, strings.TrimSpace(rr.Body.String()))

		var res struct {
			ID string `json:"id"`
		}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &res))
		rootID = uuid.MustParse(res.ID)
	}

	// Create child under root.
	var childID uuid.UUID
	{
		body := mustJSON(t, map[string]any{
			"code":           "D0001",
			"name":           "Dept A",
			"parent_id":      rootID,
			"effective_date": asOf,
		})
		req := newOrgAPIRequestWithBody(t, http.MethodPost, "/org/api/nodes", tenantID, u, body)
		req = req.WithContext(composables.WithPool(req.Context(), pool))
		req.Header.Set("X-Request-ID", "req-org-crud-child")

		rr := httptest.NewRecorder()
		c.CreateNode(rr, req)
		require.Equal(t, http.StatusCreated, rr.Code, strings.TrimSpace(rr.Body.String()))

		var res struct {
			ID string `json:"id"`
		}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &res))
		childID = uuid.MustParse(res.ID)
	}

	// Update child by introducing a new slice at a later effective_date.
	{
		body := mustJSON(t, map[string]any{
			"effective_date": "2025-02-01",
			"name":           "Dept A v2",
		})
		req := newOrgAPIRequestWithBody(t, http.MethodPatch, "/org/api/nodes/"+childID.String(), tenantID, u, body)
		req = req.WithContext(composables.WithPool(req.Context(), pool))
		req = mux.SetURLVars(req, map[string]string{"id": childID.String()})
		req.Header.Set("X-Request-ID", "req-org-crud-update")

		rr := httptest.NewRecorder()
		c.UpdateNode(rr, req)
		require.Equal(t, http.StatusOK, rr.Code, strings.TrimSpace(rr.Body.String()))
	}

	// Move child under a new parent node.
	var parentBID uuid.UUID
	{
		body := mustJSON(t, map[string]any{
			"code":           "D0002",
			"name":           "Dept B",
			"parent_id":      rootID,
			"effective_date": asOf,
		})
		req := newOrgAPIRequestWithBody(t, http.MethodPost, "/org/api/nodes", tenantID, u, body)
		req = req.WithContext(composables.WithPool(req.Context(), pool))
		req.Header.Set("X-Request-ID", "req-org-crud-parent-b")

		rr := httptest.NewRecorder()
		c.CreateNode(rr, req)
		require.Equal(t, http.StatusCreated, rr.Code, strings.TrimSpace(rr.Body.String()))

		var res struct {
			ID string `json:"id"`
		}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &res))
		parentBID = uuid.MustParse(res.ID)
	}
	{
		body := mustJSON(t, map[string]any{
			"effective_date": "2025-03-01",
			"new_parent_id":  parentBID,
		})
		req := newOrgAPIRequestWithBody(t, http.MethodPost, "/org/api/nodes/"+childID.String()+":move", tenantID, u, body)
		req = req.WithContext(composables.WithPool(req.Context(), pool))
		req = mux.SetURLVars(req, map[string]string{"id": childID.String()})
		req.Header.Set("X-Request-ID", "req-org-crud-move")

		rr := httptest.NewRecorder()
		c.MoveNode(rr, req)
		require.Equal(t, http.StatusOK, rr.Code, strings.TrimSpace(rr.Body.String()))
	}

	// Edge slice reflects move at the requested effective date.
	{
		var parentID uuid.UUID
		err := pool.QueryRow(
			context.Background(),
			`
SELECT parent_node_id
	FROM org_edges
	WHERE tenant_id=$1 AND hierarchy_type='OrgUnit' AND child_node_id=$2
	  AND effective_date <= $3::date AND end_date >= $3::date
	ORDER BY effective_date DESC
	LIMIT 1
	`,
			tenantID,
			childID,
			time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
		).Scan(&parentID)
		require.NoError(t, err)
		require.Equal(t, parentBID, parentID)
	}

	// Outbox payloads are valid OrgEventV1 with stable snake_case fields and topics.
	assertOutboxEvents(t, pool, tenantID, "req-org-crud-root", map[string]string{
		"org_node": "node.created",
		"org_edge": "edge.created",
	})
}

func assertOutboxEvents(tb testing.TB, pool *pgxpool.Pool, tenantID uuid.UUID, requestID string, expected map[string]string) {
	tb.Helper()

	type row struct {
		Topic   string
		Payload []byte
	}
	out := make([]row, 0, len(expected))
	rows, err := pool.Query(context.Background(), `
SELECT topic, payload
FROM org_outbox
WHERE tenant_id=$1 AND payload->>'request_id'=$2
ORDER BY sequence ASC
`, tenantID, requestID)
	require.NoError(tb, err)
	defer rows.Close()
	for rows.Next() {
		var r row
		require.NoError(tb, rows.Scan(&r.Topic, &r.Payload))
		out = append(out, r)
	}
	require.NoError(tb, rows.Err())

	got := map[string]string{}
	for _, r := range out {
		var ev events.OrgEventV1
		require.NoError(tb, json.Unmarshal(r.Payload, &ev))
		require.Equal(tb, events.EventVersionV1, ev.EventVersion)
		require.Equal(tb, tenantID, ev.TenantID)
		require.Equal(tb, requestID, ev.RequestID)

		wantChangeType, ok := expected[ev.EntityType]
		if !ok {
			continue
		}
		got[ev.EntityType] = ev.ChangeType
		require.Equal(tb, wantChangeType, ev.ChangeType)
		require.Equal(tb, events.TopicOrgChangedV1, r.Topic)
	}
	require.Equal(tb, expected, got)
}

func TestOrgAPIController_Assignments_Create_AutoPositionAndOutbox(t *testing.T) {
	setAuthzEnv(t)
	testhelpers.WithAuthzMode(t, authz.ModeDisabled)

	pool, tenantID := setupOrgTestDB(t, []string{
		"00001_org_baseline.sql",
		"00002_org_migration_smoke.sql",
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
		"20251228120000_org_eliminate_effective_on_end_on.sql",
		"20251228140000_org_assignment_employment_status.sql",
		"20251228150000_org_gap_free_constraint_triggers.sql",
		"20251230090000_org_job_architecture_workday_profiles.sql",
		"20251231120000_org_remove_job_family_allocation_percent.sql",
		"20260101020855_org_job_catalog_effective_dated_slices_phase_a.sql",
		"20260101020930_org_job_catalog_effective_dated_slices_gates_and_backfill.sql",
		"20260104100000_org_drop_job_profile_job_families_legacy.sql",
	})
	ensureOrgSettings(t, pool, tenantID)

	svc := orgsvc.NewOrgService(persistence.NewOrgRepository())

	// persons table is owned by Person migrations; for controller integration tests we only need the minimal schema contract.
	root := filepath.Clean("../../../../")
	personSQL := readGooseUpSQL(t, filepath.Join(root, "migrations", "person", "00001_person_baseline.sql"))
	_, err := pool.Exec(context.Background(), personSQL)
	require.NoError(t, err)

	personID := uuid.New()
	_, err = pool.Exec(context.Background(), `INSERT INTO persons (tenant_id, person_uuid, pernr, display_name) VALUES ($1,$2,$3,$4)`, tenantID, personID, "0001", "Test Person")
	require.NoError(t, err)

	asOfDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	svcCtx := composables.WithPool(context.Background(), pool)
	group, err := svc.CreateJobFamilyGroup(svcCtx, tenantID, orgsvc.JobFamilyGroupCreate{
		Code:          "AUTO-GROUP",
		Name:          "Auto Group",
		IsActive:      true,
		EffectiveDate: asOfDate,
	})
	require.NoError(t, err)
	family, err := svc.CreateJobFamily(svcCtx, tenantID, orgsvc.JobFamilyCreate{
		JobFamilyGroupID: group.ID,
		Code:             "AUTO-FAMILY",
		Name:             "Auto Family",
		IsActive:         true,
		EffectiveDate:    asOfDate,
	})
	require.NoError(t, err)
	_, err = svc.CreateJobProfile(svcCtx, tenantID, orgsvc.JobProfileCreate{
		Code:          "JP-AUTO",
		Name:          "Auto Job Profile",
		IsActive:      true,
		JobFamilies:   orgsvc.JobProfileJobFamiliesSet{Items: []orgsvc.JobProfileJobFamilySetItem{{JobFamilyID: family.ID, IsPrimary: true}}},
		EffectiveDate: asOfDate,
	})
	require.NoError(t, err)

	withOrgRolloutEnabled(t, tenantID)
	u := newTestOrgUser(tenantID)

	c := &OrgAPIController{
		org: svc,
	}

	// Create root node (required for auto-position path).
	asOf := "2025-01-01"
	var nodeID uuid.UUID
	{
		body := mustJSON(t, map[string]any{
			"code":           "D0000",
			"name":           "Company",
			"effective_date": asOf,
		})
		req := newOrgAPIRequestWithBody(t, http.MethodPost, "/org/api/nodes", tenantID, u, body)
		req = req.WithContext(composables.WithPool(req.Context(), pool))
		req.Header.Set("X-Request-ID", "req-org-assign-root")

		rr := httptest.NewRecorder()
		c.CreateNode(rr, req)
		require.Equal(t, http.StatusCreated, rr.Code, strings.TrimSpace(rr.Body.String()))

		var res struct {
			ID string `json:"id"`
		}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &res))
		nodeID = uuid.MustParse(res.ID)
	}

	// Create assignment and verify auto-position + outbox.
	{
		body := mustJSON(t, map[string]any{
			"pernr":          "0001",
			"effective_date": asOf,
			"org_node_id":    nodeID,
		})
		req := newOrgAPIRequestWithBody(t, http.MethodPost, "/org/api/assignments", tenantID, u, body)
		req = req.WithContext(composables.WithPool(req.Context(), pool))
		req.Header.Set("X-Request-ID", "req-org-assign-create")

		rr := httptest.NewRecorder()
		c.CreateAssignment(rr, req)
		require.Equal(t, http.StatusCreated, rr.Code, strings.TrimSpace(rr.Body.String()))

		var res struct {
			AssignmentID string `json:"assignment_id"`
			PositionID   string `json:"position_id"`
			SubjectID    string `json:"subject_id"`
		}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &res))
		require.NotEmpty(t, res.AssignmentID)
		require.NotEmpty(t, res.PositionID)
		require.Equal(t, personID.String(), res.SubjectID)
	}

	// Outbox payload fields are stable and topic routing matches entity type.
	assertOutboxAssignmentCreated(t, pool, tenantID, "req-org-assign-create")
}

func assertOutboxAssignmentCreated(tb testing.TB, pool *pgxpool.Pool, tenantID uuid.UUID, requestID string) {
	tb.Helper()

	var topic string
	var payload []byte
	err := pool.QueryRow(context.Background(), `
SELECT topic, payload
FROM org_outbox
WHERE tenant_id=$1 AND payload->>'request_id'=$2 AND payload->>'entity_type'='org_assignment'
ORDER BY sequence ASC
LIMIT 1
`, tenantID, requestID).Scan(&topic, &payload)
	require.NoError(tb, err)

	require.Equal(tb, events.TopicOrgAssignmentChangedV1, topic)

	var ev events.OrgEventV1
	require.NoError(tb, json.Unmarshal(payload, &ev))
	require.Equal(tb, events.EventVersionV1, ev.EventVersion)
	require.Equal(tb, tenantID, ev.TenantID)
	require.Equal(tb, requestID, ev.RequestID)
	require.Equal(tb, "org_assignment", ev.EntityType)
	require.Equal(tb, "assignment.created", ev.ChangeType)
}
