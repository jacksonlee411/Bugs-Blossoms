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

func TestOrg068LongNameProjectionQueryBudget(t *testing.T) {
	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")

	if !canDialPostgres(t) {
		if isCI {
			t.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		t.Skip("postgres is not reachable; skipping query budget test")
	}

	dbName := t.Name()
	if !safeCreateDB(t, dbName) {
		return
	}

	tracer := &queryCountTracer{}
	pool := newPoolWithQueryTracer(t, itf.DbOpts(dbName), tracer)
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

	asOf := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	seedOrgTree(t, ctx, pool, tenantID, 1000, asOf, "balanced", 68)

	nodeIDs := querySomeOrgNodeIDs(t, ctx, pool, tenantID, 200)
	require.Greater(t, len(nodeIDs), 10)

	queriesSingle := measureOrg068LongNamesAsOfQueries(t, ctx, pool, tracer, tenantID, asOf, nodeIDs)
	const expectedSingle = 1
	require.Equal(t, expectedSingle, queriesSingle, "unexpected query count for single as-of projection")

	mixedQueries := make([]orglabels.OrgNodeLongNameQuery, 0, len(nodeIDs))
	for i, id := range nodeIDs {
		day := asOf
		if i%2 == 1 {
			day = asOf.AddDate(0, 0, 1)
		}
		mixedQueries = append(mixedQueries, orglabels.OrgNodeLongNameQuery{OrgNodeID: id, AsOfDay: day})
	}
	queriesMixed := measureOrg068LongNamesMixedQueries(t, ctx, pool, tracer, tenantID, mixedQueries)
	const expectedMixed = 1
	require.Equal(t, expectedMixed, queriesMixed, "unexpected query count for mixed as-of projection")
}

func measureOrg068LongNamesAsOfQueries(
	tb testing.TB,
	ctx context.Context,
	pool *pgxpool.Pool,
	tracer *queryCountTracer,
	tenantID uuid.UUID,
	asOf time.Time,
	nodeIDs []uuid.UUID,
) int {
	tb.Helper()

	tx, err := pool.Begin(ctx)
	require.NoError(tb, err)
	defer func() { _ = tx.Rollback(ctx) }()

	reqCtx := composables.WithPool(ctx, pool)
	reqCtx = composables.WithTx(reqCtx, tx)

	tracer.Reset()

	out, err := orglabels.ResolveOrgNodeLongNamesAsOf(reqCtx, tenantID, asOf, nodeIDs)
	require.NoError(tb, err)
	require.NotEmpty(tb, out)

	return tracer.Count()
}

func measureOrg068LongNamesMixedQueries(
	tb testing.TB,
	ctx context.Context,
	pool *pgxpool.Pool,
	tracer *queryCountTracer,
	tenantID uuid.UUID,
	queries []orglabels.OrgNodeLongNameQuery,
) int {
	tb.Helper()

	tx, err := pool.Begin(ctx)
	require.NoError(tb, err)
	defer func() { _ = tx.Rollback(ctx) }()

	reqCtx := composables.WithPool(ctx, pool)
	reqCtx = composables.WithTx(reqCtx, tx)

	tracer.Reset()

	out, err := orglabels.ResolveOrgNodeLongNames(reqCtx, tenantID, queries)
	require.NoError(tb, err)
	require.NotEmpty(tb, out)

	return tracer.Count()
}

func querySomeOrgNodeIDs(tb testing.TB, ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID, limit int) []uuid.UUID {
	tb.Helper()

	rows, err := pool.Query(ctx, `
SELECT id
FROM org_nodes
WHERE tenant_id=$1
ORDER BY id
LIMIT $2
`, tenantID, limit)
	require.NoError(tb, err)
	defer rows.Close()

	out := make([]uuid.UUID, 0, limit)
	for rows.Next() {
		var id uuid.UUID
		require.NoError(tb, rows.Scan(&id))
		out = append(out, id)
	}
	require.NoError(tb, rows.Err())
	return out
}
