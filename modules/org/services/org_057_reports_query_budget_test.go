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

	"github.com/iota-uz/iota-sdk/modules/org/infrastructure/persistence"
	orgsvc "github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/itf"
)

func TestOrg057StaffingSummaryQueryBudget(t *testing.T) {
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

	asOf := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)

	schemaSQL := readGooseUpSQL(t, filepath.Clean(filepath.Join("..", "..", "..", "migrations", "org", "00001_org_baseline.sql")))
	_, err := pool.Exec(ctx, schemaSQL, pgx.QueryExecModeSimpleProtocol)
	require.NoError(t, err)

	largeTenantID := uuid.MustParse("00000000-0000-0000-0000-000000000057")
	smallTenantID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(largeTenantID.String()+":small"))
	ensureTenant(t, ctx, pool, smallTenantID)
	ensureTenant(t, ctx, pool, largeTenantID)

	seedOrgTree(t, ctx, pool, smallTenantID, 10, asOf, "balanced", 57)
	seedOrgTree(t, ctx, pool, largeTenantID, 1000, asOf, "balanced", 57)

	smallRoot := queryRootNodeID(t, ctx, pool, smallTenantID)
	largeRoot := queryRootNodeID(t, ctx, pool, largeTenantID)

	smallPosID := uuid.New()
	largePosID := uuid.New()
	_, err = pool.Exec(ctx, `
INSERT INTO org_positions (tenant_id, id, org_node_id, code, status, is_auto_created, effective_date, end_date)
VALUES ($1,$2,$3,$4,'active',false,$5,$6)
`, smallTenantID, smallPosID, smallRoot, "POS-SMALL", asOf, endDate)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
INSERT INTO org_positions (tenant_id, id, org_node_id, code, status, is_auto_created, effective_date, end_date)
VALUES ($1,$2,$3,$4,'active',false,$5,$6)
`, largeTenantID, largePosID, largeRoot, "POS-LARGE", asOf, endDate)
	require.NoError(t, err)

	m053 := readGooseUpSQL(t, filepath.Clean(filepath.Join("..", "..", "..", "migrations", "org", "20251220160000_org_position_slices_and_fte.sql")))
	_, err = pool.Exec(ctx, m053)
	require.NoError(t, err)

	repo := persistence.NewOrgRepository()
	svc := orgsvc.NewOrgService(repo)
	reqCtx := composables.WithPool(ctx, pool)

	queriesSmall := measureOrg057SummaryQueries(t, reqCtx, tracer, svc, smallTenantID, asOf)
	queriesLarge := measureOrg057SummaryQueries(t, reqCtx, tracer, svc, largeTenantID, asOf)

	const expected = 4
	require.Equal(t, expected, queriesSmall, "unexpected query count for small dataset")
	require.Equal(t, expected, queriesLarge, "unexpected query count for large dataset")
}

func measureOrg057SummaryQueries(tb testing.TB, ctx context.Context, tracer *queryCountTracer, svc *orgsvc.OrgService, tenantID uuid.UUID, asOf time.Time) int {
	tb.Helper()

	pool, err := composables.UsePool(ctx)
	require.NoError(tb, err)

	tx, err := pool.Begin(ctx)
	require.NoError(tb, err)
	defer func() { _ = tx.Rollback(ctx) }()

	reqCtx := composables.WithPool(ctx, pool)
	reqCtx = composables.WithTx(reqCtx, tx)

	tracer.Reset()

	_, err = svc.GetStaffingSummary(reqCtx, tenantID, orgsvc.StaffingSummaryInput{
		EffectiveDate: asOf,
		Scope:         orgsvc.StaffingScopeSubtree,
		GroupBy:       orgsvc.StaffingGroupByNone,
	})
	require.NoError(tb, err)
	return tracer.Count()
}

func queryRootNodeID(tb testing.TB, ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID) uuid.UUID {
	tb.Helper()

	var id uuid.UUID
	err := pool.QueryRow(ctx, `SELECT id FROM org_nodes WHERE tenant_id=$1 AND is_root=true LIMIT 1`, tenantID).Scan(&id)
	require.NoError(tb, err)
	return id
}
