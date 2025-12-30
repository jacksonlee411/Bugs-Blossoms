package services_test

import (
	"context"
	"fmt"
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

func TestOrg029DeepReadConsistency(t *testing.T) {
	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")

	if !canDialPostgres(t) {
		if isCI {
			t.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		t.Skip("postgres is not reachable; skipping deep read consistency test")
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
		"20251228120000_org_eliminate_effective_on_end_on.sql",
		"20251228140000_org_assignment_employment_status.sql",
		"20251228150000_org_gap_free_constraint_triggers.sql",
		"20251230090000_org_job_architecture_workday_profiles.sql",
	}
	for _, f := range files {
		sql := readGooseUpSQL(t, filepath.Clean(filepath.Join("..", "..", "..", "migrations", "org", f)))
		_, err := pool.Exec(ctx, sql)
		require.NoError(t, err, "failed migration %s", f)
	}

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ensureTenant(t, ctx, pool, tenantID)

	asOfDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	nodes := buildPerfNodes(t, tenantID, 32, "balanced", 42)
	seedOrgTreeFromNodes(t, ctx, pool, tenantID, nodes, asOfDate)

	repo := persistence.NewOrgRepository()
	reqCtx := composables.WithPool(ctx, pool)

	_, err := repo.BuildDeepReadClosure(reqCtx, tenantID, "OrgUnit", true, "test")
	require.NoError(t, err)
	_, err = repo.BuildDeepReadSnapshot(reqCtx, tenantID, "OrgUnit", asOfDate, true, "test")
	require.NoError(t, err)

	target := nodes[len(nodes)-1].ID
	root := nodes[0].ID

	ancEdges, err := repo.ListAncestorsAsOf(reqCtx, tenantID, "OrgUnit", target, asOfDate, orgsvc.DeepReadBackendEdges)
	require.NoError(t, err)
	ancClosure, err := repo.ListAncestorsAsOf(reqCtx, tenantID, "OrgUnit", target, asOfDate, orgsvc.DeepReadBackendClosure)
	require.NoError(t, err)
	ancSnapshot, err := repo.ListAncestorsAsOf(reqCtx, tenantID, "OrgUnit", target, asOfDate, orgsvc.DeepReadBackendSnapshot)
	require.NoError(t, err)
	require.Equal(t, relationsToMap(ancEdges), relationsToMap(ancClosure), "ancestors mismatch: edges vs closure")
	require.Equal(t, relationsToMap(ancEdges), relationsToMap(ancSnapshot), "ancestors mismatch: edges vs snapshot")

	desEdges, err := repo.ListDescendantsAsOf(reqCtx, tenantID, "OrgUnit", root, asOfDate, orgsvc.DeepReadBackendEdges)
	require.NoError(t, err)
	desClosure, err := repo.ListDescendantsAsOf(reqCtx, tenantID, "OrgUnit", root, asOfDate, orgsvc.DeepReadBackendClosure)
	require.NoError(t, err)
	desSnapshot, err := repo.ListDescendantsAsOf(reqCtx, tenantID, "OrgUnit", root, asOfDate, orgsvc.DeepReadBackendSnapshot)
	require.NoError(t, err)
	require.Equal(t, relationsToMap(desEdges), relationsToMap(desClosure), "descendants mismatch: edges vs closure")
	require.Equal(t, relationsToMap(desEdges), relationsToMap(desSnapshot), "descendants mismatch: edges vs snapshot")
}

func TestOrg029RoleAssignmentsConsistency(t *testing.T) {
	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")

	if !canDialPostgres(t) {
		if isCI {
			t.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		t.Skip("postgres is not reachable; skipping role assignments consistency test")
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
		"20251228120000_org_eliminate_effective_on_end_on.sql",
		"20251228140000_org_assignment_employment_status.sql",
		"20251228150000_org_gap_free_constraint_triggers.sql",
		"20251230090000_org_job_architecture_workday_profiles.sql",
	}
	for _, f := range files {
		sql := readGooseUpSQL(t, filepath.Clean(filepath.Join("..", "..", "..", "migrations", "org", f)))
		_, err := pool.Exec(ctx, sql)
		require.NoError(t, err, "failed migration %s", f)
	}

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ensureTenant(t, ctx, pool, tenantID)

	asOfDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	nodes := buildPerfNodes(t, tenantID, 16, "balanced", 42)
	seedOrgTreeFromNodes(t, ctx, pool, tenantID, nodes, asOfDate)

	rootID := nodes[0].ID
	targetID := nodes[len(nodes)-1].ID

	var roleID uuid.UUID
	err := pool.QueryRow(ctx, `
INSERT INTO org_roles (tenant_id, code, name, description, is_system)
VALUES ($1, 'Org.Admin', 'Org Admin', NULL, true)
RETURNING id
`, tenantID).Scan(&roleID)
	require.NoError(t, err)

	subjectID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("subject:029"))
	endDate := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
	_, err = pool.Exec(ctx, `
	INSERT INTO org_role_assignments (tenant_id, role_id, subject_type, subject_id, org_node_id, effective_date, end_date)
	VALUES (
		$1,$2,'user',$3,$4,
		($5 AT TIME ZONE 'UTC')::date,
		($6 AT TIME ZONE 'UTC')::date
	)
		`, tenantID, roleID, subjectID, rootID, asOfDate, endDate)
	require.NoError(t, err)

	repo := persistence.NewOrgRepository()
	reqCtx := composables.WithPool(ctx, pool)

	_, buildErr := repo.BuildDeepReadClosure(reqCtx, tenantID, "OrgUnit", true, "test")
	require.NoError(t, buildErr)
	_, buildErr = repo.BuildDeepReadSnapshot(reqCtx, tenantID, "OrgUnit", asOfDate, true, "test")
	require.NoError(t, buildErr)

	rowsEdges, err := repo.ListRoleAssignmentsAsOf(reqCtx, tenantID, "OrgUnit", targetID, asOfDate, true, orgsvc.DeepReadBackendEdges, nil, nil, nil)
	require.NoError(t, err)
	rowsClosure, err := repo.ListRoleAssignmentsAsOf(reqCtx, tenantID, "OrgUnit", targetID, asOfDate, true, orgsvc.DeepReadBackendClosure, nil, nil, nil)
	require.NoError(t, err)
	rowsSnapshot, err := repo.ListRoleAssignmentsAsOf(reqCtx, tenantID, "OrgUnit", targetID, asOfDate, true, orgsvc.DeepReadBackendSnapshot, nil, nil, nil)
	require.NoError(t, err)

	require.Equal(t, roleAssignmentsToSet(rowsEdges), roleAssignmentsToSet(rowsClosure), "role assignments mismatch: edges vs closure")
	require.Equal(t, roleAssignmentsToSet(rowsEdges), roleAssignmentsToSet(rowsSnapshot), "role assignments mismatch: edges vs snapshot")
}

func TestOrg029DeepReadQueryBudget(t *testing.T) {
	ctx := context.Background()
	isCI := strings.TrimSpace(getenvDefault("CI", "")) != "" || strings.EqualFold(strings.TrimSpace(getenvDefault("GITHUB_ACTIONS", "")), "true")

	if !canDialPostgres(t) {
		if isCI {
			t.Fatalf("postgres is not reachable (DB_HOST/DB_PORT).")
		}
		t.Skip("postgres is not reachable; skipping deep read query budget test")
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
		"20251228140000_org_assignment_employment_status.sql",
		"20251228150000_org_gap_free_constraint_triggers.sql",
		"20251230090000_org_job_architecture_workday_profiles.sql",
	}
	for _, f := range files {
		sql := readGooseUpSQL(t, filepath.Clean(filepath.Join("..", "..", "..", "migrations", "org", f)))
		_, err := pool.Exec(ctx, sql)
		require.NoError(t, err, "failed migration %s", f)
	}

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ensureTenant(t, ctx, pool, tenantID)

	asOfDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	nodes := buildPerfNodes(t, tenantID, 32, "deep", 42)
	seedOrgTreeFromNodes(t, ctx, pool, tenantID, nodes, asOfDate)

	repo := persistence.NewOrgRepository()
	reqCtx := composables.WithPool(ctx, pool)

	_, err := repo.BuildDeepReadClosure(reqCtx, tenantID, "OrgUnit", true, "test")
	require.NoError(t, err)
	_, err = repo.BuildDeepReadSnapshot(reqCtx, tenantID, "OrgUnit", asOfDate, true, "test")
	require.NoError(t, err)

	target := nodes[len(nodes)-1].ID
	root := nodes[0].ID

	tracer.Reset()
	_, err = repo.ListAncestorsAsOf(reqCtx, tenantID, "OrgUnit", target, asOfDate, orgsvc.DeepReadBackendEdges)
	require.NoError(t, err)
	require.Equal(t, 1, tracer.Count(), "unexpected query count for ancestors (edges)")

	tracer.Reset()
	_, err = repo.ListAncestorsAsOf(reqCtx, tenantID, "OrgUnit", target, asOfDate, orgsvc.DeepReadBackendSnapshot)
	require.NoError(t, err)
	require.Equal(t, 2, tracer.Count(), "unexpected query count for ancestors (snapshot)")

	tracer.Reset()
	_, err = repo.ListAncestorsAsOf(reqCtx, tenantID, "OrgUnit", target, asOfDate, orgsvc.DeepReadBackendClosure)
	require.NoError(t, err)
	require.Equal(t, 2, tracer.Count(), "unexpected query count for ancestors (closure)")

	tracer.Reset()
	_, err = repo.ListDescendantsAsOf(reqCtx, tenantID, "OrgUnit", root, asOfDate, orgsvc.DeepReadBackendEdges)
	require.NoError(t, err)
	require.Equal(t, 1, tracer.Count(), "unexpected query count for descendants (edges)")

	tracer.Reset()
	_, err = repo.ListDescendantsAsOf(reqCtx, tenantID, "OrgUnit", root, asOfDate, orgsvc.DeepReadBackendSnapshot)
	require.NoError(t, err)
	require.Equal(t, 2, tracer.Count(), "unexpected query count for descendants (snapshot)")

	tracer.Reset()
	_, err = repo.ListDescendantsAsOf(reqCtx, tenantID, "OrgUnit", root, asOfDate, orgsvc.DeepReadBackendClosure)
	require.NoError(t, err)
	require.Equal(t, 2, tracer.Count(), "unexpected query count for descendants (closure)")
}

func seedOrgTreeFromNodes(tb testing.TB, ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID, nodes []perfNode, asOf time.Time) {
	tb.Helper()

	endDate := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
	for _, n := range nodes {
		_, err := pool.Exec(ctx, "INSERT INTO org_nodes (tenant_id, id, type, code, is_root) VALUES ($1,$2,$3,$4,$5)",
			tenantID, n.ID, "OrgUnit", n.Code, n.ParentID == nil,
		)
		require.NoError(tb, err)
	}
	for _, n := range nodes {
		_, err := pool.Exec(ctx, `INSERT INTO org_node_slices
					(tenant_id, org_node_id, name, display_order, parent_hint, effective_date, end_date)
					VALUES (
						$1,$2,$3,$4,$5,
						($6 AT TIME ZONE 'UTC')::date,
						($7 AT TIME ZONE 'UTC')::date
					)`,
			tenantID, n.ID, n.Code, n.DisplayOrder, n.ParentID, asOf, endDate,
		)
		require.NoError(tb, err)
	}
	for _, n := range nodes {
		_, err := pool.Exec(ctx, `INSERT INTO org_edges
					(tenant_id, hierarchy_type, parent_node_id, child_node_id, effective_date, end_date)
					VALUES (
						$1,$2,$3,$4,
						($5 AT TIME ZONE 'UTC')::date,
						($6 AT TIME ZONE 'UTC')::date
					)`,
			tenantID, "OrgUnit", n.ParentID, n.ID, asOf, endDate,
		)
		require.NoError(tb, err)
	}
}

func relationsToMap(in []orgsvc.DeepReadRelation) map[uuid.UUID]int {
	out := make(map[uuid.UUID]int, len(in))
	for _, r := range in {
		out[r.NodeID] = r.Depth
	}
	return out
}

func roleAssignmentsToSet(in []orgsvc.RoleAssignmentRow) map[string]struct{} {
	out := make(map[string]struct{}, len(in))
	for _, r := range in {
		key := fmt.Sprintf("%s|%s|%s|%s", r.RoleCode, r.SubjectType, r.SubjectID.String(), r.SourceOrgNodeID.String())
		out[key] = struct{}{}
	}
	return out
}
