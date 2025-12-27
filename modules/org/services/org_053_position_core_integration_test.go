package services_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/modules/org/infrastructure/persistence"
	orgsvc "github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/itf"
)

func setupOrg053DB(tb testing.TB) (context.Context, *pgxpool.Pool, uuid.UUID, uuid.UUID, time.Time, *orgsvc.OrgService) {
	tb.Helper()

	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")
	if !canDialPostgres(tb) {
		if isCI {
			tb.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		tb.Skip("postgres is not reachable; skipping org 053 integration test")
	}

	dbName := tb.Name()
	if !safeCreateDB(tb, dbName) {
		return nil, nil, uuid.Nil, uuid.Nil, time.Time{}, nil
	}

	pool := newPoolWithQueryTracer(tb, itf.DbOpts(dbName), &queryCountTracer{})
	tb.Cleanup(pool.Close)

	migrations := []string{
		"00001_org_baseline.sql",
		"20251218005114_org_placeholders_and_event_contracts.sql",
		"20251218130000_org_settings_and_audit.sql",
		"20251221090000_org_reason_code_mode.sql",
		"20251218150000_org_outbox.sql",
		"20251219090000_org_hierarchy_closure_and_snapshots.sql",
		"20251219195000_org_security_group_mappings_and_links.sql",
		"20251219220000_org_reporting_nodes_and_view.sql",
		"20251220160000_org_position_slices_and_fte.sql",
		"20251220200000_org_job_catalog_profiles_and_validation_modes.sql",
		"20251222120000_org_personnel_events.sql",
		"20251227090000_org_valid_time_day_granularity.sql",
	}
	for _, f := range migrations {
		sql := readGooseUpSQL(tb, filepath.Clean(filepath.Join("..", "..", "..", "migrations", "org", f)))
		_, err := pool.Exec(ctx, sql)
		require.NoError(tb, err, "failed migration %s", f)
	}

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000053")
	ensureTenant(tb, ctx, pool, tenantID)
	_, err := pool.Exec(ctx, `
INSERT INTO org_settings (tenant_id, freeze_mode, freeze_grace_days)
VALUES ($1,'disabled',0)
ON CONFLICT (tenant_id) DO UPDATE SET freeze_mode=excluded.freeze_mode, freeze_grace_days=excluded.freeze_grace_days
`, tenantID)
	require.NoError(tb, err)

	asOf := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	seedOrgTree(tb, ctx, pool, tenantID, 3, asOf, "deep", 53)
	nodes := buildPerfNodes(tb, tenantID, 3, "deep", 53)
	rootID := nodes[0].ID

	repo := persistence.NewOrgRepository()
	svc := orgsvc.NewOrgService(repo)
	return composables.WithPool(ctx, pool), pool, tenantID, rootID, asOf, svc
}

func TestOrg053ShiftBoundaryPosition_MovesAdjacentBoundary(t *testing.T) {
	ctx, pool, tenantID, rootNodeID, asOf, svc := setupOrg053DB(t)

	initiatorID := uuid.New()
	pos, err := svc.CreatePosition(ctx, tenantID, "req-053-create", initiatorID, orgsvc.CreatePositionInput{
		Code:               "POS-A",
		OrgNodeID:          rootNodeID,
		EffectiveDate:      asOf,
		PositionType:       "regular",
		EmploymentType:     "full_time",
		JobFamilyGroupCode: "TST",
		JobFamilyCode:      "TST-FAMILY",
		JobRoleCode:        "TST-ROLE",
		JobLevelCode:       "L1",
		CapacityFTE:        1.0,
		ReasonCode:         "create",
	})
	require.NoError(t, err)

	secondStart := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	_, err = svc.UpdatePosition(ctx, tenantID, "req-053-update", initiatorID, orgsvc.UpdatePositionInput{
		PositionID:    pos.PositionID,
		EffectiveDate: secondStart,
		Title:         ptr("B"),
		ReasonCode:    "update",
	})
	require.NoError(t, err)

	newStart := time.Date(2025, 2, 5, 0, 0, 0, 0, time.UTC)
	_, err = svc.ShiftBoundaryPosition(ctx, tenantID, "req-053-shift", initiatorID, orgsvc.ShiftBoundaryPositionInput{
		PositionID:          pos.PositionID,
		TargetEffectiveDate: secondStart,
		NewEffectiveDate:    newStart,
		ReasonCode:          "correct",
	})
	require.NoError(t, err)

	type sliceWindow struct {
		EffectiveDate time.Time
		EndDate       time.Time
	}
	var got []sliceWindow
	rows, err := pool.Query(ctx, `
SELECT effective_date, end_date
FROM org_position_slices
WHERE tenant_id=$1 AND position_id=$2
ORDER BY effective_date ASC
`, tenantID, pos.PositionID)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var w sliceWindow
		require.NoError(t, rows.Scan(&w.EffectiveDate, &w.EndDate))
		got = append(got, w)
	}
	require.NoError(t, rows.Err())
	require.Len(t, got, 2)
	require.True(t, got[0].EffectiveDate.Equal(asOf))
	require.True(t, got[0].EndDate.Equal(newStart))
	require.True(t, got[1].EffectiveDate.Equal(newStart))
	require.True(t, got[1].EndDate.Equal(time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)))
}

func TestOrg053ReportsToCycle_IsRejected(t *testing.T) {
	ctx, _, tenantID, rootNodeID, asOf, svc := setupOrg053DB(t)

	initiatorID := uuid.New()
	a, err := svc.CreatePosition(ctx, tenantID, "req-053-a", initiatorID, orgsvc.CreatePositionInput{
		Code:               "POS-A",
		OrgNodeID:          rootNodeID,
		EffectiveDate:      asOf,
		PositionType:       "regular",
		EmploymentType:     "full_time",
		JobFamilyGroupCode: "TST",
		JobFamilyCode:      "TST-FAMILY",
		JobRoleCode:        "TST-ROLE",
		JobLevelCode:       "L1",
		CapacityFTE:        1.0,
		ReasonCode:         "create",
	})
	require.NoError(t, err)
	b, err := svc.CreatePosition(ctx, tenantID, "req-053-b", initiatorID, orgsvc.CreatePositionInput{
		Code:               "POS-B",
		OrgNodeID:          rootNodeID,
		EffectiveDate:      asOf,
		PositionType:       "regular",
		EmploymentType:     "full_time",
		JobFamilyGroupCode: "TST",
		JobFamilyCode:      "TST-FAMILY",
		JobRoleCode:        "TST-ROLE",
		JobLevelCode:       "L1",
		CapacityFTE:        1.0,
		ReportsToID:        &a.PositionID,
		ReasonCode:         "create",
	})
	require.NoError(t, err)
	c, err := svc.CreatePosition(ctx, tenantID, "req-053-c", initiatorID, orgsvc.CreatePositionInput{
		Code:               "POS-C",
		OrgNodeID:          rootNodeID,
		EffectiveDate:      asOf,
		PositionType:       "regular",
		EmploymentType:     "full_time",
		JobFamilyGroupCode: "TST",
		JobFamilyCode:      "TST-FAMILY",
		JobRoleCode:        "TST-ROLE",
		JobLevelCode:       "L1",
		CapacityFTE:        1.0,
		ReportsToID:        &b.PositionID,
		ReasonCode:         "create",
	})
	require.NoError(t, err)

	_, err = svc.CorrectPosition(ctx, tenantID, "req-053-cycle", initiatorID, orgsvc.CorrectPositionInput{
		PositionID:  a.PositionID,
		AsOf:        asOf,
		ReportsToID: &c.PositionID,
		ReasonCode:  "correct",
	})
	var svcErr *orgsvc.ServiceError
	require.ErrorAs(t, err, &svcErr)
	require.Equal(t, 422, svcErr.Status)
	require.Equal(t, "ORG_POSITION_REPORTS_TO_CYCLE", svcErr.Code)
}

func TestOrg053Position_ContractFieldsArePersistedAndReadable(t *testing.T) {
	ctx, _, tenantID, rootNodeID, asOf, svc := setupOrg053DB(t)

	initiatorID := uuid.New()
	profile := json.RawMessage(`{"k":"v"}`)
	pos, err := svc.CreatePosition(ctx, tenantID, "req-053-fields-create", initiatorID, orgsvc.CreatePositionInput{
		Code:               "POS-FIELDS",
		OrgNodeID:          rootNodeID,
		EffectiveDate:      asOf,
		Title:              ptr("Fields"),
		LifecycleStatus:    "planned",
		PositionType:       "regular",
		EmploymentType:     "full_time",
		CapacityFTE:        1.0,
		JobFamilyGroupCode: "TST",
		JobFamilyCode:      "TST-FAMILY",
		JobRoleCode:        "TST-ROLE",
		JobLevelCode:       "L1",
		CostCenterCode:     ptr("CC-001"),
		Profile:            profile,
		ReasonCode:         "create",
	})
	require.NoError(t, err)

	got, gotAsOf, err := svc.GetPosition(ctx, tenantID, pos.PositionID, &asOf)
	require.NoError(t, err)
	require.True(t, gotAsOf.Equal(asOf))
	require.Equal(t, "regular", derefStringPtr(got.PositionType))
	require.Equal(t, "full_time", derefStringPtr(got.EmploymentType))
	require.Equal(t, "TST", derefStringPtr(got.JobFamilyGroupCode))
	require.Equal(t, "TST-FAMILY", derefStringPtr(got.JobFamilyCode))
	require.Equal(t, "TST-ROLE", derefStringPtr(got.JobRoleCode))
	require.Equal(t, "L1", derefStringPtr(got.JobLevelCode))
	require.Equal(t, "CC-001", derefStringPtr(got.CostCenterCode))

	var profileObj map[string]any
	require.NoError(t, json.Unmarshal(got.Profile, &profileObj))
	require.Equal(t, "v", profileObj["k"])

	secondStart := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	newProfile := json.RawMessage(`{"k":"v2"}`)
	_, err = svc.UpdatePosition(ctx, tenantID, "req-053-fields-update", initiatorID, orgsvc.UpdatePositionInput{
		PositionID:    pos.PositionID,
		EffectiveDate: secondStart,
		JobLevelCode:  ptr("L2"),
		Profile:       &newProfile,
		ReasonCode:    "update",
	})
	require.NoError(t, err)

	got2, got2AsOf, err := svc.GetPosition(ctx, tenantID, pos.PositionID, &secondStart)
	require.NoError(t, err)
	require.True(t, got2AsOf.Equal(secondStart))
	require.Equal(t, "L2", derefStringPtr(got2.JobLevelCode))
	require.NoError(t, json.Unmarshal(got2.Profile, &profileObj))
	require.Equal(t, "v2", profileObj["k"])
}

func ptr[T any](v T) *T { return &v }

func derefStringPtr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
