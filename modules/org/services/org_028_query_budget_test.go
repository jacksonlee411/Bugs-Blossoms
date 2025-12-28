package services_test

import (
	"context"
	"os"
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
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/pkg/itf"
)

func TestOrg028QueryBudget(t *testing.T) {
	ctx := context.Background()
	isCI := strings.TrimSpace(os.Getenv("CI")) != "" || strings.EqualFold(strings.TrimSpace(os.Getenv("GITHUB_ACTIONS")), "true")

	profile := strings.ToLower(strings.TrimSpace(getenvDefault("PROFILE", "balanced")))
	seed := parseInt64Default(t, "SEED", 42)
	scale := parseScaleDefault(t, "SCALE", 1000)
	if scale < 10 {
		scale = 10
	}
	asOf := parseEffectiveDateDefault(t, "EFFECTIVE_DATE", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))

	largeTenantID := parseUUIDDefault(t, "TENANT_ID", uuid.MustParse("00000000-0000-0000-0000-000000000001"))
	smallTenantID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(largeTenantID.String()+":small"))

	if !canDialPostgres(t) {
		if isCI {
			t.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		t.Skip("postgres is not reachable; skipping query budget test")
	}

	cfg := configuration.Use()
	prevInheritanceEnabled := cfg.OrgInheritanceEnabled
	prevRoleReadEnabled := cfg.OrgRoleReadEnabled
	prevOrgCacheEnabled := cfg.OrgCacheEnabled
	cfg.OrgInheritanceEnabled = true
	cfg.OrgRoleReadEnabled = true
	cfg.OrgCacheEnabled = false
	t.Cleanup(func() {
		cfg.OrgInheritanceEnabled = prevInheritanceEnabled
		cfg.OrgRoleReadEnabled = prevRoleReadEnabled
		cfg.OrgCacheEnabled = prevOrgCacheEnabled
	})

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
		"20251228140000_org_assignment_employment_status.sql",
		"20251228150000_org_gap_free_constraint_triggers.sql",
	}
	for _, f := range files {
		sql := readGooseUpSQL(t, filepath.Clean(filepath.Join("..", "..", "..", "migrations", "org", f)))
		_, err := pool.Exec(ctx, sql, pgx.QueryExecModeSimpleProtocol)
		require.NoError(t, err, "failed migration %s", f)
	}

	ensureTenant(t, ctx, pool, largeTenantID)
	ensureTenant(t, ctx, pool, smallTenantID)

	seedOrgTree(t, ctx, pool, smallTenantID, 10, asOf, profile, seed)
	seedOrgTree(t, ctx, pool, largeTenantID, scale, asOf, profile, seed)

	seed028RulesAndRoles(t, ctx, pool, smallTenantID, 10, asOf, profile, seed)
	seed028RulesAndRoles(t, ctx, pool, largeTenantID, scale, asOf, profile, seed)

	repo := persistence.NewOrgRepository()
	svc := orgsvc.NewOrgService(repo)

	queriesResolvedSmall := measureResolvedHierarchyQueries(t, ctx, pool, tracer, svc, smallTenantID, asOf)
	queriesResolvedLarge := measureResolvedHierarchyQueries(t, ctx, pool, tracer, svc, largeTenantID, asOf)

	const expectedResolved = 3
	require.Equal(t, expectedResolved, queriesResolvedSmall, "unexpected resolved hierarchy query count for small dataset")
	require.Equal(t, expectedResolved, queriesResolvedLarge, "unexpected resolved hierarchy query count for large dataset")

	queriesRoleAssignmentsSmall := measureRoleAssignmentsQueries(t, ctx, pool, tracer, svc, smallTenantID, asOf, 10, profile, seed)
	queriesRoleAssignmentsLarge := measureRoleAssignmentsQueries(t, ctx, pool, tracer, svc, largeTenantID, asOf, scale, profile, seed)

	const expectedRoleAssignments = 3
	require.Equal(t, expectedRoleAssignments, queriesRoleAssignmentsSmall, "unexpected role-assignments query count for small dataset")
	require.Equal(t, expectedRoleAssignments, queriesRoleAssignmentsLarge, "unexpected role-assignments query count for large dataset")
}

func seed028RulesAndRoles(tb testing.TB, ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID, count int, asOf time.Time, profile string, seed int64) {
	tb.Helper()

	endDate := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)

	_, err := pool.Exec(ctx, `
			INSERT INTO org_attribute_inheritance_rules
				(tenant_id, hierarchy_type, attribute_name, can_override, effective_date, end_date)
			VALUES
				(
					$1,'OrgUnit','company_code',true,
					($2 AT TIME ZONE 'UTC')::date,
					($3 AT TIME ZONE 'UTC')::date
				)
			ON CONFLICT DO NOTHING
			`, tenantID, asOf, endDate)
	require.NoError(tb, err)

	rootID := buildPerfNodes(tb, tenantID, count, profile, seed)[0].ID

	roleID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(tenantID.String()+":role:HRBP"))
	_, err = pool.Exec(ctx, `
	INSERT INTO org_roles (tenant_id, id, code, name, is_system)
	VALUES ($1,$2,'HRBP','HR Business Partner',true)
	ON CONFLICT (tenant_id, code) DO NOTHING
	`, tenantID, roleID)
	require.NoError(tb, err)

	subjectID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(tenantID.String()+":user:1"))
	_, err = pool.Exec(ctx, `
			INSERT INTO org_role_assignments
				(tenant_id, role_id, subject_type, subject_id, org_node_id, effective_date, end_date)
			VALUES
				(
					$1,$2,'user',$3,$4,
					($5 AT TIME ZONE 'UTC')::date,
					($6 AT TIME ZONE 'UTC')::date
				)
			ON CONFLICT DO NOTHING
			`, tenantID, roleID, subjectID, rootID, asOf, endDate)
	require.NoError(tb, err)
}

func measureResolvedHierarchyQueries(tb testing.TB, ctx context.Context, pool *pgxpool.Pool, tracer *queryCountTracer, svc *orgsvc.OrgService, tenantID uuid.UUID, asOf time.Time) int {
	tb.Helper()

	tx, err := pool.Begin(ctx)
	require.NoError(tb, err)
	defer func() { _ = tx.Rollback(ctx) }()

	reqCtx := composables.WithPool(ctx, pool)
	reqCtx = composables.WithTx(reqCtx, tx)

	tracer.Reset()

	nodes, effectiveDate, sources, ruleAttributes, err := svc.GetHierarchyResolvedAttributes(reqCtx, tenantID, "OrgUnit", asOf)
	_ = effectiveDate
	_ = sources
	_ = ruleAttributes
	require.NoError(tb, err)
	require.NotEmpty(tb, nodes)

	return tracer.Count()
}

func measureRoleAssignmentsQueries(tb testing.TB, ctx context.Context, pool *pgxpool.Pool, tracer *queryCountTracer, svc *orgsvc.OrgService, tenantID uuid.UUID, asOf time.Time, count int, profile string, seed int64) int {
	tb.Helper()

	tx, err := pool.Begin(ctx)
	require.NoError(tb, err)
	defer func() { _ = tx.Rollback(ctx) }()

	reqCtx := composables.WithPool(ctx, pool)
	reqCtx = composables.WithTx(reqCtx, tx)

	tracer.Reset()

	nodes := buildPerfNodes(tb, tenantID, count, profile, seed)
	targetNodeID := nodes[len(nodes)-1].ID

	items, _, err := svc.ListRoleAssignments(reqCtx, tenantID, targetNodeID, asOf, true, nil, nil, nil)
	require.NoError(tb, err)
	require.NotEmpty(tb, items)

	return tracer.Count()
}
