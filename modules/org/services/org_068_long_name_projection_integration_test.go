package services_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/itf"
	"github.com/iota-uz/iota-sdk/pkg/orglabels"
)

func TestOrg068LongNameProjectionAsOfCorrectness(t *testing.T) {
	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")

	if !canDialPostgres(t) {
		if isCI {
			t.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		t.Skip("postgres is not reachable; skipping integration test")
	}

	dbName := t.Name()
	if !safeCreateDB(t, dbName) {
		return
	}

	pool, err := pgxpool.New(ctx, itf.DbOpts(dbName))
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	files := []string{
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
	}
	for _, f := range files {
		sql := readGooseUpSQL(t, filepath.Clean(filepath.Join("..", "..", "..", "migrations", "org", f)))
		_, err := pool.Exec(ctx, sql, pgx.QueryExecModeSimpleProtocol)
		require.NoError(t, err, "failed migration %s", f)
	}

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ensureTenant(t, ctx, pool, tenantID)

	rootID := uuid.New()
	childID := uuid.New()
	_, err = pool.Exec(ctx, `
INSERT INTO org_nodes (tenant_id, id, type, code, is_root) VALUES
($1,$2,'OrgUnit','ROOT',true),
($1,$3,'OrgUnit','CHILD',false)
`, tenantID, rootID, childID)
	require.NoError(t, err)

	asOf1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	asOf2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)

	_, err = pool.Exec(ctx, `
INSERT INTO org_node_slices (tenant_id, org_node_id, name, status, display_order, effective_date, end_date)
VALUES
($1,$2,'Company','active',0,$4,$5),
($1,$3,'Dept','active',0,$4,$5)
`, tenantID, rootID, childID, asOf1, endDate)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
UPDATE org_node_slices
SET end_date=$1
WHERE tenant_id=$2 AND org_node_id=$3 AND effective_date=$4
`, asOf1, tenantID, rootID, asOf1)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
INSERT INTO org_node_slices (tenant_id, org_node_id, name, status, display_order, effective_date, end_date)
VALUES ($1,$2,'Company v2','active',0,$3,$4)
`, tenantID, rootID, asOf2, endDate)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
INSERT INTO org_edges (tenant_id, hierarchy_type, parent_node_id, child_node_id, effective_date, end_date)
VALUES
($1,'OrgUnit',NULL,$2,$3,$4),
($1,'OrgUnit',$2,$5,$3,$4)
`, tenantID, rootID, asOf1, endDate, childID)
	require.NoError(t, err)

	reqCtx := composables.WithPool(ctx, pool)
	long1, err := orglabels.ResolveOrgNodeLongNamesAsOf(reqCtx, tenantID, asOf1, []uuid.UUID{childID})
	require.NoError(t, err)
	require.Equal(t, "Company / Dept", strings.TrimSpace(long1[childID]))

	long2, err := orglabels.ResolveOrgNodeLongNamesAsOf(reqCtx, tenantID, asOf2, []uuid.UUID{childID})
	require.NoError(t, err)
	require.Equal(t, "Company v2 / Dept", strings.TrimSpace(long2[childID]))
}
