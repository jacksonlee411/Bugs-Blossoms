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

func setupOrg059DB(tb testing.TB) (context.Context, *pgxpool.Pool, uuid.UUID, uuid.UUID, time.Time, *orgsvc.OrgService) {
	tb.Helper()

	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")
	if !canDialPostgres(tb) {
		if isCI {
			tb.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		tb.Skip("postgres is not reachable; skipping org 059 integration test")
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
		"20251228120000_org_eliminate_effective_on_end_on.sql",
		"20251228140000_org_assignment_employment_status.sql",
		"20251228150000_org_gap_free_constraint_triggers.sql",
		"20251230090000_org_job_architecture_workday_profiles.sql",
		"20251231120000_org_remove_job_family_allocation_percent.sql",
	}
	for _, f := range migrations {
		sql := readGooseUpSQL(tb, filepath.Clean(filepath.Join("..", "..", "..", "migrations", "org", f)))
		_, err := pool.Exec(ctx, sql)
		require.NoError(tb, err, "failed migration %s", f)
	}

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000059")
	ensureTenant(tb, ctx, pool, tenantID)
	_, err := pool.Exec(ctx, `
INSERT INTO org_settings (tenant_id, freeze_mode, freeze_grace_days)
VALUES ($1,'disabled',0)
ON CONFLICT (tenant_id) DO UPDATE SET freeze_mode=excluded.freeze_mode, freeze_grace_days=excluded.freeze_grace_days
`, tenantID)
	require.NoError(tb, err)

	asOf := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	seedOrgTree(tb, ctx, pool, tenantID, 1, asOf, "deep", 59)
	nodes := buildPerfNodes(tb, tenantID, 1, "deep", 59)
	rootID := nodes[0].ID

	repo := persistence.NewOrgRepository()
	svc := orgsvc.NewOrgService(repo)
	return composables.WithPool(ctx, pool), pool, tenantID, rootID, asOf, svc
}

func TestOrg059PositionRescind_WritesAuditAndOutbox(t *testing.T) {
	ctx, pool, tenantID, rootNodeID, asOf, svc := setupOrg059DB(t)
	jobProfileID := seedOrg053JobProfile(t, ctx, tenantID, svc)

	initiatorID := uuid.New()

	pos, err := svc.CreatePosition(ctx, tenantID, "req-059-pos-create", initiatorID, orgsvc.CreatePositionInput{
		Code:           "POS-059-SMOKE",
		OrgNodeID:      rootNodeID,
		EffectiveDate:  asOf,
		PositionType:   "regular",
		EmploymentType: "full_time",
		JobProfileID:   jobProfileID,
		JobLevelCode:   ptr("L1"),
		CapacityFTE:    1.0,
		Profile:        json.RawMessage(`{}`),
		ReasonCode:     "create",
	})
	require.NoError(t, err)

	_, err = svc.RescindPosition(ctx, tenantID, "req-059-pos-rescind", initiatorID, orgsvc.RescindPositionInput{
		PositionID:    pos.PositionID,
		EffectiveDate: asOf,
		ReasonCode:    "rescind",
	})
	require.NoError(t, err)

	var changeType, reasonCode string
	err = pool.QueryRow(ctx, `
SELECT change_type, meta->>'reason_code'
FROM org_audit_logs
WHERE tenant_id=$1 AND request_id=$2
`, tenantID, "req-059-pos-rescind").Scan(&changeType, &reasonCode)
	require.NoError(t, err)
	require.Equal(t, "position.rescinded", changeType)
	require.Equal(t, "rescind", reasonCode)

	var outboxCount int64
	err = pool.QueryRow(ctx, `
SELECT COUNT(*)::bigint
FROM org_outbox
WHERE tenant_id=$1 AND payload->>'request_id'=$2
`, tenantID, "req-059-pos-rescind").Scan(&outboxCount)
	require.NoError(t, err)
	require.Equal(t, int64(1), outboxCount)
}
