package services_test

import (
	"context"
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
	"github.com/iota-uz/iota-sdk/pkg/orglabels"
)

func setupOrg069DB(tb testing.TB) (context.Context, *pgxpool.Pool, uuid.UUID, time.Time, *orgsvc.OrgService) {
	tb.Helper()

	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")
	if !canDialPostgres(tb) {
		if isCI {
			tb.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		tb.Skip("postgres is not reachable; skipping org 069 integration test")
	}

	dbName := tb.Name()
	if !safeCreateDB(tb, dbName) {
		return nil, nil, uuid.Nil, time.Time{}, nil
	}

	pool := newPoolWithQueryTracer(tb, itf.DbOpts(dbName), &queryCountTracer{})
	tb.Cleanup(pool.Close)

	root := filepath.Clean(filepath.Join("..", "..", ".."))
	migrations := []string{
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
	for _, f := range migrations {
		sql := readGooseUpSQL(tb, filepath.Join(root, "migrations", "org", f))
		_, err := pool.Exec(ctx, sql)
		require.NoError(tb, err, "failed migration %s", f)
	}

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000069")
	ensureTenant(tb, ctx, pool, tenantID)
	_, err := pool.Exec(ctx, `
INSERT INTO org_settings (tenant_id, freeze_mode, freeze_grace_days)
VALUES ($1,'disabled',0)
ON CONFLICT (tenant_id) DO UPDATE SET freeze_mode=excluded.freeze_mode, freeze_grace_days=excluded.freeze_grace_days
`, tenantID)
	require.NoError(tb, err)

	asOf := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	repo := persistence.NewOrgRepository()
	svc := orgsvc.NewOrgService(repo)
	return composables.WithPool(ctx, pool), pool, tenantID, asOf, svc
}

func TestOrg069_CorrectMove_RewritesFutureDescendantPaths(t *testing.T) {
	ctx, pool, tenantID, asOf, svc := setupOrg069DB(t)

	rootID := uuid.New()
	xID := uuid.New()
	yID := uuid.New()
	zID := uuid.New()

	nodes := []perfNode{
		{ID: rootID, Code: "ROOT", ParentID: nil, DisplayOrder: 0},
		{ID: xID, Code: "X", ParentID: &rootID, DisplayOrder: 0},
		{ID: yID, Code: "Y", ParentID: &rootID, DisplayOrder: 1},
		{ID: zID, Code: "Z", ParentID: &yID, DisplayOrder: 0},
	}
	seedOrgTreeFromNodes(t, context.Background(), pool, tenantID, nodes, asOf)

	initiatorID := uuid.New()
	e1 := asOf.AddDate(0, 0, 9)

	_, err := svc.MoveNode(ctx, tenantID, "req-069-move-z", initiatorID, orgsvc.MoveNodeInput{
		NodeID:        zID,
		NewParentID:   yID,
		EffectiveDate: e1,
	})
	require.NoError(t, err)

	_, err = svc.CorrectMoveNode(ctx, tenantID, "req-069-correct-move-y", initiatorID, orgsvc.CorrectMoveNodeInput{
		NodeID:        yID,
		NewParentID:   xID,
		EffectiveDate: asOf,
	})
	require.NoError(t, err)

	byID, err := orglabels.ResolveOrgNodeLongNamesAsOf(ctx, tenantID, e1, []uuid.UUID{zID})
	require.NoError(t, err)
	require.Equal(t, "ROOT / X / Y / Z", byID[zID])

	var inconsistent int
	err = pool.QueryRow(context.Background(), `
SELECT count(*) AS inconsistent_edges
FROM org_edges c
JOIN org_edges p
  ON p.tenant_id=c.tenant_id
 AND p.hierarchy_type=c.hierarchy_type
 AND p.child_node_id=c.parent_node_id
 AND p.effective_date <= c.effective_date
 AND p.end_date >= c.effective_date
WHERE c.tenant_id=$1
  AND c.hierarchy_type=$2
  AND c.parent_node_id IS NOT NULL
  AND NOT (p.path @> c.path)
`, tenantID, "OrgUnit").Scan(&inconsistent)
	require.NoError(t, err)
	require.Equal(t, 0, inconsistent)
}
