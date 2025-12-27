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
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/pkg/itf"
)

func TestOrg033NodePathAndExport(t *testing.T) {
	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")

	if !canDialPostgres(t) {
		if isCI {
			t.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		t.Skip("postgres is not reachable; skipping org 033 test")
	}

	dbName := t.Name()
	if !safeCreateDB(t, dbName) {
		return
	}

	pool := newPoolWithQueryTracer(t, itf.DbOpts(dbName), &queryCountTracer{})
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
	}
	for _, f := range files {
		sql := readGooseUpSQL(t, filepath.Clean(filepath.Join("..", "..", "..", "migrations", "org", f)))
		_, err := pool.Exec(ctx, sql)
		require.NoError(t, err, "failed migration %s", f)
	}

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ensureTenant(t, ctx, pool, tenantID)

	asOf := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	nodes := buildPerfNodes(t, tenantID, 32, "balanced", 42)
	seedOrgTreeFromNodes(t, ctx, pool, tenantID, nodes, asOf)

	repo := persistence.NewOrgRepository()
	svc := orgsvc.NewOrgService(repo)

	// Force edges backend for deterministic coverage (no snapshot build required).
	cfg := configuration.Use()
	prevRolloutMode := cfg.OrgRolloutMode
	prevRolloutTenants := cfg.OrgRolloutTenants
	prevDeepReadEnabled := cfg.OrgDeepReadEnabled
	prevDeepReadBackend := cfg.OrgDeepReadBackend
	cfg.OrgRolloutMode = "enabled"
	cfg.OrgRolloutTenants = tenantID.String()
	cfg.OrgDeepReadEnabled = false
	cfg.OrgDeepReadBackend = "edges"
	t.Cleanup(func() {
		cfg.OrgRolloutMode = prevRolloutMode
		cfg.OrgRolloutTenants = prevRolloutTenants
		cfg.OrgDeepReadEnabled = prevDeepReadEnabled
		cfg.OrgDeepReadBackend = prevDeepReadBackend
	})

	reqCtx := composables.WithPool(ctx, pool)

	target := nodes[len(nodes)-1].ID
	path, err := svc.GetNodePath(reqCtx, tenantID, target, asOf)
	require.NoError(t, err)
	require.NotEmpty(t, path.Path)
	require.Equal(t, target, path.Path[len(path.Path)-1].ID)
	require.Equal(t, 0, path.Path[0].Depth)

	export1, err := svc.ExportHierarchy(reqCtx, tenantID, "OrgUnit", asOf, nil, nil, true, false, false, 5, nil)
	require.NoError(t, err)
	require.Len(t, export1.Nodes, 5)
	require.NotNil(t, export1.NextCursorID)
	require.Contains(t, export1.Includes, "edges")

	export2, err := svc.ExportHierarchy(reqCtx, tenantID, "OrgUnit", asOf, nil, nil, true, false, false, 5, export1.NextCursorID)
	require.NoError(t, err)
	require.NotEmpty(t, export2.Nodes)
	require.NotEqual(t, export1.Nodes[0].ID, export2.Nodes[0].ID)
}

func TestOrg033PersonPath(t *testing.T) {
	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")

	if !canDialPostgres(t) {
		if isCI {
			t.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		t.Skip("postgres is not reachable; skipping org 033 person-path test")
	}

	dbName := t.Name()
	if !safeCreateDB(t, dbName) {
		return
	}

	pool := newPoolWithQueryTracer(t, itf.DbOpts(dbName), &queryCountTracer{})
	t.Cleanup(pool.Close)

	applyAllOrgMigrationsFor033(t, ctx, pool)

	personSchemaSQL := readGooseUpSQL(t, filepath.Clean(filepath.Join("..", "..", "..", "migrations", "person", "00001_person_baseline.sql")))
	_, err := pool.Exec(ctx, personSchemaSQL)
	require.NoError(t, err)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ensureTenant(t, ctx, pool, tenantID)

	asOf := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	nodes := buildPerfNodes(t, tenantID, 8, "balanced", 42)
	seedOrgTreeFromNodes(t, ctx, pool, tenantID, nodes, asOf)
	targetNodeID := nodes[len(nodes)-1].ID

	// Minimal position + primary assignment as-of.
	positionID := uuid.New()
	endDate := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
	_, err = pool.Exec(ctx, `
		INSERT INTO org_positions (tenant_id, id, org_node_id, code, status, is_auto_created, effective_date, end_date, effective_on, end_on)
		VALUES (
			$1,$2,$3,'P-1','active',false,$4,$5,
			($4 AT TIME ZONE 'UTC')::date,
			CASE
				WHEN ($5 AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
				ELSE ((($5 AT TIME ZONE 'UTC') - interval '1 microsecond'))::date
			END
		)
		`, tenantID, positionID, targetNodeID, asOf, endDate)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO org_position_slices (tenant_id, position_id, org_node_id, lifecycle_status, capacity_fte, effective_date, end_date, effective_on, end_on)
		VALUES (
			$1,$2,$3,'active',1.0::numeric(9,2),$4,$5,
			($4 AT TIME ZONE 'UTC')::date,
			CASE
				WHEN ($5 AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
				ELSE ((($5 AT TIME ZONE 'UTC') - interval '1 microsecond'))::date
			END
		)
		`, tenantID, positionID, targetNodeID, asOf, endDate)
	require.NoError(t, err)

	personUUID := uuid.New()
	_, err = pool.Exec(ctx, `
INSERT INTO persons (tenant_id, person_uuid, pernr, display_name, status)
VALUES ($1,$2,'000123','Test Person','active')
`, tenantID, personUUID)
	require.NoError(t, err)

	assignmentID := uuid.New()
	_, err = pool.Exec(ctx, `
	INSERT INTO org_assignments (tenant_id, id, position_id, subject_type, subject_id, pernr, assignment_type, is_primary, effective_date, end_date, effective_on, end_on)
	VALUES (
		$1,$2,$3,'person',$4,'000123','primary',true,$5,$6,
		($5 AT TIME ZONE 'UTC')::date,
		CASE
			WHEN ($6 AT TIME ZONE 'UTC')::date = DATE '9999-12-31' THEN DATE '9999-12-31'
			ELSE ((($6 AT TIME ZONE 'UTC') - interval '1 microsecond'))::date
		END
	)
	`, tenantID, assignmentID, positionID, personUUID, asOf, endDate)
	require.NoError(t, err)

	repo := persistence.NewOrgRepository()
	svc := orgsvc.NewOrgService(repo)
	reqCtx := composables.WithPool(ctx, pool)

	cfg := configuration.Use()
	prevRolloutMode := cfg.OrgRolloutMode
	prevRolloutTenants := cfg.OrgRolloutTenants
	prevDeepReadEnabled := cfg.OrgDeepReadEnabled
	cfg.OrgRolloutMode = "enabled"
	cfg.OrgRolloutTenants = tenantID.String()
	cfg.OrgDeepReadEnabled = false
	t.Cleanup(func() {
		cfg.OrgRolloutMode = prevRolloutMode
		cfg.OrgRolloutTenants = prevRolloutTenants
		cfg.OrgDeepReadEnabled = prevDeepReadEnabled
	})

	res, err := svc.GetPersonPath(reqCtx, tenantID, "person:000123", asOf)
	require.NoError(t, err)
	require.Equal(t, assignmentID, res.Assignment.AssignmentID)
	require.Equal(t, positionID, res.Assignment.PositionID)
	require.Equal(t, targetNodeID, res.Assignment.OrgNodeID)
	require.NotEmpty(t, res.Path)
}

func TestOrg033ReportingBuild(t *testing.T) {
	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")

	if !canDialPostgres(t) {
		if isCI {
			t.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		t.Skip("postgres is not reachable; skipping org 033 reporting build test")
	}

	dbName := t.Name()
	if !safeCreateDB(t, dbName) {
		return
	}

	pool := newPoolWithQueryTracer(t, itf.DbOpts(dbName), &queryCountTracer{})
	t.Cleanup(pool.Close)

	applyAllOrgMigrationsFor033(t, ctx, pool)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ensureTenant(t, ctx, pool, tenantID)

	asOf := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	nodes := buildPerfNodes(t, tenantID, 32, "balanced", 42)
	seedOrgTreeFromNodes(t, ctx, pool, tenantID, nodes, asOf)

	repo := persistence.NewOrgRepository()
	reqCtx := composables.WithPool(ctx, pool)

	_, err := repo.BuildDeepReadSnapshot(reqCtx, tenantID, "OrgUnit", asOf, true, "test")
	require.NoError(t, err)

	build, err := repo.BuildOrgReportingNodes(reqCtx, tenantID, "OrgUnit", asOf, false, false, true, "test")
	require.NoError(t, err)
	require.Positive(t, build.RowCount)

	var viewCount int64
	err = pool.QueryRow(ctx, `SELECT COUNT(*)::bigint FROM org_reporting WHERE tenant_id=$1 AND as_of_date=$2::date`, tenantID, asOf.Format("2006-01-02")).Scan(&viewCount)
	require.NoError(t, err)
	require.Equal(t, build.RowCount, viewCount)
}

func applyAllOrgMigrationsFor033(tb testing.TB, ctx context.Context, pool *pgxpool.Pool) {
	tb.Helper()

	applyAllPersonMigrations(tb, ctx, pool)

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
	}
	for _, f := range files {
		sql := readGooseUpSQL(tb, filepath.Clean(filepath.Join("..", "..", "..", "migrations", "org", f)))
		_, err := pool.Exec(ctx, sql)
		require.NoError(tb, err, "failed migration %s", f)
	}
}
